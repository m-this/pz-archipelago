package pz

import (
	"encoding/binary"
	"math"
	"regexp"
	"strconv"

	"github.com/m-this/pz-archipelago/launcher/internal/game/mem"
)

// Rewards grants research rewards the AP items carry. Animal rewards
// (enrichment, supplement, breeding, education) are records in the research
// system's unlockables map, granted by setting their unlocked byte. Mechanic
// build content and barriers are gated behind "gate" research items the
// installer minted, granted by writing the gate's status. Upstream's
// memory/rewards.py.
const (
	unlockMap      = 0x148
	unlockStride   = 0x14
	unlockType     = 0x04
	unlockKey      = 0x08
	unlockBookkeep = 0x0C
	unlockFlag     = 0x12

	countMap      = 0x210
	countStride   = 0xC
	levelMap      = 0x1E8
	levelStride   = 0x10
	eduCounter    = 0x52C
	eduStoreOwner = 0x10
	eduStore      = 0x660
	eduStoreMap   = 0x88
	eduRecStride  = 0x60
	eduRecLevel   = 0x8
	eduMaxLevel   = 3

	typeSupplement = 0
	typeEnrichment = 1
	typeBreeding   = 2
	typeEducation  = 3

	buildableStatus = 4
	breedingMax     = 2
	supplementTiers = 3
	rateEpsilon     = 1e-4
)

// Family is a progressive research reward family.
type Family uint8

const (
	FamilySupplement Family = iota + 1
	FamilyBreeding
	FamilyEducation
	FamilyExhibitEnrichment
)

// A level family's contents are named <Species><Family>L<k>.
var familyLevel = map[Family]*regexp.Regexp{
	FamilyExhibitEnrichment: regexp.MustCompile(`enrichmentl(\d+)`),
	FamilySupplement:        regexp.MustCompile(`supplementl(\d+)`),
	FamilyBreeding:          regexp.MustCompile(`breedingl(\d+)`),
	FamilyEducation:         regexp.MustCompile(`educationl(\d+)`),
}

var breedingRate = [breedingMax + 1]float32{0, 0.15, 0.30}

// barrierGrade is the gate research item of each barrier grade: grades 1 and
// 3 reuse vanilla blueprint items, the others were minted by the installer.
var barrierGrade = [...]string{
	1: "scenarioblueprint01", 2: "apbarriergate2", 3: "scenarioblueprint02",
	4: "apbarriergate4", 5: "apbarriergate5", 6: "apbarriergate6",
}

// facilityGate is the placeholder research item that reveals each facility.
var facilityGate = map[string]string{"research_centre": "guestspawner", "workshop": "parkgate"}

type unlockRecord struct {
	addr      uint64
	cid       uint32
	typ, flag byte
}

type unlocks struct {
	count   int64
	records []unlockRecord
}

// Rewards holds what the granter caches.
type Rewards struct {
	g          *Game
	levels     map[Family]map[uint32]int
	lastCount  int64
	warned     map[string]bool
	eduMapAddr uint64
}

func (g *Game) NewRewards() *Rewards {
	return &Rewards{g: g, levels: map[Family]map[uint32]int{}, lastCount: -1, warned: map[string]bool{}}
}

func (r *Rewards) unlocks() (unlocks, bool) {
	rs := r.g.Research.System()
	if rs == 0 {
		return unlocks{}, false
	}
	count, err := mem.I64(r.g.proc, rs+unlockMap+0x08)
	slots := mem.Ptr(r.g.proc, rs+unlockMap+0x10)
	buckets := mem.Ptr(r.g.proc, rs+unlockMap+0x18)
	if err != nil || count <= 0 || uint64(count) > slots || !isPow2(slots) || !isHeap(buckets) {
		return unlocks{}, false
	}
	bm := bitmapBytes(slots)
	bitmap, err := r.g.proc.Read(buckets, int(bm))
	if err != nil {
		return unlocks{}, false
	}
	blob, err := r.g.proc.Read(buckets+bm, int(slots*unlockStride))
	if err != nil {
		return unlocks{}, false
	}
	u := unlocks{count: count}
	for i := range slots {
		if !occupied(bitmap, i) {
			continue
		}
		off := i * unlockStride
		u.records = append(u.records, unlockRecord{
			addr: buckets + bm + off, cid: binary.LittleEndian.Uint32(blob[off+unlockKey:]),
			typ: blob[off+unlockType], flag: blob[off+unlockFlag],
		})
	}
	return u, true
}

