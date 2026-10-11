// Package fakepz lays out a Planet Zoo process in fake memory: enough of
// the park, the anchors, the research map and the hook sites for the client
// loop to attach, recognise the AP park, and run its gates. Tests only.
package fakepz

import (
	"math"

	"github.com/m-this/pz-archipelago/launcher/internal/game/mem"
	"github.com/m-this/pz-archipelago/launcher/internal/game/pz"
)

const (
	Base = 0x140000000
	heap = 0x20000000
)

// Game is the fake process and handles on what a test changes.
type Game struct {
	*mem.Fake
	layout   pz.Layout
	next     uint64
	Park     uint64
	parkInfo uint64
	anchors  map[string]uint64
}

func New() *Game {
	f := mem.NewSparseFake(Base, 0xC000000)
	f.Map(heap, 0x800000, true)
	g := &Game{Fake: f, layout: pz.FakeLayout(), next: heap, anchors: map[string]uint64{}}
	l := g.layout
	for _, rva := range []uint64{0, l.RootCash, l.RootGuest, l.RegistryRVA, l.ResearchChain[0]} {
		f.MapPage(Base + rva)
	}
	for _, s := range l.Sites {
		f.MapPage(Base + s.RVA)
		f.MapPage(Base + s.RVA + uint64(len(s.Orig)))
		g.must(f.Write(Base+s.RVA, s.Orig))
	}
	g.layPark()
	g.layAnchors()
	g.layRegistry()
	g.layResearch()
	return g
}

func (g *Game) must(err error) {
	if err != nil {
		panic(err)
	}
}

func (g *Game) alloc(size uint64) uint64 {
	at := g.next
	g.next += (size + 15) &^ 15
	return at
}

func (g *Game) put64(addr, v uint64) { g.must(mem.PutU64(g, addr, v)) }

func (g *Game) layPark() {
	l := g.layout
	p1, p2 := g.alloc(0x40), g.alloc(0x1000)
	g.put64(Base+l.RootGuest, p1)
	g.put64(p1+l.ParkFirst, p2)
	g.Park = p2 + l.ParkOffset
	for _, off := range []uint64{l.HabitatMarket, l.ExhibitMarket} {
		g.put64(g.Park+off, g.alloc(0x600))
	}
	g.parkInfo = g.alloc(0x200)
	g.put64(g.parkInfo, Base+l.ParkInfoVtable)
	g.SetParkName(pz.APParkName)
}

// SetParkName names the park; "" leaves it unnamed.
func (g *Game) SetParkName(name string) {
	if name == "" {
		g.put64(g.parkInfo+g.layout.ParkInfoName, 0)
		return
	}
	str := g.alloc(0x20 + uint64(len(name)))
	g.put64(str, uint64(len(name)))
	g.must(g.Write(str+0x14, []byte(name)))
	g.put64(g.parkInfo+g.layout.ParkInfoName, str)
}

// SetYears sets the years the park has been open.
func (g *Game) SetYears(y uint64) { g.put64(g.parkInfo+g.layout.ParkInfoYears, y) }

// link follows the pointer at slot, creating the object it points to when
// there is none yet; chains that share a prefix share their objects, as in
// the game.
func (g *Game) link(slot uint64) uint64 {
	if cur := mem.Ptr(g, slot); cur != 0 {
		return cur
	}
	next := g.alloc(0x1000)
	g.put64(slot, next)
	return next
}

// anchor lays out a chain from a root global to a value slot.
func (g *Game) anchor(name string, offsets []uint64) {
	addr := g.link(Base + offsets[0])
	for _, off := range offsets[1 : len(offsets)-1] {
		addr = g.link(addr + off)
	}
	g.anchors[name] = addr + offsets[len(offsets)-1]
}

func (g *Game) layAnchors() {
	l := g.layout
	g.anchor("cash", l.Cash)
	g.anchor("credits", l.Credits)
	g.anchor("rating", l.Rating)
	g.anchor("guests", l.Guests)
	g.SetCash(150000)
}

// SetCash, Cash, SetGuests and SetRating read and write the anchors.
func (g *Game) SetCash(dollars float64) {
	g.must(mem.PutI64(g, g.anchors["cash"], int64(math.Round(dollars*100))))
}

func (g *Game) Cash() float64 {
	v, _ := mem.I64(g, g.anchors["cash"])
	return float64(v) / 100
}

func (g *Game) SetGuests(n int32) { g.must(mem.PutI32(g, g.anchors["guests"], n)) }

func (g *Game) SetRating(stars float32) { g.must(mem.PutF32(g, g.anchors["rating"], stars*0.2)) }

func (g *Game) layRegistry() {
	reg := g.alloc(0x100)
	slots := g.alloc(0x100)
	g.put64(Base+g.layout.RegistryRVA, reg)
	g.put64(reg+0x10, 8)
	g.put64(reg+0x30, slots+0x80)
	g.must(mem.PutU32(g, reg+0x48, 4))
}

func (g *Game) layResearch() {
	l := g.layout
	addr := g.link(Base + l.ResearchChain[0])
	for _, off := range l.ResearchChain[1 : len(l.ResearchChain)-1] {
		addr = g.link(addr + off)
	}
	rs := g.link(addr + l.ResearchChain[len(l.ResearchChain)-1])
	slots := uint64(16)
	buckets := g.alloc(8 + slots*l.RecordStride)
	g.put64(rs+l.ResearchItemsMap+0x10, slots)
	g.put64(rs+l.ResearchItemsMap+0x18, buckets)
}
