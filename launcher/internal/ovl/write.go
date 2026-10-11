package ovl

import "fmt"

// Write encodes the file. Untouched archives keep the compressed bytes they
// were read with. ovs is nil when nothing outside STATIC changed, so the
// caller can leave the .ovs on disk as it is.
func (f *File) Write() (ovl, ovs []byte, err error) {
	codec, err := f.codecs.get(f.Compression())
	if err != nil {
		return nil, nil, err
	}
	rewriteOVS := false
	for i, a := range f.Archives {
		if !a.dirty {
			continue
		}
		if err := f.encodeArchive(a, &f.ArchivesMeta[i], codec); err != nil {
			return nil, nil, fmt.Errorf("archive %s: %w", f.ArchiveName(a), err)
		}
		rewriteOVS = rewriteOVS || f.ArchiveName(a) != staticArchive
	}
	if rewriteOVS {
		ovs = f.layOutOVS()
	}
	return f.encodeHeader(), ovs, nil
}

func (f *File) encodeArchive(a *Archive, meta *ArchiveMeta, codec Codec) error {
	before := a.Entry.UncompressedSize
	oldSize2 := a.content.sumSize2Original
	raw := a.content.encode()
	compressed, err := codec.Compress(raw)
	if err != nil {
		return err
	}
	a.content.syncEntry(&a.Entry, len(raw))
	if f.Compression() == None {
		a.Entry.UncompressedSize = 0
	}
	a.Entry.CompressedSize = uint32(len(compressed))
	a.compressed = compressed
	// Keep whatever relation the vanilla meta had to the sizes, shifted by
	// the change.
	meta.Unk0 = uint32(int64(meta.Unk0) + int64(a.Entry.UncompressedSize) - int64(before))
	meta.Unk1 = uint32(int64(meta.Unk1) + int64(a.content.sumSize2()) - int64(oldSize2))
	a.content.sumSize2Original = a.content.sumSize2()
	a.dirty = false
	return nil
}

// layOutOVS places every archive outside STATIC one after another, in
// archive order, the way cobra-tools writes them.
func (f *File) layOutOVS() []byte {
	var out []byte
	for _, a := range f.Archives {
		if f.ArchiveName(a) == staticArchive {
			continue
		}
		a.Entry.ReadStart = uint32(len(out))
		out = append(out, a.compressed...)
	}
	return out
}

func (f *File) encodeHeader() []byte {
	var static []byte
	entries := make([]ArchiveEntry, 0, len(f.Archives))
	for _, a := range f.Archives {
		entries = append(entries, a.Entry)
		if f.ArchiveName(a) == staticArchive {
			static = a.compressed
		}
	}
	c := f.counts
	c.LenNames = uint32(len(f.Names))
	c.NumMimes = uint16(len(f.Mimes))
	c.NumTriplets = uint32(len(f.Triplets))
	c.NumFiles = uint32(len(f.Files))
	c.LenArchiveNames = uint32(len(f.ArchiveNames))
	c.NumArchives = uint32(len(f.Archives))
	c.NumIncludedOVLs = uint16(len(f.IncludedOVLs))
	c.NumDependencies = uint32(len(f.Dependencies))
	c.NumAuxEntries = uint32(len(f.AuxEntries))
	c.NumStreamFiles = uint32(len(f.StreamFiles))
	f.counts = c

	var w writer
	w.write(f.header)
	w.write(c)
	w.Write(f.Names)
	w.write(f.Mimes)
	w.write(f.Triplets)
	w.Write(make([]byte, tripletPadding(len(f.Triplets))))
	w.write(f.Files)
	w.Write(f.ArchiveNames)
	w.write(entries)
	w.write(f.IncludedOVLs)
	w.write(f.Dependencies)
	w.write(f.AuxEntries)
	w.write(f.StreamFiles)
	w.write(f.ArchivesMeta)
	w.Write(static)
	return w.Bytes()
}
