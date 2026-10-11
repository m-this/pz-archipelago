package ovl

import (
	"cmp"
	"encoding/binary"
	"fmt"
	"slices"
	"strings"
)

// mimeSpec is what an OVL declares for an extension, from cobra-tools'
// constants/Planet Zoo/mimes.py.
type mimeSpec struct {
	ext      string
	name     string
	hash     uint32
	version  uint32
	triplets []Triplet
	poolType uint16
}

var (
	mimeLua = mimeSpec{
		ext: "lua", name: "FGDK:LuaModule:lua", hash: 1779074288, version: 7,
		triplets: []Triplet{{2, 2, 0}}, poolType: 2,
	}
	mimeTxt = mimeSpec{
		ext: "txt", name: "FGDK:Text:txt", hash: 640591495, version: 3, poolType: 1,
	}
	mimeResearchStart = mimeSpec{
		ext: "animalresearchstartunlockedsettings", name: "Game:AnimalResearchStartUnlockedSettings:animalresearchstartunlockedsettings",
		hash: 2180070180, version: 1, triplets: []Triplet{{1, 2, 3}}, poolType: 4,
	}
)

// Builder makes a new single-archive OVL, the way cobra-tools' "new" does,
// for the three kinds of file the AP content pack holds.
type Builder struct {
	entries []entry
}

type entry struct {
	basename string
	mime     mimeSpec
	data     []byte
}

// AddLua adds a Lua module; name is the basename without .lua.
func (b *Builder) AddLua(basename string, source []byte) {
	b.entries = append(b.entries, entry{basename: basename, mime: mimeLua, data: source})
}

// AddText adds a localised string.
func (b *Builder) AddText(basename, text string) {
	b.entries = append(b.entries, entry{basename: basename, mime: mimeTxt, data: []byte(text)})
}

// AddEmptyResearchStart adds an animal research start config that unlocks
// nothing, so the scenario's animal research starts at level 0.
func (b *Builder) AddEmptyResearchStart(basename string) {
	b.entries = append(b.entries, entry{basename: basename, mime: mimeResearchStart})
}

// built is one file's place once laid out.
type built struct {
	entry
	index int
	pool  int
	root  uint32
}

// Build lays the files out and returns the OVL, zlib-compressed as cobra
// writes new Planet Zoo files.
func (b *Builder) Build() (*File, error) {
	if len(b.entries) == 0 {
		return nil, fmt.Errorf("an ovl needs at least one file")
	}
	f := &File{header: genericHeader{Version: formatVersion, Seventh: 1, UserVersion: userVersion(planetZooUserVersion).withCompression(Zlib)}}
	copy(f.header.Magic[:], magic)
	files := f.declareFiles(b.entries)
	c, err := layOutContent(files)
	if err != nil {
		return nil, err
	}
	f.ArchiveNames = padded([]byte(staticArchive + "\x00"))
	a := &Archive{content: c, dirty: true}
	a.Entry.PoolsEnd = c.poolBytes()
	f.Archives = []*Archive{a}
	f.ArchivesMeta = []ArchiveMeta{{Unk0: 68}}
	f.counts.NumFiles2 = uint32(len(f.Files))
	f.counts.NumFiles3 = uint32(len(f.Files))
	f.counts.NumPoolGroups = uint32(len(c.PoolGroups))
	f.counts.NumPools = uint32(len(c.Pools))
	f.counts.NumDatas = uint32(len(c.DataEntries))
	f.counts.NumBuffers = uint32(len(c.BufferEntries))
	return f, nil
}

func padded(b []byte) []byte {
	return append(b, make([]byte, padTo(len(b), 8))...)
}

// declareFiles writes the names, mimes, triplets and file table, and returns
// the files in file-table order: by extension, then by hash.
func (f *File) declareFiles(entries []entry) []built {
	mimes := map[string]mimeSpec{}
	for _, e := range entries {
		mimes[e.mime.ext] = e.mime
	}
	exts := slices.Sorted(func(yield func(string) bool) {
		for ext := range mimes {
			if !yield(ext) {
				return
			}
		}
	})
	mimeNames := make([]string, 0, len(exts))
	for _, ext := range exts {
		mimeNames = append(mimeNames, mimes[ext].name)
	}
	basenames := make([]string, 0, len(entries))
	for _, e := range entries {
		basenames = append(basenames, e.basename)
	}
	slices.Sort(mimeNames)
	slices.Sort(basenames)
	offsets := map[string]uint32{}
	for _, s := range append(mimeNames, basenames...) {
		offsets[s] = uint32(len(f.Names))
		f.Names = append(append(f.Names, s...), 0)
	}
	f.Names = padded(f.Names)

	var files []built
	for i, ext := range exts {
		m := mimes[ext]
		var ofMime []built
		for _, e := range entries {
			if e.mime.ext == ext {
				ofMime = append(ofMime, built{entry: e})
			}
		}
		slices.SortFunc(ofMime, func(a, b built) int { return cmp.Compare(djb2(a.basename), djb2(b.basename)) })
		f.Mimes = append(f.Mimes, Mime{
			Name: offsets[m.name], Hash: m.hash, Version: m.version,
			FileIndexOffset: uint32(len(files)), FileCount: uint32(len(ofMime)),
			TripletCount: uint32(len(m.triplets)), TripletOffset: uint32(len(f.Triplets)),
		})
		f.Triplets = append(f.Triplets, m.triplets...)
		for _, file := range ofMime {
			file.index = len(files)
			files = append(files, file)
			f.Files = append(f.Files, FileEntry{
				Basename: offsets[file.basename], FileHash: djb2(file.basename),
				PoolType: int8(m.poolType), Extension: uint16(i),
			})
		}
	}
	f.counts.LenTypeNames = f.minBasename()
	return files
}

