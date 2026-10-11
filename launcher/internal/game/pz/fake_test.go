package pz

import (
	"encoding/binary"
	"log/slog"
	"testing"

	"github.com/m-this/pz-archipelago/launcher/internal/game/mem"
)

const (
	testBase = 0x140000000
	heap     = 0x20000000
)

// world is a fake game process laid out the way the readers expect it.
type world struct {
	t    *testing.T
	f    *mem.Fake
	g    *Game
	next uint64
}

func newWorld(t *testing.T) *world {
	t.Helper()
	f := mem.NewFake(testBase, 0x3000000)
	f.Map(heap, 0x400000, true)
	w := &world{t: t, f: f, next: heap}
	w.g = New(f, slog.New(slog.DiscardHandler))
	return w
}

func (w *world) alloc(size uint64) uint64 {
	at := w.next
	w.next += (size + 15) &^ 15
	return at
}

func (w *world) put64(addr, v uint64) {
	w.t.Helper()
	if err := mem.PutU64(w.f, addr, v); err != nil {
		w.t.Fatal(err)
	}
}

func (w *world) put32(addr uint64, v uint32) {
	w.t.Helper()
	if err := mem.PutU32(w.f, addr, v); err != nil {
		w.t.Fatal(err)
	}
}

// registry interns names and returns their ids in order, from 1.
func (w *world) registry(names ...string) map[string]uint32 {
	const stride = 8
	reg := w.alloc(0x100)
	slots := w.alloc(uint64(len(names)+2) * stride)
	top := slots + uint64(len(names)+1)*stride
	w.put64(testBase+registryRVA, reg)
	w.put64(reg+regStride, stride)
	w.put64(reg+regTop, top)
	w.put32(reg+regCap, uint32(len(names)+1))
	ids := map[string]uint32{}
	for i, name := range names {
		id := uint32(i + 1)
		entry := w.alloc(uint64(len(name) + 16))
		w.put32(entry+regEntryRef, 1)
		if err := w.f.Write(entry+regEntryName, append([]byte(name), 0)); err != nil {
			w.t.Fatal(err)
		}
		w.put64(top-uint64(id)*stride, entry)
		ids[name] = id
	}
	return ids
}

// research lays out a research system holding records, through the
// primary chain.
func (w *world) research(records []Record) {
	rs := w.alloc(0x200)
	a, b := w.alloc(0x40), w.alloc(0x400)
	w.put64(testBase+0x2944690, a)
	w.put64(a+0x18, b)
	w.put64(b+0x350, rs)
	slots := uint64(16)
	bm := bitmapBytes(slots)
	buckets := w.alloc(bm + slots*researchRecStride)
	w.put64(rs+researchItemsMap+0x10, slots)
	w.put64(rs+researchItemsMap+0x18, buckets)
	bitmap := make([]byte, bm)
	for i, rec := range records {
		slot := uint64(i * 2 % int(slots))
		bitmap[slot>>3] |= 1 << (slot & 7)
		raw := make([]byte, researchRecStride)
		binary.LittleEndian.PutUint32(raw[recItem:], rec.Item)
		binary.LittleEndian.PutUint32(raw[recName:], rec.Name)
		binary.LittleEndian.PutUint32(raw[recLevel:], rec.Level)
		binary.LittleEndian.PutUint32(raw[recHandle:], rec.Handle)
		raw[recCategory], raw[recStatus] = rec.Category, rec.Status
		if err := w.f.Write(buckets+bm+slot*researchRecStride, raw); err != nil {
			w.t.Fatal(err)
		}
	}
	if err := w.f.Write(buckets, bitmap); err != nil {
		w.t.Fatal(err)
	}
}

func TestWelfareLevelsByRank(t *testing.T) {
	w := newWorld(t)
	ids := w.registry("Aardvark", "AfricaThemeSetsScenery")
	zebra := ids["Aardvark"]
	// An exhibit-like species whose item ids do not follow its levels.
	w.research([]Record{
		{Item: 0x900, Level: 2, Handle: zebra, Category: CategoryAnimal, Status: StatusCompleted},
		{Item: 0x901, Level: 1, Handle: zebra, Category: CategoryAnimal, Status: 1},
		{Item: 0x902, Level: 3, Handle: zebra, Category: CategoryAnimal, Status: StatusCompleted},
		{Item: 0x903, Level: 10, Handle: zebra, Category: CategoryAnimal, Status: 1},
		{Item: 0x2000, Name: ids["AfricaThemeSetsScenery"], Category: CategoryMechanic, Status: StatusCompleted},
	})
	r := w.g.Research
	snap, ok := r.Snapshot()
	if !ok {
		t.Fatal("snapshot")
	}
	if key, ok := r.HandleKeys(snap)[zebra]; !ok || key != "aardvark" {
		t.Fatalf("handle keys: %v", r.HandleKeys(snap))
	}
	got := []bool{
		r.WelfareLevelComplete("aardvark", 1, snap), r.WelfareLevelComplete("aardvark", 2, snap),
		r.WelfareLevelComplete("aardvark", 3, snap), r.WelfareLevelComplete("aardvark", 4, snap),
	}
	if got[0] || !got[1] || !got[2] || got[3] {
		t.Errorf("levels complete: %v, want false true true false", got)
	}
	if !r.MechanicComplete("AfricaThemeSetsScenery", snap) || r.MechanicComplete("PowerSolarPanel", snap) {
		t.Error("mechanic completion")
	}
	if h, ok := r.Handle("aardvark", snap); !ok || h != zebra {
		t.Errorf("handle %d", h)
	}
}

