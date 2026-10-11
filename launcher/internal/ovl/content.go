package ovl

import "fmt"

// Content is a decoded archive: its tables, the bytes of every pool, and the
// bytes of every buffer, in the order they are stored.
type Content struct {
	PoolGroups    []PoolGroup
	Pools         []MemPool
	PoolData      [][]byte
	DataEntries   []DataEntry
	BufferEntries []BufferEntry
	BufferData    [][]byte
	BufferGroups  []BufferGroup
	RootEntries   []RootEntry
	Fragments     []Fragment
	Sets          []SetEntry
	Assets        []AssetEntry

	sumSize2Original uint64
}

func decodeContent(raw []byte, e ArchiveEntry) (*Content, error) {
	r := &reader{data: raw}
	c := &Content{}
	c.PoolGroups = readSlice[PoolGroup](r, int(e.NumPoolGroups))
	c.Pools = readSlice[MemPool](r, int(e.NumPools))
	c.DataEntries = readSlice[DataEntry](r, int(e.NumDatas))
	c.BufferEntries = readSlice[BufferEntry](r, int(e.NumBuffers))
	c.BufferGroups = readSlice[BufferGroup](r, int(e.NumBufferGroups))
	c.RootEntries = readSlice[RootEntry](r, int(e.NumRootEntries))
	c.Fragments = readSlice[Fragment](r, int(e.NumFragments))
	var sets setHeaderCounts
	r.read(&sets)
	if r.err == nil && (sets.SigA != setSigA || sets.SigB != setSigB) {
		return nil, fmt.Errorf("set header signature %d %d", sets.SigA, sets.SigB)
	}
	c.Sets = readSlice[SetEntry](r, int(sets.SetCount))
	c.Assets = readSlice[AssetEntry](r, int(sets.AssetCount))
	if r.err == nil && uint32(c.setDataSize()) != e.SetDataSize {
		return nil, fmt.Errorf("set data is %d bytes, the archive entry says %d", c.setDataSize(), e.SetDataSize)
	}
	for _, p := range c.Pools {
		c.PoolData = append(c.PoolData, r.bytes(int(p.Size)))
	}
	for _, b := range c.BufferEntries {
		c.BufferData = append(c.BufferData, r.bytes(int(b.Size)))
	}
	if r.err != nil {
		return nil, r.err
	}
	if r.pos != len(raw) {
		return nil, fmt.Errorf("%d bytes left after the last buffer", len(raw)-r.pos)
	}
	c.sumSize2Original = c.sumSize2()
	return c, nil
}

func (c *Content) setDataSize() int {
	return 16 + 12*len(c.Sets) + 24*len(c.Assets)
}

func (c *Content) encode() []byte {
	var w writer
	w.write(c.PoolGroups)
	w.write(c.Pools)
	w.write(c.DataEntries)
	w.write(c.BufferEntries)
	w.write(c.BufferGroups)
	w.write(c.RootEntries)
	w.write(c.Fragments)
	w.write(setHeaderCounts{SetCount: uint32(len(c.Sets)), AssetCount: uint32(len(c.Assets)), SigA: setSigA, SigB: setSigB})
	w.write(c.Sets)
	w.write(c.Assets)
	for _, p := range c.PoolData {
		w.Write(p)
	}
	for _, b := range c.BufferData {
		w.Write(b)
	}
	return w.Bytes()
}

// syncEntry writes the counts and sizes of the content back into its entry.
func (c *Content) syncEntry(e *ArchiveEntry, uncompressed int) {
	e.NumPoolGroups = uint16(len(c.PoolGroups))
	e.NumPools = uint32(len(c.Pools))
	e.NumDatas = uint16(len(c.DataEntries))
	e.NumBuffers = uint32(len(c.BufferEntries))
	e.NumBufferGroups = uint32(len(c.BufferGroups))
	e.NumRootEntries = uint32(len(c.RootEntries))
	e.NumFragments = uint32(len(c.Fragments))
	e.SetDataSize = uint32(c.setDataSize())
	e.UncompressedSize = uint64(uncompressed)
}

func (c *Content) sumSize2() uint64 {
	var total uint64
	for _, d := range c.DataEntries {
		total += d.Size2
	}
	return total
}
