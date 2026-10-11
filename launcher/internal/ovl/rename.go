package ovl

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

// Rename gives a file a new basename and keeps its extension. The files of
// one extension are kept sorted by hash, as the game finds them that way,
// so the rename can move the file; every index into the file table in every
// archive follows it.
func (f *File) Rename(oldName, newName string) error {
	index, err := f.FileIndex(oldName)
	if err != nil {
		return err
	}
	ext := "." + f.mimeExtension(int(f.Files[index].Extension))
	base, ok := strings.CutSuffix(newName, ext)
	if !ok {
		return fmt.Errorf("rename %s to %s: the extension must stay %s", oldName, newName, ext)
	}
	if _, err := f.FileIndex(newName); err == nil {
		return fmt.Errorf("rename %s: %s already exists", oldName, newName)
	}
	f.Files[index].Basename = f.addName(base)
	f.Files[index].FileHash = djb2(base)
	f.counts.LenTypeNames = f.minBasename()

	perm := f.sortMimeByHash(int(f.Files[index].Extension))
	return f.remapFileIndices(perm)
}

// addName appends a string to the names buffer, which stays padded to 8.
// The old name is left where it is, unused.
func (f *File) addName(s string) uint32 {
	at := uint32(len(f.Names))
	f.Names = append(append(f.Names, s...), 0)
	f.Names = append(f.Names, make([]byte, padTo(len(f.Names), 8))...)
	return at
}

func (f *File) minBasename() uint32 {
	least := f.Files[0].Basename
	for _, e := range f.Files {
		least = min(least, e.Basename)
	}
	return least
}

// sortMimeByHash re-sorts one extension's files by hash and returns, for
// every file index, where that file now is.
func (f *File) sortMimeByHash(mime int) []uint32 {
	perm := make([]uint32, len(f.Files))
	for i := range perm {
		perm[i] = uint32(i)
	}
	m := f.Mimes[mime]
	lo, hi := int(m.FileIndexOffset), int(m.FileIndexOffset+m.FileCount)
	order := make([]int, hi-lo)
	for i := range order {
		order[i] = lo + i
	}
	slices.SortStableFunc(order, func(a, b int) int { return cmp.Compare(f.Files[a].FileHash, f.Files[b].FileHash) })
	sorted := make([]FileEntry, len(order))
	for to, from := range order {
		sorted[to] = f.Files[from]
		perm[from] = uint32(lo + to)
	}
	copy(f.Files[lo:hi], sorted)
	return perm
}

func identity(perm []uint32) bool {
	for i, p := range perm {
		if uint32(i) != p {
			return false
		}
	}
	return true
}

func (f *File) remapFileIndices(perm []uint32) error {
	if identity(perm) {
		return nil
	}
	for i := range f.Dependencies {
		f.Dependencies[i].FileIndex = perm[f.Dependencies[i].FileIndex]
	}
	for i := range f.AuxEntries {
		f.AuxEntries[i].FileIndex = perm[f.AuxEntries[i].FileIndex]
	}
	for _, a := range f.Archives {
		c, err := f.Content(a)
		if err != nil {
			return err
		}
		c.remap(perm)
		a.Touch()
	}
	return nil
}

// remap rewrites every file index in the archive and restores each order
// cobra-tools keeps, where the archive kept it before.
func (c *Content) remap(perm []uint32) {
	rootsSorted := slices.IsSortedFunc(c.RootEntries, func(a, b RootEntry) int { return cmp.Compare(a.FileHash, b.FileHash) })
	datasSorted := slices.IsSortedFunc(c.DataEntries, func(a, b DataEntry) int { return cmp.Compare(a.FileHash, b.FileHash) })
	buffersSorted := c.buffersSortedInGroups()

	for i := range c.RootEntries {
		c.RootEntries[i].FileHash = perm[c.RootEntries[i].FileHash]
	}
	for i := range c.DataEntries {
		c.DataEntries[i].FileHash = perm[c.DataEntries[i].FileHash]
	}
	for i := range c.BufferEntries {
		c.BufferEntries[i].FileHash = perm[c.BufferEntries[i].FileHash]
	}
	for i := range c.Pools {
		if int(c.Pools[i].FileHash) < len(perm) {
			c.Pools[i].FileHash = perm[c.Pools[i].FileHash]
		}
	}
	for i := range c.Sets {
		c.Sets[i].FileHash = perm[c.Sets[i].FileHash]
	}
	for i := range c.Assets {
		c.Assets[i].FileHash = uint64(perm[c.Assets[i].FileHash])
	}

	if rootsSorted {
		c.sortRoots()
	}
	if datasSorted {
		slices.SortStableFunc(c.DataEntries, func(a, b DataEntry) int { return cmp.Compare(a.FileHash, b.FileHash) })
	}
	if buffersSorted {
		c.sortBuffersInGroups()
	}
}

// sortRoots sorts root entries by file index and points the assets at
// their roots' new places.
func (c *Content) sortRoots() {
	order := make([]int, len(c.RootEntries))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return cmp.Compare(c.RootEntries[a].FileHash, c.RootEntries[b].FileHash) })
	moved := make([]uint64, len(order))
	roots := make([]RootEntry, len(order))
	for to, from := range order {
		roots[to] = c.RootEntries[from]
		moved[from] = uint64(to)
	}
	c.RootEntries = roots
	for i := range c.Assets {
		c.Assets[i].RootIndex = moved[c.Assets[i].RootIndex]
	}
}

func (c *Content) buffersSortedInGroups() bool {
	for _, g := range c.BufferGroups {
		group := c.BufferEntries[g.BufferOffset : g.BufferOffset+g.BufferCount]
		if !slices.IsSortedFunc(group, func(a, b BufferEntry) int { return cmp.Compare(a.FileHash, b.FileHash) }) {
			return false
		}
	}
	return true
}

// sortBuffersInGroups sorts each buffer group by file index, carrying the
// buffers' bytes along.
func (c *Content) sortBuffersInGroups() {
	for _, g := range c.BufferGroups {
		lo, hi := int(g.BufferOffset), int(g.BufferOffset+g.BufferCount)
		order := make([]int, hi-lo)
		for i := range order {
			order[i] = lo + i
		}
		slices.SortStableFunc(order, func(a, b int) int {
			return cmp.Compare(c.BufferEntries[a].FileHash, c.BufferEntries[b].FileHash)
		})
		entries := make([]BufferEntry, len(order))
		data := make([][]byte, len(order))
		for to, from := range order {
			entries[to], data[to] = c.BufferEntries[from], c.BufferData[from]
		}
		copy(c.BufferEntries[lo:hi], entries)
		copy(c.BufferData[lo:hi], data)
	}
}