func TestResearchGateWritesCategoriesAndStopsRunning(t *testing.T) {
	w := newWorld(t)
	w.registry()
	w.research([]Record{{Item: 1, Category: CategoryAnimal, Status: statusResearching}, {Item: 2, Category: CategoryMechanic, Status: statusResearching}})
	site := testBase + siteResearchStart.RVA
	if err := w.f.Write(site, siteResearchStart.Orig); err != nil {
		t.Fatal(err)
	}
	gate := w.g.NewResearchGate()
	if !gate.Reconcile(false, true) {
		t.Fatal("reconcile")
	}
	count, _ := mem.U32(w.f, gate.scratch+0)
	cat, _ := mem.U8(w.f, gate.scratch+8)
	if count != 1 || cat != CategoryAnimal {
		t.Errorf("gated %d categories, first %d", count, cat)
	}
	snap, _ := w.g.Research.Snapshot()
	for _, rec := range snap.Records() {
		want := uint8(statusResearching)
		if rec.Category == CategoryAnimal {
			want = statusResearchable
		}
		if rec.Status != want {
			t.Errorf("item %d status %d, want %d", rec.Item, rec.Status, want)
		}
	}
}

// park lays out the park chain and returns the park object.
func (w *world) park() uint64 {
	p1, p2 := w.alloc(0x40), w.alloc(0x1000)
	w.put64(testBase+rootGuest, p1)
	w.put64(p1+parkFirst, p2)
	return p2 + parkOffset
}

func TestMarketApplyAndRestore(t *testing.T) {
	w := newWorld(t)
	park := w.park()
	mgr := w.alloc(0x500)
	w.put64(park+habitatMarket.parkOff, mgr)
	inc := mgr + habitatMarket.defaultWL + wlIncludeSet
	w.put64(inc+setBuffer, 0xABC0)
	w.put64(inc+setCap, 4)
	habitat, _ := w.g.NewMarketGates()
	if !habitat.ScenarioMode() {
		t.Fatal("mode 0 is scenario mode")
	}
	if !habitat.Apply([]uint32{0x30B0, 0x309F, 0x30B0}) {
		t.Fatal("apply")
	}
	slots := mem.Ptr(w.f, inc+setCap)
	buf := mem.Ptr(w.f, inc+setBuffer)
	blob, _ := w.f.Read(buf, int(bitmapBytes(slots)+slots*4))
	found := map[uint32]bool{}
	for i := range slots {
		if occupied(blob, i) {
			found[binary.LittleEndian.Uint32(blob[bitmapBytes(slots)+i*4:])] = true
		}
	}
	if len(found) != 2 || !found[0x30B0] || !found[0x309F] || mem.Ptr(w.f, inc+setCount) != 2 {
		t.Errorf("set %v", found)
	}
	if head, _ := w.f.Read(buf-uint64(len(sentinel)), len(sentinel)); string(head) != string(sentinel) {
		t.Error("no sentinel before the buffer")
	}
	if !habitat.Restore() || mem.Ptr(w.f, inc+setBuffer) != 0xABC0 || mem.Ptr(w.f, inc+setCap) != 4 {
		t.Error("restore did not put the game's buffer back")
	}
}

func TestMarketEmptiesAnOrphanFromAKilledClient(t *testing.T) {
	w := newWorld(t)
	park := w.park()
	mgr := w.alloc(0x500)
	w.put64(park+habitatMarket.parkOff, mgr)
	orphan := w.alloc(0x100)
	_ = w.f.Write(orphan, sentinel)
	inc := mgr + habitatMarket.defaultWL + wlIncludeSet
	w.put64(inc+setBuffer, orphan+uint64(len(sentinel)))
	habitat, _ := w.g.NewMarketGates()
	habitat.Apply([]uint32{1})
	if !habitat.Restore() || mem.Ptr(w.f, inc+setBuffer) != 0 {
		t.Error("the orphan was captured as the game's own buffer")
	}
}

func TestRosterLookup(t *testing.T) {
	w := newWorld(t)
	mgr := w.alloc(0x1000)
	slots := uint64(16)
	bm := bitmapBytes(slots)
	buckets := w.alloc(bm + slots*0x10)
	table := w.alloc(4 * recordStride)
	w.put64(mgr+mgrHashMap+mapCap, slots)
	w.put64(mgr+mgrHashMap+mapBuckets, buckets)
	w.put64(mgr+mgrTable, table)
	handle := uint64(0xDEADBEEF12)
	slot := animalHash(handle, slots)
	_ = w.f.Write(buckets+(slot>>6)*8, binary.LittleEndian.AppendUint64(nil, 1<<(slot&0x3F)))
	w.put64(buckets+bm+slot*0x10, handle)
	w.put32(buckets+bm+slot*0x10+8, 2)
	w.put32(table+2*recordStride+entitySpeciesHandle, 0x46DA)
	e, ok := w.g.Animals.EntityByManager(mgr, handle)
	if !ok || e != table+2*recordStride {
		t.Fatalf("entity 0x%X %v", e, ok)
	}
	if sh, _ := w.g.Animals.SpeciesHandle(e); sh != 0x46DA || !w.g.Animals.Newborn(e) {
		t.Error("entity fields")
	}
	if _, ok := w.g.Animals.EntityByManager(mgr, handle+1); ok {
		t.Error("an absent handle resolved")
	}
	w.put64(mgr+mgrHashMap+mapCap, 15)
	if _, ok := w.g.Animals.EntityByManager(mgr, handle); ok {
		t.Error("a manager with a non power-of-two slots was trusted")
	}
}
