package ovl

import (
	"fmt"
	"slices"
)

// Appending to a pool follows cobra-tools: structs start on 16 bytes,
// strings anywhere, and the pool ends on 4.
const (
	structAlignment = 16
	poolAlignment   = 4
)

func padTo(n, alignment int) int {
	return (alignment - n%alignment) % alignment
}

// grower collects what is appended to one pool of one archive, then commits
// the growth to every offset that depends on the pool's size.
type grower struct {
	file    *File
	archive *Archive
	content *Content
	pool    int
	oldSize uint32
	// atEnd are the fragments that pointed at the end of the pool before
	// anything was appended: empty arrays, which must keep pointing at the end.
	atEnd []int
}

func (f *File) grow(a *Archive, c *Content, pool int) *grower {
	g := &grower{file: f, archive: a, content: c, pool: pool, oldSize: c.Pools[pool].Size}
	for i, fr := range c.Fragments {
		if int(fr.Struct.Pool) == pool && fr.Struct.Offset == g.oldSize {
			g.atEnd = append(g.atEnd, i)
		}
	}
	return g
}

// appendStruct places bytes at the next 16-byte boundary and returns their offset.
func (g *grower) appendStruct(b []byte) uint32 {
	data := g.content.PoolData[g.pool]
	data = append(data, make([]byte, padTo(len(data), structAlignment))...)
	at := uint32(len(data))
	g.content.PoolData[g.pool] = append(data, b...)
	return at
}

// appendString places a zero-terminated string and returns its offset.
func (g *grower) appendString(s string) uint32 {
	data := g.content.PoolData[g.pool]
	at := uint32(len(data))
	g.content.PoolData[g.pool] = append(append(data, s...), 0)
	return at
}

// commit pads the pool and moves everything the growth displaces: pointers
// to the old end of the pool, the pools after it in the flattened pool
// block, and the stream entries that address them.
func (g *grower) commit() error {
	data := g.content.PoolData[g.pool]
	data = append(data, make([]byte, padTo(len(data), poolAlignment))...)
	g.content.PoolData[g.pool] = data
	delta := uint32(len(data)) - g.oldSize
	pool := &g.content.Pools[g.pool]
	pool.Size = uint32(len(data))

	for _, i := range g.atEnd {
		g.content.Fragments[i].Struct.Offset = pool.Size
	}
	boundary := pool.Offset + g.oldSize
	if err := g.file.shiftPoolsAfter(boundary, delta, g.archive, g.pool); err != nil {
		return err
	}
	for i, s := range g.file.StreamFiles {
		if s.StreamOffset >= boundary {
			g.file.StreamFiles[i].StreamOffset += delta
		}
		if s.FileOffset >= boundary {
			g.file.StreamFiles[i].FileOffset += delta
		}
	}
	g.archive.Touch()
	return nil
}

// shiftPoolsAfter moves every pool that starts at or after boundary in the
// flattened pool block, in every archive, and the archives' pool ranges.
func (f *File) shiftPoolsAfter(boundary, delta uint32, grown *Archive, grownPool int) error {
	for _, a := range f.Archives {
		if a.Entry.PoolsEnd < boundary && a != grown {
			continue
		}
		if a != grown && a.Entry.PoolsStart >= boundary {
			a.Entry.PoolsStart += delta
		}
		if a.Entry.PoolsEnd >= boundary {
			a.Entry.PoolsEnd += delta
		}
		if a.Entry.NumPools == 0 {
			continue
		}
		c, err := f.Content(a)
		if err != nil {
			return err
		}
		moved := false
		for i := range c.Pools {
			if a == grown && i == grownPool {
				continue
			}
			if c.Pools[i].Offset >= boundary {
				c.Pools[i].Offset += delta
				moved = true
			}
		}
		if moved {
			a.Touch()
		}
	}
	return nil
}

// fragmentAt finds the fragment whose pointer sits at link.
func (c *Content) fragmentAt(link Pointer) (int, bool) {
	i := slices.IndexFunc(c.Fragments, func(fr Fragment) bool { return fr.Link == link })
	return i, i >= 0
}

// fragmentsSorted reports whether the fragments follow cobra-tools' order,
// so an edit can keep that order when the vanilla file had it.
func (c *Content) fragmentsSorted() bool {
	return slices.IsSortedFunc(c.Fragments, compareFragments)
}

func compareFragments(a, b Fragment) int {
	keys := [4][2]int64{
		{int64(a.Link.Pool), int64(b.Link.Pool)},
		{int64(a.Struct.Pool), int64(b.Struct.Pool)},
		{int64(a.Link.Offset), int64(b.Link.Offset)},
		{int64(a.Struct.Offset), int64(b.Struct.Offset)},
	}
	for _, k := range keys {
		if k[0] != k[1] {
			if k[0] < k[1] {
				return -1
			}
			return 1
		}
	}
	return 0
}

func errPointer(p Pointer) error {
	return fmt.Errorf("pool %d offset %d", p.Pool, p.Offset)
}
