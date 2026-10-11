package ovl

import (
	"fmt"
	"strings"
)

// djb2 is the hash Cobra files are looked up by: of the basename, without
// its extension.
func djb2(s string) uint32 {
	h := uint32(5381)
	for i := range len(s) {
		h = h*33 + uint32(s[i])
	}
	return h
}

// FileName is a file's name with its extension, as cobra-tools prints it.
func (f *File) FileName(index int) string {
	e := f.Files[index]
	return f.Name(e.Basename) + "." + f.mimeExtension(int(e.Extension))
}

func (f *File) mimeExtension(mime int) string {
	name := f.Name(f.Mimes[mime].Name)
	return name[strings.LastIndexByte(name, ':')+1:]
}

// FileIndex finds a file by its full name, extension included. In Planet Zoo
// ovls an archive refers to a file by this index, not by a hash.
func (f *File) FileIndex(name string) (int, error) {
	for i := range f.Files {
		if f.FileName(i) == name {
			return i, nil
		}
	}
	return 0, fmt.Errorf("no file %q in the ovl", name)
}

// Located is where a file's root struct lives.
type Located struct {
	Archive *Archive
	Content *Content
	Root    int
}

// Locate finds the archive holding a file's root entry and decodes it.
func (f *File) Locate(name string) (Located, error) {
	index, err := f.FileIndex(name)
	if err != nil {
		return Located{}, err
	}
	for _, a := range f.Archives {
		content, err := f.Content(a)
		if err != nil {
			return Located{}, err
		}
		for i, root := range content.RootEntries {
			if int(root.FileHash) == index {
				return Located{Archive: a, Content: content, Root: i}, nil
			}
		}
	}
	return Located{}, fmt.Errorf("file %q has no root entry in any archive", name)
}

// RootBytes is the file's root struct, from its offset to the end of its
// pool. Writes through it change the pool; Touch the archive afterwards.
func (l Located) RootBytes() ([]byte, error) {
	ptr := l.Content.RootEntries[l.Root].Struct
	if ptr.Pool < 0 || int(ptr.Pool) >= len(l.Content.PoolData) {
		return nil, fmt.Errorf("root entry points at pool %d of %d", ptr.Pool, len(l.Content.PoolData))
	}
	pool := l.Content.PoolData[ptr.Pool]
	if int(ptr.Offset) > len(pool) {
		return nil, fmt.Errorf("root entry points past the end of its pool")
	}
	return pool[ptr.Offset:], nil
}

// fileID is how the archive refers to the file: its index.
func (l Located) fileID() uint32 { return l.Content.RootEntries[l.Root].FileHash }

// Buffers returns the file's buffers in buffer-index order.
func (l Located) Buffers() ([][]byte, error) {
	indices, err := l.bufferPositions()
	if err != nil {
		return nil, err
	}
	out := make([][]byte, len(indices))
	for i, at := range indices {
		out[i] = l.Content.BufferData[at]
	}
	return out, nil
}

// bufferPositions lists where in BufferEntries each of the file's buffers
// sits, ordered by buffer index, which the buffer groups assign.
func (l Located) bufferPositions() ([]int, error) {
	id := l.fileID()
	var positions []int
	for _, g := range l.Content.BufferGroups {
		for at := g.BufferOffset; at < g.BufferOffset+g.BufferCount; at++ {
			if l.Content.BufferEntries[at].FileHash != id {
				continue
			}
			if int(g.BufferIndex) != len(positions) {
				return nil, fmt.Errorf("buffer %d of file %d comes before buffer %d", g.BufferIndex, id, len(positions))
			}
			positions = append(positions, int(at))
		}
	}
	data, err := l.dataEntry()
	if err != nil {
		return nil, err
	}
	if len(positions) != int(l.Content.DataEntries[data].BufferCount) {
		return nil, fmt.Errorf("file %d: %d buffers found, its data entry says %d", id, len(positions), l.Content.DataEntries[data].BufferCount)
	}
	return positions, nil
}

func (l Located) dataEntry() (int, error) {
	for i, d := range l.Content.DataEntries {
		if d.FileHash == l.fileID() {
			return i, nil
		}
	}
	return 0, fmt.Errorf("file %d has no data entry", l.fileID())
}

// ReplaceBuffers swaps the file's buffers for new ones of any size and
// updates every size that counts them. The buffer count cannot change.
func (l Located) ReplaceBuffers(buffers [][]byte) error {
	positions, err := l.bufferPositions()
	if err != nil {
		return err
	}
	if len(buffers) != len(positions) {
		return fmt.Errorf("%d buffers given, the file has %d", len(buffers), len(positions))
	}
	data, err := l.dataEntry()
	if err != nil {
		return err
	}
	c := l.Content
	sizes := make([]uint64, len(buffers))
	for i, at := range positions {
		sizes[i] = uint64(c.BufferEntries[at].Size)
	}
	if err := checkDataSizes(c.DataEntries[data], sizes); err != nil {
		return fmt.Errorf("file %d: %w", l.fileID(), err)
	}
	for i, at := range positions {
		sizes[i] = uint64(len(buffers[i]))
		c.BufferEntries[at].Size = uint32(len(buffers[i]))
		c.BufferData[at] = buffers[i]
	}
	c.DataEntries[data].Size1, c.DataEntries[data].Size2 = dataSizes(c.DataEntries[data], sizes)
	for i, g := range c.BufferGroups {
		var total uint64
		for at := g.BufferOffset; at < g.BufferOffset+g.BufferCount; at++ {
			total += uint64(c.BufferEntries[at].Size)
		}
		c.BufferGroups[i].Size = total
	}
	l.Archive.Touch()
	return nil
}

// A data entry's two sizes split its buffers' total between them. Which
// split a file uses depends on its format, so the one it has is kept: all
// in the first, all in the second, or the first two buffers against the rest.
func dataSizes(d DataEntry, sizes []uint64) (size1, size2 uint64) {
	var total, firstTwo uint64
	for i, s := range sizes {
		total += s
		if i < 2 {
			firstTwo += s
		}
	}
	switch {
	case d.Size2 == 0:
		return total, 0
	case d.Size1 == 0:
		return 0, total
	default:
		return firstTwo, total - firstTwo
	}
}

func checkDataSizes(d DataEntry, sizes []uint64) error {
	size1, size2 := dataSizes(d, sizes)
	if size1 != d.Size1 || size2 != d.Size2 {
		return fmt.Errorf("data entry sizes %d+%d do not split its buffers %v in any known way", d.Size1, d.Size2, sizes)
	}
	return nil
}