// refreshNames rebuilds the name index when the unlockables map grew: a
// species' reward names intern only when its research tree loads.
func (r *Rewards) refreshNames(u unlocks) {
	if u.count == r.lastCount {
		return
	}
	r.lastCount = u.count
	r.g.Registry.Refresh()
	clear(r.levels)
}

func (r *Rewards) flip(rec unlockRecord) bool {
	if rec.flag != 0 {
		return true
	}
	if err := mem.PutU8(r.g.proc, rec.addr+unlockFlag, 1); err != nil {
		return false
	}
	r.bookkeep(rec)
	return true
}

// bookkeep mirrors the side effects the game applies when it grants a
// record itself, on the bookkeeping records that already exist.
func (r *Rewards) bookkeep(rec unlockRecord) {
	rs := r.g.Research.System()
	if rs == 0 {
		return
	}
	switch rec.typ {
	case typeEducation:
		if cur, err := mem.I32(r.g.proc, rs+eduCounter); err == nil {
			_ = mem.PutI32(r.g.proc, rs+eduCounter, cur+1)
		}
	case typeSupplement, typeBreeding:
		bk, err := mem.U32(r.g.proc, rec.addr+unlockBookkeep)
		if err != nil {
			return
		}
		crec, ok := r.intMapFind(rs+countMap, countStride, bk)
		if !ok {
			return
		}
		if rec.typ == typeSupplement {
			if cur, err := mem.I32(r.g.proc, crec+8); err == nil {
				_ = mem.PutI32(r.g.proc, crec+8, cur+1)
			}
			return
		}
		lrec, ok := r.intMapFind(rs+levelMap, levelStride, rec.cid)
		if !ok {
			return
		}
		level, err1 := mem.F32(r.g.proc, lrec+4)
		cur, err2 := mem.F32(r.g.proc, crec+4)
		if err1 == nil && err2 == nil && level > cur {
			_ = mem.PutF32(r.g.proc, crec+4, level)
		}
	}
}

// intMapFind finds a record by its u32 key in the engine's int maps
// {+0x08 count, +0x10 slots, +0x18 buckets}.
func (r *Rewards) intMapFind(base, stride uint64, key uint32) (uint64, bool) {
	count, err := mem.I64(r.g.proc, base+0x08)
	slots := mem.Ptr(r.g.proc, base+0x10)
	buckets := mem.Ptr(r.g.proc, base+0x18)
	if err != nil || count < 0 || uint64(count) > slots || !isPow2(slots) || !isHeap(buckets) {
		return 0, false
	}
	bm := bitmapBytes(slots)
	bitmap, err := r.g.proc.Read(buckets, int(bm))
	if err != nil {
		return 0, false
	}
	blob, err := r.g.proc.Read(buckets+bm, int(slots*stride))
	if err != nil {
		return 0, false
	}
	for i := range slots {
		if occupied(bitmap, i) && binary.LittleEndian.Uint32(blob[i*stride:]) == key {
			return buckets + bm + i*stride, true
		}
	}
	return 0, false
}

// Grant unlocks one animal reward. False only on a transient failure; a
// content this zoo does not gate is acknowledged, or the item queue would
// stall on it forever.
func (r *Rewards) Grant(content string) bool {
	cid, ok := r.g.Registry.Lookup(content)
	if !ok {
		r.g.Registry.Refresh()
		cid, ok = r.g.Registry.Lookup(content)
	}
	if !ok {
		r.warnOnce(content, "not an interned content name")
		return true
	}
	u, ok := r.unlocks()
	if !ok {
		return false
	}
	for _, rec := range u.records {
		if rec.cid == cid {
			if r.flip(rec) {
				r.g.log.Info("research reward granted", "content", content)
				return true
			}
			return false
		}
	}
	r.warnOnce(content, "not gated in this zoo")
	return true
}

func (r *Rewards) warnOnce(content, why string) {
	if !r.warned[content] {
		r.warned[content] = true
		r.g.log.Warn("a reward the zoo cannot grant is acknowledged", "content", content, "why", why)
	}
}

