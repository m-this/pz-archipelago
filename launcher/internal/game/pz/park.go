package pz

import (
	"math"
	"time"

	"github.com/m-this/pz-archipelago/launcher/internal/game/mem"
)

// The park-info object has no stable pointer chain; it is found by its
// vtable. It carries the years the park has been open at +0x1C8 (0 in its
// first year) and the park's name at +0x1E8, which the AP scenario script
// sets to the marker below.
const (
	parkInfoVtableRVA = 0x26C2ED8
	parkInfoYears     = 0x1C8
	parkInfoName      = 0x1E8

	// APParkName is the marker. It must match the scenario script and the
	// career entry in mod/.
	APParkName = "ARCHIPELAGO ZOO"

	parkScanPeriod     = 5 * time.Second
	parkNameRescanIdle = 20 * time.Second
	parkMaxYears       = 100000
	parkMaxInstances   = 64
	parkNameMax        = 96
)

// parkInfo caches the park-info instances and rescans the heap for them only
// when the cache misses, at most once per period.
type parkInfo struct {
	g        *Game
	cached   []uint64
	lastScan time.Time
}

func (p *parkInfo) vtable() uint64 { return p.g.base + parkInfoVtableRVA }

func (p *parkInfo) valid(addr uint64) bool { return mem.Ptr(p.g.proc, addr) == p.vtable() }

// scan keeps the instances that carry the vtable and pass keep.
func (p *parkInfo) scan(keep func(addr uint64) bool) {
	p.lastScan = time.Now()
	hits, err := mem.FindQword(p.g.proc, p.vtable(), parkMaxInstances)
	if err != nil {
		p.cached = nil
		return
	}
	p.cached = p.cached[:0]
	for _, h := range hits {
		if p.valid(h) && keep(h) {
			p.cached = append(p.cached, h)
		}
	}
}

func (p *parkInfo) cacheValid() bool {
	if len(p.cached) == 0 {
		return false
	}
	for _, a := range p.cached {
		if !p.valid(a) {
			return false
		}
	}
	return true
}

func (p *parkInfo) forget() {
	p.cached = nil
	p.lastScan = time.Time{}
}

// ParkName reads the loaded park's name.
type ParkName struct {
	info parkInfo
	last string
}

func (g *Game) NewParkName() *ParkName { return &ParkName{info: parkInfo{g: g}} }

func (n *ParkName) first() string {
	for _, a := range n.info.cached {
		if name, ok := n.info.g.nativeString(mem.Ptr(n.info.g.proc, a+parkInfoName), parkNameMax); ok {
			return name
		}
	}
	return ""
}

// Read is the park's name, "" when unnamed or no zoo is loaded. A cache that
// holds only nameless instances is rescanned on a longer period: the live
// park-info is allocated at load, after a cache built at the menu.
func (n *ParkName) Read() string {
	period := parkScanPeriod
	if n.info.cacheValid() {
		if name := n.first(); name != "" {
			n.last = name
			return name
		}
		period = parkNameRescanIdle
	}
	if !n.info.lastScan.IsZero() && time.Since(n.info.lastScan) < period {
		return n.last
	}
	n.info.scan(func(uint64) bool { return true })
	n.last = n.first()
	return n.last
}

// Forget drops the cache: freed park-info memory can keep the old park's
// name readable long after it unloaded.
func (n *ParkName) Forget() {
	n.info.forget()
	n.last = ""
}

// ParkAge reads the completed years the park has been open.
type ParkAge struct {
	info parkInfo
	last int64
	ok   bool
}

func (g *Game) NewParkAge() *ParkAge { return &ParkAge{info: parkInfo{g: g}} }

func (a *ParkAge) years(addr uint64) (int64, bool) {
	y, err := mem.I64(a.info.g.proc, addr+parkInfoYears)
	return y, err == nil && y >= 0 && y < parkMaxYears
}

func (a *ParkAge) maxYears() (int64, bool) {
	best, found := int64(-1), false
	for _, addr := range a.info.cached {
		if !a.info.valid(addr) {
			return 0, false
		}
		y, ok := a.years(addr)
		if !ok {
			return 0, false
		}
		best, found = max(best, y), true
	}
	return best, found
}

// Read returns the years, the largest over the instances: a static template
// always reads 0. False when no instance resolves.
func (a *ParkAge) Read() (int64, bool) {
	if y, ok := a.maxYears(); ok {
		return y, true
	}
	if !a.info.lastScan.IsZero() && time.Since(a.info.lastScan) < parkScanPeriod {
		return a.last, a.ok
	}
	a.info.scan(func(addr uint64) bool { _, ok := a.years(addr); return ok })
	a.last, a.ok = a.maxYears()
	return a.last, a.ok
}

// Metric is a value read through an anchor chain from a root global.
type Metric struct {
	Name    string
	offsets []uint64
	kind    metricKind
	scale   float64
	lo, hi  float64
}

type metricKind uint8

const (
	kindI32 metricKind = iota + 1
	kindI64
	kindF32
)

// The four data anchors, from upstream's memory/anchors.json, with the range
// a sane read falls in: a read outside it means no zoo, or a stale chain.
var (
	Cash                = Metric{Name: "cash", offsets: []uint64{rootCash, 24, 3832, 232}, kind: kindI64, scale: 100, lo: -5_000_000, hi: 1_000_000_000}
	ConservationCredits = Metric{Name: "conservation_credits", offsets: []uint64{rootCash, 24, 3832, 760}, kind: kindI64, scale: 1, lo: 0, hi: 50_000_000}
	ZooRating           = Metric{Name: "zoo_rating", offsets: []uint64{rootGuest, 32, 1936, 320}, kind: kindF32, scale: 0.2, lo: 0, hi: 5}
	GuestCount          = Metric{Name: "guest_count", offsets: []uint64{rootGuest, 32, 408, 2096}, kind: kindI32, scale: 1, lo: 0, hi: 200_000}
)

func (g *Game) metricAddr(m Metric) uint64 {
	return mem.Chain(g.proc, g.root(m.offsets[0]), append([]uint64{0}, m.offsets[1:]...)...)
}

// Read is the metric in its logical unit (dollars, credits, stars, guests),
// false when unresolved or out of its sane range.
func (g *Game) Read(m Metric) (float64, bool) {
	addr := g.metricAddr(m)
	if addr == 0 {
		return 0, false
	}
	var raw float64
	switch m.kind {
	case kindI32:
		v, err := mem.I32(g.proc, addr)
		if err != nil {
			return 0, false
		}
		raw = float64(v)
	case kindI64:
		v, err := mem.I64(g.proc, addr)
		if err != nil {
			return 0, false
		}
		raw = float64(v)
	case kindF32:
		v, err := mem.F32(g.proc, addr)
		if err != nil {
			return 0, false
		}
		raw = float64(v)
	}
	v := raw / m.scale
	if math.IsNaN(v) || v < m.lo || v > m.hi {
		return 0, false
	}
	return v, true
}

// Write stores a value in the metric's logical unit.
func (g *Game) Write(m Metric, value float64) bool {
	addr := g.metricAddr(m)
	if addr == 0 {
		return false
	}
	stored := value * m.scale
	var err error
	switch m.kind {
	case kindI32:
		err = mem.PutI32(g.proc, addr, int32(math.Round(stored)))
	case kindI64:
		err = mem.PutI64(g.proc, addr, int64(math.Round(stored)))
	case kindF32:
		err = mem.PutF32(g.proc, addr, float32(stored))
	}
	return err == nil
}
