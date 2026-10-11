package pz

import (
	"encoding/binary"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/m-this/pz-archipelago/gamedata"
	"github.com/m-this/pz-archipelago/launcher/internal/game/mem"
)

// The research system holds every research item in an open-addressing map
// at +0xF8 {+0x10 slots, +0x18 buckets}: an occupancy bitmap, then 0x58-byte
// records. Upstream's memory/research.py:
//
//	+0x00 u32 research item id (stable across restarts)
//	+0x08 u32 name intern id   +0x0C u32 level
//	+0x10 u32 species handle (changes every run)
//	+0x3C u8 category (7 animal, 3 mechanic)   +0x49 u8 status (4 completed)
const (
	researchItemsMap   = 0xF8
	researchRecStride  = 0x58
	recItem            = 0x00
	recName            = 0x08
	recLevel           = 0x0C
	recHandle          = 0x10
	recCategory        = 0x3C
	recStatus          = 0x49
	researchVtableRVA  = 0x26C3490
	researchScanPeriod = 5 * time.Second

	CategoryAnimal   = 7
	CategoryMechanic = 3

	statusResearchable = 1
	statusResearching  = 2
	StatusCompleted    = 4
	advancedLevel      = 10
)

// Two pointer chains reach the research system in most saves; a heap scan
// for its vtable reaches it in the rest.
var researchChains = [][]uint64{{0x2944690, 0x18, 0x350}, {0x29446A0, 0x20, 0x518}}

// Captured level-0 welfare item ids, a fallback for when the registry cannot
// name a species' handle.
var welfareItem = map[gamedata.SpeciesKey]uint32{
	"pzebra": 0xDAC, "cwarthog": 0x640, "scroc": 0x10CC, "wgorilla": 0x1450,
	"gpanda": 0x834, "hippo": 0x9C4, "twolf": 0x1324, "abison": 0x1F4,
	"aelephant": 0x13, "btiger": 0x320, "sleopard": 0x1194,
}

// Record is one research item as read.
type Record struct {
	Item, Name, Level, Handle uint32
	Category, Status          uint8
	statusAddr                uint64
}

// Snapshot is the research map read once.
type Snapshot struct {
	byItem   map[uint32]Record
	byHandle map[uint32][]Record
	records  []Record
}

// Research reads the research system.
type Research struct {
	g          *Game
	cache      uint64
	lastScan   time.Time
	welfare    map[gamedata.SpeciesKey]uint32
	tokenToKey map[string]gamedata.SpeciesKey
	mechanic   map[string]uint32
}

func newResearch(g *Game) *Research {
	r := &Research{g: g, welfare: map[gamedata.SpeciesKey]uint32{}, tokenToKey: map[string]gamedata.SpeciesKey{}}
	maps.Copy(r.welfare, welfareItem)
	for _, s := range gamedata.AllSpecies {
		r.tokenToKey[Normalise(s.EngineToken)] = s.Key
	}
	return r
}

// Normalise lowercases a name and keeps letters and digits, the form names
// and tokens are matched in.
func Normalise(s string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(s) {
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
			b.WriteRune(c)
		}
	}
	return b.String()
}

func (r *Research) mapOK(rs uint64) bool {
	c, err := mem.I64(r.g.proc, rs+researchItemsMap+0x10)
	bk := mem.Ptr(r.g.proc, rs+researchItemsMap+0x18)
	return err == nil && c > 0 && c <= 1<<20 && isHeap(bk)
}

// System is the research system, or 0 when it cannot be found yet.
func (r *Research) System() uint64 {
	if r.cache != 0 && r.mapOK(r.cache) {
		return r.cache
	}
	for _, chain := range researchChains {
		if rs := mem.Deref(r.g.proc, r.g.base, chain...); rs != 0 && r.mapOK(rs) {
			r.cache = rs
			return rs
		}
	}
	if time.Since(r.lastScan) < researchScanPeriod {
		return 0
	}
	r.lastScan = time.Now()
	hits, err := mem.FindQword(r.g.proc, r.g.base+researchVtableRVA, 16)
	if err != nil {
		return 0
	}
	for _, obj := range hits {
		if r.mapOK(obj) {
			r.g.log.Info("research system found by its vtable", "addr", obj)
			r.cache = obj
			return obj
		}
	}
	return 0
}

// Snapshot reads every record, or returns false when the map is not readable.
func (r *Research) Snapshot() (*Snapshot, bool) {
	rs := r.System()
	if rs == 0 {
		return nil, false
	}
	c, err := mem.I64(r.g.proc, rs+researchItemsMap+0x10)
	bk := mem.Ptr(r.g.proc, rs+researchItemsMap+0x18)
	if err != nil || c <= 0 || c > 1<<20 || !isHeap(bk) {
		return nil, false
	}
	slots := uint64(c)
	bm := bitmapBytes(slots)
	bitmap, err := r.g.proc.Read(bk, int(bm))
	if err != nil {
		return nil, false
	}
	recs, err := r.g.proc.Read(bk+bm, int(slots*researchRecStride))
	if err != nil {
		return nil, false
	}
	s := &Snapshot{byItem: map[uint32]Record{}, byHandle: map[uint32][]Record{}}
	for i := range slots {
		if !occupied(bitmap, i) {
			continue
		}
		raw := recs[i*researchRecStride : (i+1)*researchRecStride]
		rec := Record{
			Item: binary.LittleEndian.Uint32(raw[recItem:]), Name: binary.LittleEndian.Uint32(raw[recName:]),
			Level: binary.LittleEndian.Uint32(raw[recLevel:]), Handle: binary.LittleEndian.Uint32(raw[recHandle:]),
			Category: raw[recCategory], Status: raw[recStatus],
			statusAddr: bk + bm + i*researchRecStride + recStatus,
		}
		s.byItem[rec.Item] = rec
		s.byHandle[rec.Handle] = append(s.byHandle[rec.Handle], rec)
		s.records = append(s.records, rec)
	}
	return s, true
}