// gate sets the unlocked byte of every record in gated to whether it is in
// want. Locking writes the byte only; unlocking goes through the
// bookkeeping.
func (r *Rewards) gate(u unlocks, gated map[uint32]bool, want map[uint32]bool) (locked, unlocked int) {
	for _, rec := range u.records {
		if !gated[rec.cid] {
			continue
		}
		target := want[rec.cid]
		if (rec.flag != 0) == target {
			continue
		}
		if target {
			if r.flip(rec) {
				unlocked++
			}
		} else if mem.PutU8(r.g.proc, rec.addr+unlockFlag, 0) == nil {
			locked++
		}
	}
	return locked, unlocked
}

func (r *Rewards) ids(contents []string) map[uint32]bool {
	out := map[uint32]bool{}
	for _, c := range contents {
		if id, ok := r.g.Registry.Lookup(c); ok {
			out[id] = true
		}
	}
	return out
}

// ReconcileRewards locks every animal reward in universe that was not
// received, the ones the scenario ships unlocked included, and unlocks the
// received ones.
func (r *Rewards) ReconcileRewards(received, universe []string) bool {
	u, ok := r.unlocks()
	if !ok {
		return false
	}
	r.refreshNames(u)
	gated := r.ids(universe)
	if len(gated) == 0 {
		return true
	}
	if locked, unlocked := r.gate(u, gated, r.ids(received)); locked+unlocked > 0 {
		r.g.log.Info("reward gate re-synced", "locked", locked, "unlocked", unlocked)
	}
	return true
}

func (r *Rewards) familyLevels(f Family) map[uint32]int {
	if m, ok := r.levels[f]; ok {
		return m
	}
	m := map[uint32]int{}
	for name, id := range r.g.Registry.Index() {
		if sub := familyLevel[f].FindStringSubmatch(name); sub != nil {
			if k, err := strconv.Atoi(sub[1]); err == nil {
				m[id] = k
			}
		}
	}
	r.levels[f] = m
	return m
}

// ReconcileLevels unlocks level <= count of a family for every species, and
// for breeding, supplement and education writes the effect values too.
func (r *Rewards) ReconcileLevels(f Family, count int) bool {
	u, ok := r.unlocks()
	if !ok {
		return false
	}
	r.refreshNames(u)
	levels := r.familyLevels(f)
	if len(levels) == 0 {
		return true
	}
	gated, want := map[uint32]bool{}, map[uint32]bool{}
	for id, k := range levels {
		gated[id] = true
		if k <= count {
			want[id] = true
		}
	}
	locked, unlocked := r.gate(u, gated, want)
	effects := r.syncEffects(f, u, levels, count)
	if f == FamilyEducation {
		effects += r.syncEducationCounter() + r.syncEducationLevels(count)
	}
	if locked+unlocked+effects > 0 {
		r.g.log.Info("level gate re-synced", "family", f, "level", count, "unlocked", unlocked, "locked", locked, "effects", effects)
	}
	return true
}

func (r *Rewards) syncEffects(f Family, u unlocks, levels map[uint32]int, received int) int {
	if f != FamilyBreeding && f != FamilySupplement {
		return 0
	}
	rs := r.g.Research.System()
	if rs == 0 {
		return 0
	}
	bks := map[uint32]bool{}
	for _, rec := range u.records {
		if _, ok := levels[rec.cid]; ok {
			if bk, err := mem.U32(r.g.proc, rec.addr+unlockBookkeep); err == nil && bk != 0 {
				bks[bk] = true
			}
		}
	}
	writes := 0
	for bk := range bks {
		crec, ok := r.intMapFind(rs+countMap, countStride, bk)
		if !ok {
			continue
		}
		if f == FamilyBreeding {
			want := breedingRate[min(max(received, 0), breedingMax)]
			if cur, err := mem.F32(r.g.proc, crec+4); err == nil && math.Abs(float64(cur-want)) > rateEpsilon {
				if mem.PutF32(r.g.proc, crec+4, want) == nil {
					writes++
				}
			}
			continue
		}
		want := int32(min(1+max(received, 0), supplementTiers))
		if cur, err := mem.I32(r.g.proc, crec+8); err == nil && cur != want {
			if mem.PutI32(r.g.proc, crec+8, want) == nil {
				writes++
			}
		}
	}
	return writes
}

// syncEducationCounter sets the zoo's education rating numerator to the
// number of education records unlocked.
func (r *Rewards) syncEducationCounter() int {
	rs := r.g.Research.System()
	u, ok := r.unlocks()
	if rs == 0 || !ok {
		return 0
	}
	var total int32
	for _, rec := range u.records {
		if rec.typ == typeEducation && rec.flag != 0 {
			total++
		}
	}
	cur, err := mem.I32(r.g.proc, rs+eduCounter)
	if err != nil || cur == total {
		return 0
	}
	if mem.PutI32(r.g.proc, rs+eduCounter, total) != nil {
		return 0
	}
	return 1
}

