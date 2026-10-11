package pz

import (
	"encoding/binary"

	"github.com/m-this/pz-archipelago/launcher/internal/game/mem"
)

// An animal is a 0x3F0-byte record in its manager's table, found through an
// open-addressing hash map from its handle. Upstream's memory/animals.py.
const (
	entitySpeciesHandle = 0x50
	entityLifeStage     = 0x3A7
	lifeStageNewborn    = 0

	zooAnimalManager = 0x2F8
	mgrHashMap       = 0xB90
	mgrTable         = 0xC20
	mapCap           = 0x10
	mapBuckets       = 0x18
	recordStride     = 0x3F0

	parkAnimalManager  = 0x188
	parkExhibitManager = 0x1D0
	exhibitCensus      = 0x318
	exhibitDefHolder   = 0xF8
	exhibitIDSet       = 0x2A0
	exhibitDefSub      = 0x358
	defSubMap          = 0x108
	defEntryStride     = 0x160
	defEntryObj        = 0x8
	defEntrySpecies    = 0x30
)

var (
	exhibitHolderAlt = [2]uint64{0x2F0, 0xD8}
	defStringOffsets = []uint64{0x0, 0x8, 0x10, 0x20}
)

// Animals resolves animal handles and the exhibit structures.
type Animals struct {
	g *Game
}

// animalHash is the slot a handle starts probing at.
func animalHash(key, slots uint64) uint64 {
	h := key*0x40000 + ^key
	h = (h>>0x1F ^ h) * 0x15
	h = (h>>0x0B ^ h) * 0x41
	return (h>>0x16 ^ h) & (slots - 1)
}

type rosterMap struct {
	slots, buckets, table uint64
}

func (a *Animals) roster(mgr uint64) (rosterMap, bool) {
	if mgr == 0 {
		return rosterMap{}, false
	}
	m := rosterMap{
		slots:   mem.Ptr(a.g.proc, mgr+mgrHashMap+mapCap),
		buckets: mem.Ptr(a.g.proc, mgr+mgrHashMap+mapBuckets),
		table:   mem.Ptr(a.g.proc, mgr+mgrTable),
	}
	// A slots that is not a power of two means mgr is not a manager, which
	// is what lets a caller try a guessed pointer safely.
	if m.slots == 0 || m.buckets == 0 || m.table == 0 || m.slots > 1<<24 || !isPow2(m.slots) {
		return rosterMap{}, false
	}
	return m, true
}

func (a *Animals) slots(m rosterMap) ([]byte, uint64, bool) {
	bm := bitmapBytes(m.slots)
	data, err := a.g.proc.Read(m.buckets, int(bm+m.slots*0x10))
	return data, bm, err == nil
}

// EntityByManager resolves a handle through a roster manager.
func (a *Animals) EntityByManager(mgr, handle uint64) (uint64, bool) {
	m, ok := a.roster(mgr)
	if !ok {
		return 0, false
	}
	data, bm, ok := a.slots(m)
	if !ok {
		return 0, false
	}
	start := animalHash(handle, m.slots)
	for i, n := start, uint64(0); n < m.slots; i, n = (i+1)%m.slots, n+1 {
		if binary.LittleEndian.Uint64(data[(i>>6)*8:])>>(i&0x3F)&1 == 0 {
			return 0, false
		}
		at := bm + i*0x10
		if binary.LittleEndian.Uint64(data[at:]) == handle {
			idx := binary.LittleEndian.Uint32(data[at+8:])
			if idx == 0xFFFFFFFF {
				return 0, false
			}
			return m.table + uint64(idx)*recordStride, true
		}
	}
	return 0, false
}

// Entity resolves a handle through a zoo object.
func (a *Animals) Entity(zoo, handle uint64) (uint64, bool) {
	if zoo == 0 {
		return 0, false
	}
	return a.EntityByManager(mem.Ptr(a.g.proc, zoo+zooAnimalManager), handle)
}

func (a *Animals) Newborn(entity uint64) bool {
	stage, err := mem.U8(a.g.proc, entity+entityLifeStage)
	return err == nil && stage == lifeStageNewborn
}

func (a *Animals) SpeciesHandle(entity uint64) (uint32, bool) {
	v, err := mem.U32(a.g.proc, entity+entitySpeciesHandle)
	return v, err == nil
}

// Manager is the owned-animal roster manager: habitat and storage.
func (a *Animals) Manager() uint64 {
	park := a.g.Park()
	if park == 0 {
		return 0
	}
	return mem.Ptr(a.g.proc, park+parkAnimalManager)
}

// Roster calls visit for every animal of a manager.
func (a *Animals) Roster(mgr uint64, visit func(handle, entity uint64)) {
	m, ok := a.roster(mgr)
	if !ok {
		return
	}
	data, bm, ok := a.slots(m)
	if !ok {
		return
	}
	for i := range m.slots {
		if binary.LittleEndian.Uint64(data[(i>>6)*8:])>>(i&0x3F)&1 == 0 {
			continue
		}
		at := bm + i*0x10
		if idx := binary.LittleEndian.Uint32(data[at+8:]); idx != 0xFFFFFFFF {
			visit(binary.LittleEndian.Uint64(data[at:]), m.table+uint64(idx)*recordStride)
		}
	}
}

// ExhibitManager is the exhibit-animal manager, separate from the roster.
func (a *Animals) ExhibitManager() uint64 {
	park := a.g.Park()
	if park == 0 {
		return 0
	}
	return mem.Ptr(a.g.proc, park+parkExhibitManager)
}

