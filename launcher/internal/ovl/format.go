// Package ovl reads and writes Cobra engine OVL archives, the container
// Planet Zoo keeps its game data in. It covers the Planet Zoo 1.6+ layout
// (version 20, no DJB ids) and nothing else: anything older or from another
// game is refused when the file is opened.
//
// The model keeps every pool as opaque bytes plus the fragment table that
// says where its pointers go, which is enough to replace a file's buffers,
// patch a struct in place, grow a pool, or rename a file, and to write back
// everything else byte for byte. The layout follows cobra-tools'
// source/formats/ovl/ovl.xml.
package ovl

// Planet Zoo 1.6 and later.
const (
	formatVersion = 20
	magic         = "FRES"
)

// Compression is the codec every archive of one OVL is stored with.
type Compression uint8

const (
	None  Compression = 0
	Zlib  Compression = 1
	Oodle Compression = 4
)

// userVersion is a bitfield; bit 14 set would mean DJB ids (JWE), which this
// package does not read. Planet Zoo stores 8212, 8340 or 8724.
type userVersion uint32

const (
	compressionShift      = 7
	compressionMask       = 0b111 << compressionShift
	useDJBBit             = 1 << 14
	planetZooUserVersion  = 0x2014
	planetZooVersionFlags = 0
)

func (u userVersion) compression() Compression {
	return Compression((u & compressionMask) >> compressionShift)
}

func (u userVersion) withCompression(c Compression) userVersion {
	return u&^compressionMask | userVersion(c)<<compressionShift
}

type genericHeader struct {
	Magic       [4]byte
	VersionFlag uint8
	Version     uint8
	Bitswap     uint8
	Seventh     uint8
	UserVersion userVersion
}

type headerCounts struct {
	NumOVSTypes     uint32
	LenNames        uint32
	Zero2           uint32
	NumAuxEntries   uint32
	NumIncludedOVLs uint16
	NumMimes        uint16
	NumFiles        uint32
	NumFiles2       uint32
	NumDependencies uint32
	NumArchives     uint32
	NumPoolGroups   uint32
	NumPools        uint32
	NumDatas        uint32
	NumBuffers      uint32
	NumStreamFiles  uint32
	ZTUAC           [3]uint32
	LenArchiveNames uint32
	NumFiles3       uint32
	LenTypeNames    uint32
	NumTriplets     uint32
	Reserved        [12]uint32
}

type Mime struct {
	Name            uint32
	Zero0           uint32
	Hash            uint32
	Version         uint32
	FileIndexOffset uint32
	FileCount       uint32
	TripletCount    uint32
	TripletOffset   uint32
}

type Triplet [3]uint8

type FileEntry struct {
	Basename    uint32
	FileHash    uint32
	PoolType    int8
	SetPoolType int8
	Extension   uint16
}

type ArchiveEntry struct {
	Name              uint32
	PoolsOffset       uint32
	StreamFilesOffset uint32
	NumPools          uint32
	NumDatas          uint16
	NumPoolGroups     uint16
	NumBufferGroups   uint32
	NumBuffers        uint32
	NumFragments      uint32
	NumRootEntries    uint32
	ReadStart         uint32
	SetDataSize       uint32
	CompressedSize    uint32
	UncompressedSize  uint64
	PoolsStart        uint32
	PoolsEnd          uint32
	OVSOffset         uint32
}

// Pointer addresses a byte in a pool: the pool's index in its archive (or in
// the OVL's flattened list, for dependencies) and the offset inside it.
type Pointer struct {
	Pool   int32
	Offset uint32
}

type Dependency struct {
	FileHash  uint32
	ExtRaw    uint32
	FileIndex uint32
	Link      Pointer
}

type AuxEntry struct {
	FileIndex uint32
	Basename  uint32
	Size      uint32
}

// StreamEntry offsets are absolute in the flattened pools of all archives.
type StreamEntry struct {
	StreamOffset uint32
	FileOffset   uint32
	Zero         uint32
}

type ArchiveMeta struct {
	Unk0 uint32
	Unk1 uint32
}

type PoolGroup struct {
	Type     uint16
	NumPools uint16
}

// MemPool.Offset is relative to the start of the pool block of the whole
// OVL, not of its archive.
type MemPool struct {
	Zero1    uint64
	Size     uint32
	Offset   uint32
	FileHash uint32
	NumFiles uint16
	NumDatas uint16
	ExtHash  uint32
	Zero3    uint32
}

type DataEntry struct {
	FileHash    uint32
	ExtHash     uint32
	SetIndex    uint16
	BufferCount uint16
	Zero        uint32
	Size1       uint64
	Size2       uint64
}

type BufferEntry struct {
	Size     uint32
	FileHash uint32
}

type BufferGroup struct {
	BufferOffset uint32
	BufferCount  uint32
	ExtIndex     uint32
	BufferIndex  uint32
	Size         uint64
	DataOffset   uint32
	DataCount    uint32
}

type RootEntry struct {
	FileHash uint32
	ExtHash  uint32
	Struct   Pointer
}

// Fragment says that the 8 bytes at Link hold a pointer to Struct.
type Fragment struct {
	Link   Pointer
	Struct Pointer
}

type setHeaderCounts struct {
	SetCount   uint32
	AssetCount uint32
	SigA       uint32
	SigB       uint32
}

const (
	setSigA = 1065336831
	setSigB = 16909320
)

type SetEntry struct {
	FileHash uint32
	ExtHash  uint32
	Start    uint32
}

type AssetEntry struct {
	FileHash  uint64
	ExtHash   uint64
	RootIndex uint64
}