// Records lists every record of the snapshot.
func (s *Snapshot) Records() []Record { return s.records }

// SetStatus writes a record's status byte.
func (r *Research) SetStatus(rec Record, status uint8) error {
	return mem.PutU8(r.g.proc, rec.statusAddr, status)
}

// MechanicItems maps a mechanic research's normalised engine name to its item
// id, through the registry. Cached once the registry has named them.
func (r *Research) MechanicItems(s *Snapshot) map[string]uint32 {
	if r.mechanic != nil {
		return r.mechanic
	}
	out := map[string]uint32{}
	for _, rec := range s.records {
		if rec.Category != CategoryMechanic {
			continue
		}
		if name, ok := r.g.Registry.Name(rec.Name); ok {
			out[Normalise(name)] = rec.Item
		}
	}
	if len(out) > 0 {
		r.mechanic = out
	}
	return out
}

// SpeciesForHandle names a live species handle through the registry.
func (r *Research) SpeciesForHandle(handle uint32) (gamedata.SpeciesKey, bool) {
	name, ok := r.g.Registry.Name(handle)
	if !ok {
		return "", false
	}
	key, ok := r.tokenToKey[Normalise(name)]
	return key, ok
}

// SpeciesForName names an engine species token.
func (r *Research) SpeciesForName(name string) (gamedata.SpeciesKey, bool) {
	if name == "" {
		return "", false
	}
	key, ok := r.tokenToKey[Normalise(name)]
	return key, ok
}

// HandleKeys maps every species handle in the snapshot to its key.
func (r *Research) HandleKeys(s *Snapshot) map[uint32]gamedata.SpeciesKey {
	out := map[uint32]gamedata.SpeciesKey{}
	for handle := range s.byHandle {
		if key, ok := r.SpeciesForHandle(handle); ok {
			out[handle] = key
		}
	}
	for key, item := range r.welfare {
		if rec, ok := s.byItem[item]; ok {
			if _, named := out[rec.Handle]; !named {
				out[rec.Handle] = key
			}
		}
	}
	return out
}

// welfareItemOf is the species' level-0 welfare item: captured, or derived
// as the lowest animal-research item of the handle the registry names.
func (r *Research) welfareItemOf(key gamedata.SpeciesKey, s *Snapshot) (uint32, bool) {
	if item, ok := r.welfare[key]; ok {
		return item, true
	}
	for handle := range s.byHandle {
		if k, ok := r.SpeciesForHandle(handle); !ok || k != key {
			continue
		}
		var items []uint32
		for _, rec := range s.byHandle[handle] {
			if rec.Category == CategoryAnimal {
				items = append(items, rec.Item)
			}
		}
		if len(items) == 0 {
			return 0, false
		}
		item := slices.Min(items)
		r.welfare[key] = item
		return item, true
	}
	return 0, false
}

// Handle is the species' handle this run.
func (r *Research) Handle(key gamedata.SpeciesKey, s *Snapshot) (uint32, bool) {
	item, ok := r.welfareItemOf(key, s)
	if !ok {
		return 0, false
	}
	rec, ok := s.byItem[item]
	return rec.Handle, ok
}

// WelfareLevelComplete reports whether the species' welfare research at a
// 1-based level is completed. The level is matched by rank over the
// species' standard animal records ordered by their level field, not by item
// id: exhibit species do not number their items from level 1.
func (r *Research) WelfareLevelComplete(key gamedata.SpeciesKey, level int, s *Snapshot) bool {
	if level < 1 {
		return false
	}
	item, ok := r.welfareItemOf(key, s)
	if !ok {
		return false
	}
	rec, ok := s.byItem[item]
	if !ok {
		return false
	}
	var std []Record
	for _, x := range s.byHandle[rec.Handle] {
		if x.Category == CategoryAnimal && x.Level < advancedLevel {
			std = append(std, x)
		}
	}
	slices.SortStableFunc(std, func(a, b Record) int {
		if a.Level != b.Level {
			return int(a.Level) - int(b.Level)
		}
		return int(a.Status) - int(b.Status)
	})
	return len(std) >= level && std[level-1].Status == StatusCompleted
}

// MechanicComplete reports whether the mechanic research of that engine name
// is completed.
func (r *Research) MechanicComplete(name string, s *Snapshot) bool {
	item, ok := r.MechanicItems(s)[Normalise(name)]
	if !ok {
		return false
	}
	rec, ok := s.byItem[item]
	return ok && rec.Status == StatusCompleted
}

// Forget drops what is cached for a park.
func (r *Research) Forget() {
	r.cache = 0
	r.mechanic = nil
}