func (c *Content) poolBytes() uint32 {
	var total uint32
	for _, p := range c.Pools {
		total += p.Size
	}
	return total
}

// layOutContent writes every root struct into the pool of its type, in name
// order, and builds the tables that describe them.
func layOutContent(files []built) (*Content, error) {
	c := &Content{}
	types := map[uint16]bool{}
	for _, file := range files {
		types[file.mime.poolType] = true
	}
	poolOf := map[uint16]int{}
	for _, t := range slices.Sorted(func(yield func(uint16) bool) {
		for t := range types {
			if !yield(t) {
				return
			}
		}
	}) {
		poolOf[t] = len(c.Pools)
		c.PoolGroups = append(c.PoolGroups, PoolGroup{Type: t, NumPools: 1})
		c.Pools = append(c.Pools, MemPool{})
		c.PoolData = append(c.PoolData, nil)
	}
	placed := slices.Clone(files)
	slices.SortFunc(placed, func(a, b built) int { return strings.Compare(a.basename+"."+a.mime.ext, b.basename+"."+b.mime.ext) })
	for i := range placed {
		placed[i].pool = poolOf[placed[i].mime.poolType]
		if err := c.writeRoot(&placed[i]); err != nil {
			return nil, err
		}
	}
	c.finishPools(placed)
	c.describe(files, placed)
	return c, nil
}

func (c *Content) writeRoot(file *built) error {
	g := &grower{content: c, pool: file.pool}
	pool := Pointer{Pool: int32(file.pool)}
	switch file.mime.ext {
	case mimeLua.ext:
		root := make([]byte, luaRootSize)
		binary.LittleEndian.PutUint32(root, uint32(len(file.data)))
		file.root = g.appendStruct(root)
		name := g.appendString(file.basename)
		// cobra-tools keeps at least one byte behind this pointer.
		align := g.appendStruct(make([]byte, 1+padTo(len(file.basename)+2, 4)))
		c.Fragments = append(c.Fragments,
			Fragment{Link: at(pool, file.root+16), Struct: at(pool, name)},
			Fragment{Link: at(pool, file.root+24), Struct: at(pool, align)})
	case mimeTxt.ext:
		text := binary.LittleEndian.AppendUint32(nil, uint32(len(file.data)))
		text = append(append(text, file.data...), 0)
		text = append(text, make([]byte, padTo(len(text), 8))...)
		file.root = g.appendStruct(text)
		c.Pools[file.pool].NumFiles++
	case mimeResearchStart.ext:
		file.root = g.appendStruct(make([]byte, researchRootSize))
	default:
		return fmt.Errorf("%s: no builder for .%s", file.basename, file.mime.ext)
	}
	return nil
}

func at(pool Pointer, offset uint32) Pointer {
	return Pointer{Pool: pool.Pool, Offset: offset}
}

// finishPools pads every pool, points empty arrays at the end of theirs,
// places the pools in the block and names each after its first file.
func (c *Content) finishPools(placed []built) {
	for i := range c.PoolData {
		c.PoolData[i] = append(c.PoolData[i], make([]byte, padTo(len(c.PoolData[i]), poolAlignment))...)
		c.Pools[i].Size = uint32(len(c.PoolData[i]))
	}
	for _, file := range placed {
		if file.mime.ext == mimeResearchStart.ext {
			pool := Pointer{Pool: int32(file.pool)}
			c.Fragments = append(c.Fragments, Fragment{Link: at(pool, file.root), Struct: at(pool, c.Pools[file.pool].Size)})
		}
	}
	var offset uint32
	for i := range c.Pools {
		c.Pools[i].Offset = offset
		offset += c.Pools[i].Size
		first := slices.IndexFunc(placed, func(b built) bool { return b.pool == i })
		c.Pools[i].FileHash = uint32(placed[first].index)
	}
	slices.SortFunc(c.Fragments, compareFragments)
}

// describe writes the root, data and buffer tables in file-table order.
func (c *Content) describe(files, placed []built) {
	roots := map[int]built{}
	for _, file := range placed {
		roots[file.index] = file
	}
	for _, file := range files {
		p := roots[file.index]
		c.RootEntries = append(c.RootEntries, RootEntry{
			FileHash: uint32(file.index), ExtHash: djb2(file.mime.ext),
			Struct: Pointer{Pool: int32(p.pool), Offset: p.root},
		})
		if file.mime.ext != mimeLua.ext {
			continue
		}
		c.DataEntries = append(c.DataEntries, DataEntry{
			FileHash: uint32(file.index), ExtHash: djb2(file.mime.ext), BufferCount: 1, Size1: uint64(len(file.data)),
		})
		c.BufferEntries = append(c.BufferEntries, BufferEntry{Size: uint32(len(file.data)), FileHash: uint32(file.index)})
		c.BufferData = append(c.BufferData, file.data)
	}
	if len(c.BufferEntries) > 0 {
		var size uint64
		for _, b := range c.BufferEntries {
			size += uint64(b.Size)
		}
		lua := slices.IndexFunc(files, func(b built) bool { return b.mime.ext == mimeLua.ext })
		c.BufferGroups = []BufferGroup{{
			BufferCount: uint32(len(c.BufferEntries)), ExtIndex: uint32(files[lua].mimeIndex(files)),
			Size: size, DataCount: uint32(len(c.DataEntries)),
		}}
	}
}

func (b built) mimeIndex(files []built) int {
	var exts []string
	for _, f := range files {
		if !slices.Contains(exts, f.mime.ext) {
			exts = append(exts, f.mime.ext)
		}
	}
	return slices.Index(exts, b.mime.ext)
}