// ExhibitCensus is {species handle: count} of the placed exhibit animals,
// or false when mgr does not hold a valid map.
func (a *Animals) ExhibitCensus(mgr uint64) (map[uint32]uint32, bool) {
	if mgr == 0 {
		return nil, false
	}
	hdr := mgr + exhibitCensus
	slots := mem.Ptr(a.g.proc, hdr+mapCap)
	buckets := mem.Ptr(a.g.proc, hdr+mapBuckets)
	if slots == 0 || buckets == 0 || slots > 1<<20 || !isPow2(slots) {
		return nil, false
	}
	bm := bitmapBytes(slots)
	data, err := a.g.proc.Read(buckets, int(bm+slots*8))
	if err != nil {
		return nil, false
	}
	out := map[uint32]uint32{}
	for i := range slots {
		if binary.LittleEndian.Uint64(data[(i>>6)*8:])>>(i&0x3F)&1 == 0 {
			continue
		}
		handle := binary.LittleEndian.Uint32(data[bm+i*8:])
		if count := binary.LittleEndian.Uint32(data[bm+i*8+4:]); count > 0 {
			out[handle] = count
		}
	}
	return out, true
}

// DefHolder is the object owning the exhibit animal id set and def map:
// *(mgr+0xF8), else *(*(mgr+0x2F0)+0xD8), whichever holds a valid def map.
func (a *Animals) DefHolder(mgr uint64) uint64 {
	if mgr == 0 {
		return 0
	}
	var cands []uint64
	if c := mem.Ptr(a.g.proc, mgr+exhibitDefHolder); c != 0 {
		cands = append(cands, c)
	}
	if p := mem.Ptr(a.g.proc, mgr+exhibitHolderAlt[0]); p != 0 {
		if c := mem.Ptr(a.g.proc, p+exhibitHolderAlt[1]); c != 0 && (len(cands) == 0 || c != cands[0]) {
			cands = append(cands, c)
		}
	}
	for _, c := range cands {
		if _, _, ok := a.defMap(c); ok {
			return c
		}
	}
	if len(cands) > 0 {
		return cands[0]
	}
	return 0
}

func (a *Animals) defMap(holder uint64) (slots, buckets uint64, ok bool) {
	sub := mem.Ptr(a.g.proc, holder+exhibitDefSub)
	if sub == 0 {
		return 0, 0, false
	}
	hdr := sub + defSubMap
	count := mem.Ptr(a.g.proc, hdr+0x08)
	slots = mem.Ptr(a.g.proc, hdr+0x10)
	buckets = mem.Ptr(a.g.proc, hdr+0x18)
	return slots, buckets, buckets != 0 && slots > 0 && slots <= 1<<16 && isPow2(slots) && count <= slots
}

// ExhibitIDs is the set of owned exhibit animal ids.
func (a *Animals) ExhibitIDs(holder uint64) (map[uint32]bool, bool) {
	if holder == 0 {
		return nil, false
	}
	base := holder + exhibitIDSet
	slots := mem.Ptr(a.g.proc, base+0x10)
	buf := mem.Ptr(a.g.proc, base+0x18)
	if buf == 0 || slots == 0 || slots > 1<<20 || !isPow2(slots) {
		return nil, false
	}
	bm := bitmapBytes(slots)
	data, err := a.g.proc.Read(buf, int(bm+slots*4))
	if err != nil {
		return nil, false
	}
	out := map[uint32]bool{}
	for i := range slots {
		if occupied(data, i) {
			out[binary.LittleEndian.Uint32(data[bm+i*4:])] = true
		}
	}
	return out, true
}

// ExhibitDef is what the def map says of one exhibit animal: its species
// handle (0 when implausible) and the names on its definition.
type ExhibitDef struct {
	Handle uint32
	Names  []string
}

// ExhibitDefs is {animal id: definition}. Definitions outlive a release.
func (a *Animals) ExhibitDefs(holder uint64) (map[uint32]ExhibitDef, bool) {
	if holder == 0 {
		return nil, false
	}
	slots, buckets, ok := a.defMap(holder)
	if !ok {
		return nil, false
	}
	bm := bitmapBytes(slots)
	bitmap, err := a.g.proc.Read(buckets, int(bm))
	if err != nil {
		return nil, false
	}
	out := map[uint32]ExhibitDef{}
	for i := range slots {
		if !occupied(bitmap, i) {
			continue
		}
		entry := buckets + bm + i*defEntryStride
		id, err := mem.U32(a.g.proc, entry)
		if err != nil {
			continue
		}
		handle, err := mem.U32(a.g.proc, entry+defEntrySpecies)
		if err != nil {
			continue
		}
		if handle == 0 || handle >= 1<<24 {
			handle = 0
		}
		def := ExhibitDef{Handle: handle}
		for _, off := range defStringOffsets {
			if name, ok := a.g.nativeString(mem.Ptr(a.g.proc, entry+defEntryObj+off), 64); ok {
				def.Names = append(def.Names, name)
			}
		}
		out[id] = def
	}
	return out, true
}

// nativeString reads the engine's refcounted string {len i64 @+0, chars
// @+0x14}.
func (g *Game) nativeString(ptr uint64, limit int64) (string, bool) {
	if ptr == 0 {
		return "", false
	}
	n, err := mem.I64(g.proc, ptr)
	if err != nil || n <= 0 || n > limit {
		return "", false
	}
	raw, err := g.proc.Read(ptr+0x14, int(n))
	if err != nil {
		return "", false
	}
	for _, c := range raw {
		if c < 0x20 || c >= 0x7F {
			return "", false
		}
	}
	return string(raw), true
}
