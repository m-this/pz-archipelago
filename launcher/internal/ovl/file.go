package ovl

import (
	"bytes"
	"errors"
	"fmt"
)

// File is one OVL with its archives. The STATIC archive is stored after the
// header in the .ovl; the others live in the sibling .ovs.
type File struct {
	header genericHeader
	counts headerCounts

	Names        []byte
	Mimes        []Mime
	Triplets     []Triplet
	Files        []FileEntry
	ArchiveNames []byte
	Archives     []*Archive
	IncludedOVLs []uint32
	Dependencies []Dependency
	AuxEntries   []AuxEntry
	StreamFiles  []StreamEntry
	ArchivesMeta []ArchiveMeta

	codecs Codecs
}

// Archive keeps its compressed bytes as read. It is decoded on first use and
// re-encoded on write only once something changed it.
type Archive struct {
	Entry      ArchiveEntry
	compressed []byte
	content    *Content
	dirty      bool
}

const staticArchive = "STATIC"

// Read parses an OVL and, when it has archives outside STATIC, its .ovs.
func Read(ovl, ovs []byte, codecs Codecs) (*File, error) {
	r := &reader{data: ovl}
	f := &File{codecs: codecs}
	r.read(&f.header)
	if r.err == nil {
		if err := f.checkVersion(); err != nil {
			return nil, err
		}
	}
	r.read(&f.counts)
	f.readTables(r)
	if r.err != nil {
		return nil, fmt.Errorf("ovl header: %w", r.err)
	}
	if err := f.attachArchives(r, ovs); err != nil {
		return nil, err
	}
	return f, nil
}

func (f *File) checkVersion() error {
	h := f.header
	switch {
	case string(h.Magic[:]) == "FREA":
		return errors.New("the ovl is encrypted")
	case string(h.Magic[:]) != magic:
		return fmt.Errorf("not an ovl: magic %q", h.Magic)
	case h.Version != formatVersion || h.UserVersion&useDJBBit != 0:
		return fmt.Errorf("ovl version %d, user version %d: only Planet Zoo 1.6 and later is supported", h.Version, h.UserVersion)
	}
	return nil
}

func (f *File) readTables(r *reader) {
	c := f.counts
	f.Names = r.bytes(int(c.LenNames))
	f.Mimes = readSlice[Mime](r, int(c.NumMimes))
	f.Triplets = readSlice[Triplet](r, int(c.NumTriplets))
	r.bytes(tripletPadding(len(f.Triplets)))
	f.Files = readSlice[FileEntry](r, int(c.NumFiles))
	f.ArchiveNames = r.bytes(int(c.LenArchiveNames))
	entries := readSlice[ArchiveEntry](r, int(c.NumArchives))
	f.IncludedOVLs = readSlice[uint32](r, int(c.NumIncludedOVLs))
	f.Dependencies = readSlice[Dependency](r, int(c.NumDependencies))
	f.AuxEntries = readSlice[AuxEntry](r, int(c.NumAuxEntries))
	f.StreamFiles = readSlice[StreamEntry](r, int(c.NumStreamFiles))
	f.ArchivesMeta = readSlice[ArchiveMeta](r, int(c.NumArchives))
	for _, e := range entries {
		f.Archives = append(f.Archives, &Archive{Entry: e})
	}
}

// The triplets are padded to a multiple of four bytes.
func tripletPadding(n int) int {
	return (4 - (3*n)%4) % 4
}

func (f *File) attachArchives(r *reader, ovs []byte) error {
	for _, a := range f.Archives {
		size := int(a.Entry.CompressedSize)
		if f.ArchiveName(a) == staticArchive {
			a.compressed = r.bytes(size)
			if r.err != nil {
				return fmt.Errorf("STATIC archive: %w", r.err)
			}
			continue
		}
		start := int(a.Entry.ReadStart)
		if start+size > len(ovs) {
			return fmt.Errorf("archive %s: bytes %d to %d are past the end of the %d-byte ovs", f.ArchiveName(a), start, start+size, len(ovs))
		}
		a.compressed = ovs[start : start+size : start+size]
	}
	if r.pos != len(r.data) {
		return fmt.Errorf("%d trailing bytes after the STATIC archive", len(r.data)-r.pos)
	}
	return nil
}

// Compression is the codec every archive of the file uses.
func (f *File) Compression() Compression {
	return f.header.UserVersion.compression()
}

func (f *File) ArchiveName(a *Archive) string {
	return stringAt(f.ArchiveNames, a.Entry.Name)
}

// Name returns the string at an offset of the names buffer.
func (f *File) Name(offset uint32) string {
	return stringAt(f.Names, offset)
}

func stringAt(buffer []byte, offset uint32) string {
	if int(offset) >= len(buffer) {
		return ""
	}
	rest := buffer[offset:]
	if end := bytes.IndexByte(rest, 0); end >= 0 {
		rest = rest[:end]
	}
	return string(rest)
}

// Content decodes an archive. The result is shared: changes to it are what
// Write saves, once the archive is marked with Touch.
func (f *File) Content(a *Archive) (*Content, error) {
	if a.content != nil {
		return a.content, nil
	}
	codec, err := f.codecs.get(f.Compression())
	if err != nil {
		return nil, err
	}
	size := int(a.Entry.UncompressedSize)
	if f.Compression() == None {
		size = len(a.compressed)
	}
	raw, err := codec.Decompress(a.compressed, size)
	if err != nil {
		return nil, fmt.Errorf("archive %s: %w", f.ArchiveName(a), err)
	}
	content, err := decodeContent(raw, a.Entry)
	if err != nil {
		return nil, fmt.Errorf("archive %s: %w", f.ArchiveName(a), err)
	}
	// Everything this package writes rests on reading the archive fully. An
	// archive it cannot encode back to the same bytes has something the model
	// misses, and editing it would lose that.
	if !bytes.Equal(content.encode(), raw) {
		return nil, fmt.Errorf("archive %s: holds something this reader does not model; refusing to edit it", f.ArchiveName(a))
	}
	a.content = content
	return content, nil
}

// Touch marks an archive as changed, so Write encodes it again.
func (a *Archive) Touch() { a.dirty = true }