func (r *Rewards) eduMap() (slots, buckets uint64, ok bool) {
	valid := func(hdr uint64) (uint64, uint64, bool) {
		count := mem.Ptr(r.g.proc, hdr+0x08)
		c := mem.Ptr(r.g.proc, hdr+0x10)
		b := mem.Ptr(r.g.proc, hdr+0x18)
		return c, b, b != 0 && count > 0 && count <= c && c <= 1<<12 && isPow2(c)
	}
	if r.eduMapAddr != 0 {
		if c, b, ok := valid(r.eduMapAddr); ok {
			return c, b, true
		}
	}
	r.eduMapAddr = 0
	park := r.g.Park()
	if park == 0 {
		return 0, 0, false
	}
	store := mem.Deref(r.g.proc, park, eduStoreOwner, eduStore)
	if store == 0 {
		return 0, 0, false
	}
	r.eduMapAddr = store + eduStoreMap
	c, b, ok := valid(r.eduMapAddr)
	if !ok {
		r.eduMapAddr = 0
	}
	return c, b, ok
}

// syncEducationLevels writes every species' education unlock level, what
// the education panel reads, to the received count.
func (r *Rewards) syncEducationLevels(received int) int {
	slots, buckets, ok := r.eduMap()
	if !ok {
		return 0
	}
	bm := bitmapBytes(slots)
	data, err := r.g.proc.Read(buckets, int(bm+slots*eduRecStride))
	if err != nil {
		return 0
	}
	want := uint32(min(max(received, 0), eduMaxLevel))
	type rec struct {
		addr uint64
		cur  uint32
	}
	var recs []rec
	for i := range slots {
		if !occupied(data, i) {
			continue
		}
		base := bm + i*eduRecStride
		key := binary.LittleEndian.Uint32(data[base:])
		cur := binary.LittleEndian.Uint32(data[base+eduRecLevel:])
		if key >= 1<<20 || cur > 5 {
			return 0
		}
		recs = append(recs, rec{addr: buckets + base + eduRecLevel, cur: cur})
	}
	writes := 0
	for _, x := range recs {
		if x.cur != want && mem.PutU32(r.g.proc, x.addr, want) == nil {
			writes++
		}
	}
	return writes
}

// buildGates writes status 4 to every research item named in want, the gates
// that make content buildable. They are not AP locations, so no check fires.
func (r *Rewards) buildGates(want map[string]bool) bool {
	snap, ok := r.g.Research.Snapshot()
	if !ok {
		return false
	}
	items := r.g.Research.MechanicItems(snap)
	if len(items) == 0 {
		return false
	}
	ids := map[uint32]string{}
	for name := range want {
		if id, ok := items[name]; ok {
			ids[id] = name
		}
	}
	for _, rec := range snap.Records() {
		if name, ok := ids[rec.Item]; ok && rec.Status < buildableStatus {
			if err := r.g.Research.SetStatus(rec, buildableStatus); err != nil {
				return false
			}
			r.g.log.Info("made buildable", "gate", name)
		}
	}
	return true
}

// ReconcileBarriers makes every barrier of grade <= levels buildable.
func (r *Rewards) ReconcileBarriers(levels int) bool {
	want := map[string]bool{}
	for grade := 1; grade <= min(levels, len(barrierGrade)-1); grade++ {
		want[barrierGrade[grade]] = true
	}
	if len(want) == 0 {
		return true
	}
	return r.buildGates(want)
}

// ReconcileFacilities reveals the received facilities' build items.
func (r *Rewards) ReconcileFacilities(facilities []string) bool {
	want := map[string]bool{}
	for _, f := range facilities {
		if g, ok := facilityGate[f]; ok {
			want[g] = true
		}
	}
	if len(want) == 0 {
		return true
	}
	return r.buildGates(want)
}

// ReconcileMechanic makes the received mechanic content buildable through
// its ApGate<Content> item.
func (r *Rewards) ReconcileMechanic(contents []string) bool {
	want := map[string]bool{}
	for _, c := range contents {
		want["apgate"+Normalise(c)] = true
	}
	if len(want) == 0 {
		return true
	}
	return r.buildGates(want)
}
