package ovl

import (
	"encoding/binary"
	"fmt"
	"slices"
)

// A .mechanicresearchsettings root is a pointer to an array of Research
// entries and their count. Each entry starts with a pointer to its item
// name; its next_research pointer sits at +24 and its count at +32.
const (
	researchRootSize  = 16
	researchEntrySize = 56
	researchNextAt    = 24
)

// Research is one entry of a research settings file, as far as the shell
// needs it.
type Research struct {
	ItemName string
	// Next is the item names next_research points to.
	Next []string
}

type researchArray struct {
	located Located
	levels  int // fragment of the root's levels pointer
	array   Pointer
	count   uint64
}

func (f *File) researchArray(name string) (researchArray, error) {
	l, err := f.Locate(name)
	if err != nil {
		return researchArray{}, err
	}
	root := l.Content.RootEntries[l.Root].Struct
	rootBytes, err := l.RootBytes()
	if err != nil {
		return researchArray{}, err
	}
	if len(rootBytes) < researchRootSize {
		return researchArray{}, fmt.Errorf("%s: root of %d bytes", name, len(rootBytes))
	}
	levels, ok := l.Content.fragmentAt(root)
	if !ok {
		return researchArray{}, fmt.Errorf("%s: the levels pointer has no fragment", name)
	}
	return researchArray{
		located: l,
		levels:  levels,
		array:   l.Content.Fragments[levels].Struct,
		count:   binary.LittleEndian.Uint64(rootBytes[8:]),
	}, nil
}

// ResearchEntries reads a research settings file.
func (f *File) ResearchEntries(name string) ([]Research, error) {
	ra, err := f.researchArray(name)
	if err != nil {
		return nil, err
	}
	c := ra.located.Content
	out := make([]Research, 0, ra.count)
	for i := range ra.count {
		entry := Pointer{Pool: ra.array.Pool, Offset: ra.array.Offset + uint32(i)*researchEntrySize}
		item, err := c.stringAt(entry)
		if err != nil {
			return nil, fmt.Errorf("%s: entry %d item name: %w", name, i, err)
		}
		next, err := c.nextResearch(entry)
		if err != nil {
			return nil, fmt.Errorf("%s: entry %d next research: %w", name, i, err)
		}
		out = append(out, Research{ItemName: item, Next: next})
	}
	return out, nil
}

func (c *Content) nextResearch(entry Pointer) ([]string, error) {
	link := Pointer{Pool: entry.Pool, Offset: entry.Offset + researchNextAt}
	at, ok := c.fragmentAt(link)
	if !ok {
		return nil, nil
	}
	count := binary.LittleEndian.Uint64(c.PoolData[entry.Pool][entry.Offset+researchNextAt+8:])
	target := c.Fragments[at].Struct
	names := make([]string, 0, count)
	for i := range count {
		name, err := c.stringAt(Pointer{Pool: target.Pool, Offset: target.Offset + uint32(i)*8})
		if err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, nil
}

// stringAt follows the pointer stored at link to a zero-terminated string.
func (c *Content) stringAt(link Pointer) (string, error) {
	at, ok := c.fragmentAt(link)
	if !ok {
		return "", fmt.Errorf("no pointer at %w", errPointer(link))
	}
	target := c.Fragments[at].Struct
	if target.Pool < 0 || int(target.Pool) >= len(c.PoolData) {
		return "", fmt.Errorf("pointer into missing %w", errPointer(target))
	}
	return stringAt(c.PoolData[target.Pool], target.Offset), nil
}

// AppendResearch adds entries that are not completed, not an entry level,
// not enabled and lead nowhere: the shape the shell's gates need. The array
// moves to the end of its pool with room for them, the old copy stays behind
// unused, and every pointer into or out of it follows.
func (f *File) AppendResearch(name string, items []string) error {
	ra, err := f.researchArray(name)
	if err != nil {
		return err
	}
	c := ra.located.Content
	oldStart, oldEnd := ra.array.Offset, ra.array.Offset+uint32(ra.count)*researchEntrySize
	inside := func(p Pointer) bool {
		return p.Pool == ra.array.Pool && p.Offset >= oldStart && p.Offset < oldEnd
	}
	for i, fr := range c.Fragments {
		if i != ra.levels && inside(fr.Struct) {
			return fmt.Errorf("%s: something else points into the research array (%w), it cannot move", name, errPointer(fr.Link))
		}
	}
	sorted := c.fragmentsSorted()
	g := f.grow(ra.located.Archive, c, int(ra.array.Pool))
	newArray := make([]byte, (int(ra.count)+len(items))*researchEntrySize)
	copy(newArray, c.PoolData[ra.array.Pool][oldStart:oldEnd])
	newStart := g.appendStruct(newArray)
	for i, fr := range c.Fragments {
		if inside(fr.Link) {
			c.Fragments[i].Link.Offset = fr.Link.Offset - oldStart + newStart
		}
	}
	for i, item := range items {
		at := g.appendString(item)
		link := Pointer{Pool: ra.array.Pool, Offset: newStart + uint32(int(ra.count)+i)*researchEntrySize}
		c.Fragments = append(c.Fragments, Fragment{Link: link, Struct: Pointer{Pool: ra.array.Pool, Offset: at}})
	}
	c.Fragments[ra.levels].Struct.Offset = newStart
	rootBytes, err := ra.located.RootBytes()
	if err != nil {
		return err
	}
	binary.LittleEndian.PutUint64(rootBytes[8:], ra.count+uint64(len(items)))
	if err := g.commit(); err != nil {
		return err
	}
	if sorted {
		slices.SortFunc(c.Fragments, compareFragments)
	}
	return nil
}
