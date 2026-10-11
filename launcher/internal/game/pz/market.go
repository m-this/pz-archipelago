package pz

import (
	"bytes"
	"encoding/binary"
	"time"

	"github.com/m-this/pz-archipelago/launcher/internal/game/mem"
)

// Each animal market refills its listings from a candidate pool, which the
// rebuild filters through a default whitelist's include-set. The gate points
// that include-set at a set of the unlocked species, so only they are ever
// offered. Upstream's memory/market.py.
//
// The include-set is an int32 open-addressing hash set {+0x08 count, +0x10
// slots (power of two), +0x18 buffer: occupancy bits, then int32 keys[slots]}.
const (
	setCount  = 0x08
	setCap    = 0x10
	setBuffer = 0x18

	wlIncludeActive = 0x28
	wlIncludeSet    = 0x30

	marketWakePeriod = 10 * time.Second
	marketListingMax = 1024
	marketPoolMax    = 4096
	poolStride       = 0x0C
	poolSpecies      = 0x04
)

// sentinel prefixes our include-set buffer: a client killed before it could
// restore leaves the market pointing at it, and the next client recognises
// it and empties the set, so the game never frees memory it did not
// allocate.
var sentinel = []byte("PZAP-MKT-ORPHAN!")

// marketLayout is one market's manager offsets; the two managers are not
// laid out alike.
type marketLayout struct {
	name                                   string
	parkOff, mode, poolDirty, defaultWL    uint64
	wlIDScenario, wlIDSandbox              uint64
	poolCount, poolData                    uint64
	liveCount, liveData, liveStride        uint64
	liveSpecies, liveExpiry                uint64
	target, forceSpawn, batchMin, batchMax uint64
}

var (
	habitatMarket = marketLayout{
		name: "habitat", parkOff: 0x168, mode: 0x41C, poolDirty: 0x211, defaultWL: 0x3C0,
		wlIDScenario: 0x3B8, wlIDSandbox: 0x418, poolCount: 0x2A0, poolData: 0x2A8,
		liveCount: 0x240, liveData: 0x248, liveStride: 0x240, liveSpecies: 0x10, liveExpiry: 0x214,
		target: 0x26C, forceSpawn: 0x238, batchMin: 0x264, batchMax: 0x268,
	}
	exhibitMarket = marketLayout{
		name: "exhibit", parkOff: 0x1C0, mode: 0x304, poolDirty: 0x1C1, defaultWL: 0x2A8,
		wlIDScenario: 0x2A0, wlIDSandbox: 0x300, poolCount: 0x200, poolData: 0x208,
		liveCount: 0x1C8, liveData: 0x1D0, liveStride: 0x180, liveSpecies: 0x28, liveExpiry: 0x164,
		target: 0x1F4, forceSpawn: 0x1C2, batchMin: 0x1EC, batchMax: 0x1F0,
	}
)

// setHash is the include-set's slot hash.
func setHash(key uint32, slots uint32) uint32 {
	x := key * 0x1001
	x = (x>>0x16 ^ x) * 0x11
	x = (x>>0x09 ^ x) * 0x401
	x = (x>>0x02 ^ x) * 0x81
	return (x>>0x0C ^ x) & (slots - 1)
}

// buildSet lays out an include-set buffer for keys, kept under 70% load.
func buildSet(keys []uint32) (slots uint32, blob []byte) {
	var uniq []uint32
	seen := map[uint32]bool{}
	for _, k := range keys {
		if !seen[k] {
			seen[k] = true
			uniq = append(uniq, k)
		}
	}
	slots = 8
	need := uint32(max(1, len(uniq)))*10/7 + 1
	for slots < need {
		slots <<= 1
	}
	occ := make([]byte, bitmapBytes(uint64(slots)))
	table := make([]uint32, slots)
	for _, k := range uniq {
		slot := setHash(k, slots)
		for occ[slot>>3]>>(slot&7)&1 == 1 {
			slot = (slot + 1) & (slots - 1)
		}
		occ[slot>>3] |= 1 << (slot & 7)
		table[slot] = k
	}
	blob = occ
	for _, k := range table {
		blob = binary.LittleEndian.AppendUint32(blob, k)
	}
	return slots, blob
}

// MarketGate restricts one market to the unlocked species.
type MarketGate struct {
	g         *Game
	layout    marketLayout
	buffer    uint64
	capacity  uint64
	orig      *marketOrig
	checked   bool
	lastWake  time.Time
	lastRaise [3]int64
}

type marketOrig struct {
	mgr, buffer, slots, count uint64
	active                    byte
	wlScenario, wlSandbox     int32
}

func (g *Game) NewMarketGates() (habitat, exhibit *MarketGate) {
	return &MarketGate{g: g, layout: habitatMarket}, &MarketGate{g: g, layout: exhibitMarket}
}

// manager resolves the market manager through the park on every call: a
// cached one dangles once its park unloads, and the walk is three reads.
func (m *MarketGate) manager() uint64 {
	park := m.g.Park()
	if park == 0 {
		return 0
	}
	return mem.Ptr(m.g.proc, park+m.layout.parkOff)
}

// ScenarioMode reports whether the market runs in scenario mode, the one
// the gate restricts. The habitat market's mode is also half of the
// AP-session test.
func (m *MarketGate) ScenarioMode() bool {
	mgr := m.manager()
	if mgr == 0 {
		return false
	}
	mode, err := mem.U8(m.g.proc, mgr+m.layout.mode)
	return err == nil && mode == 0
}

func (m *MarketGate) alloc(size uint64) (uint64, bool) {
	if m.buffer != 0 && size > m.capacity {
		_ = m.g.proc.Free(m.buffer)
		m.buffer = 0
	}
	if m.buffer == 0 {
		total := max(size+uint64(len(sentinel)), 0x1000)
		base, err := m.g.proc.Alloc(total)
		if err != nil {
			m.g.log.Warn("market gate: cannot allocate the include-set", "error", err)
			return 0, false
		}
		m.buffer, m.capacity = base, total-uint64(len(sentinel))
		_ = m.g.proc.Write(base, sentinel)
	}
	return m.buffer + uint64(len(sentinel)), true
}

// Apply installs the allow-list.
func (m *MarketGate) Apply(species []uint32) bool {
	mgr := m.manager()
	if mgr == 0 {
		return false
	}
	slots, blob := buildSet(species)
	buf, ok := m.alloc(uint64(len(blob)))
	if !ok {
		return false
	}
	wl := mgr + m.layout.defaultWL
	inc := wl + wlIncludeSet
	m.neutraliseOrphan(mgr, wl, inc)
	m.captureOriginal(mgr, wl, inc)
	uniq := map[uint32]bool{}
	for _, s := range species {
		uniq[s] = true
	}
	writes := []func() error{
		func() error { return m.g.proc.Write(buf, blob) },
		func() error { return mem.PutU64(m.g.proc, inc+setBuffer, buf) },
		func() error { return mem.PutU64(m.g.proc, inc+setCap, uint64(slots)) },
		func() error { return mem.PutU64(m.g.proc, inc+setCount, uint64(len(uniq))) },
		func() error { return mem.PutU8(m.g.proc, wl+wlIncludeActive, 1) },
		func() error { return mem.PutI32(m.g.proc, mgr+m.layout.wlIDScenario, 0) },
		func() error { return mem.PutI32(m.g.proc, mgr+m.layout.wlIDSandbox, 0) },
		func() error { return mem.PutU8(m.g.proc, mgr+m.layout.poolDirty, 0) },
	}
	for _, w := range writes {
		if err := w(); err != nil {
			m.g.log.Warn("market gate: write failed", "market", m.layout.name, "error", err)
			return false
		}
	}
	m.g.log.Info("market gate: allow-list set", "market", m.layout.name, "species", len(uniq))
	return true
}

func (m *MarketGate) neutraliseOrphan(mgr, wl, inc uint64) {
	if m.checked {
		return
	}
	m.checked = true
	p := mem.Ptr(m.g.proc, inc+setBuffer)
	if p == 0 {
		return
	}
	head, err := m.g.proc.Read(p-uint64(len(sentinel)), len(sentinel))
	if err != nil || !bytes.Equal(head, sentinel) {
		return
	}
	_ = mem.PutU64(m.g.proc, inc+setBuffer, 0)
	_ = mem.PutU64(m.g.proc, inc+setCap, 0)
	_ = mem.PutU64(m.g.proc, inc+setCount, 0)
	_ = mem.PutU8(m.g.proc, wl+wlIncludeActive, 0)
	_ = mem.PutI32(m.g.proc, mgr+m.layout.wlIDScenario, 0)
	_ = mem.PutI32(m.g.proc, mgr+m.layout.wlIDSandbox, 0)
	_ = mem.PutU8(m.g.proc, mgr+m.layout.poolDirty, 0)
	m.g.log.Warn("market gate: emptied an include-set an earlier run left behind", "market", m.layout.name)
}

func (m *MarketGate) captureOriginal(mgr, wl, inc uint64) {
	if m.orig != nil {
		return
	}
	active, err := mem.U8(m.g.proc, wl+wlIncludeActive)
	if err != nil {
		return
	}
	scen, err1 := mem.I32(m.g.proc, mgr+m.layout.wlIDScenario)
	sand, err2 := mem.I32(m.g.proc, mgr+m.layout.wlIDSandbox)
	if err1 != nil || err2 != nil {
		return
	}
	m.orig = &marketOrig{
		mgr: mgr, buffer: mem.Ptr(m.g.proc, inc+setBuffer), slots: mem.Ptr(m.g.proc, inc+setCap),
		count: mem.Ptr(m.g.proc, inc+setCount), active: active, wlScenario: scen, wlSandbox: sand,
	}
}

// ExpireBlocked marks every live listing of a species not in allowed to
// expire on the next market tick.
func (m *MarketGate) ExpireBlocked(allowed []uint32) {
	mgr := m.manager()
	if mgr == 0 {
		return
	}
	ok := map[uint32]bool{}
	for _, a := range allowed {
		ok[a] = true
	}
	count, err := mem.I64(m.g.proc, mgr+m.layout.liveCount)
	data := mem.Ptr(m.g.proc, mgr+m.layout.liveData)
	if err != nil || data == 0 || count < 0 || count > marketListingMax {
		return
	}
	expired := 0
	for i := range uint64(count) {
		ent := data + i*m.layout.liveStride
		sp, err := mem.U32(m.g.proc, ent+m.layout.liveSpecies)
		if err == nil && !ok[sp] {
			if mem.PutF32(m.g.proc, ent+m.layout.liveExpiry, -1e9) == nil {
				expired++
			}
		}
	}
	if expired > 0 {
		m.g.log.Info("market gate: expired listings of blocked species", "market", m.layout.name, "count", expired)
	}
}

func (m *MarketGate) poolSize() int {
	mgr := m.manager()
	count, err := mem.I64(m.g.proc, mgr+m.layout.poolCount)
	data := mem.Ptr(m.g.proc, mgr+m.layout.poolData)
	if err != nil || data == 0 || count < 0 || count > marketPoolMax {
		return 0
	}
	return int(count)
}

// EnsureMinFill keeps a usable selection in an early market: it raises the
// listing target to at most two per unlocked species and, when the market is
// short, forces a spawn at most once per period. It never clears the
// pool-dirty flag: a market tick that rebuilds the pool does not spawn.
func (m *MarketGate) EnsureMinFill(minListings int) {
	mgr := m.manager()
	if mgr == 0 {
		return
	}
	cur, err := mem.I32(m.g.proc, mgr+m.layout.target)
	if err != nil || cur < 0 || cur > marketListingMax {
		return
	}
	pool := m.poolSize()
	floor := int32(max(pool, min(minListings, 2*pool)))
	target := max(cur, floor)
	if cur < floor {
		_ = mem.PutI32(m.g.proc, mgr+m.layout.target, floor)
		if state := [3]int64{int64(cur), int64(floor), int64(pool)}; state != m.lastRaise {
			m.lastRaise = state
			m.g.log.Info("market gate: raised the listing target", "market", m.layout.name, "from", cur, "to", floor)
		}
	}
	live, err := mem.I64(m.g.proc, mgr+m.layout.liveCount)
	if err != nil || live < 0 || live >= int64(target) || time.Since(m.lastWake) < marketWakePeriod {
		return
	}
	m.lastWake = time.Now()
	_ = mem.PutI32(m.g.proc, mgr+m.layout.wlIDScenario, 0)
	_ = mem.PutI32(m.g.proc, mgr+m.layout.wlIDSandbox, 0)
	// A rebuild can roll the spawn batch to [0,0], after which a forced
	// spawn spawns nothing and never clears: a dead market.
	bmin, err1 := mem.I32(m.g.proc, mgr+m.layout.batchMin)
	bmax, err2 := mem.I32(m.g.proc, mgr+m.layout.batchMax)
	if err1 == nil && err2 == nil && bmin >= 0 && bmin <= marketListingMax && bmax >= 0 && bmax <= marketListingMax && bmax < 1 {
		_ = mem.PutI32(m.g.proc, mgr+m.layout.batchMin, 1)
		_ = mem.PutI32(m.g.proc, mgr+m.layout.batchMax, 2)
		m.g.log.Info("market gate: repaired an empty spawn batch", "market", m.layout.name)
	}
	_ = mem.PutU8(m.g.proc, mgr+m.layout.forceSpawn, 1)
}

// Restore points the market back at the game's own include-set before the
// park is torn down; the game freeing our buffer would crash it. It writes
// only if the park still holds the manager the gate changed.
func (m *MarketGate) Restore() bool {
	if m.orig == nil {
		return false
	}
	mgr := m.manager()
	if mgr == 0 || mgr != m.orig.mgr {
		m.orig = nil
		return false
	}
	o := m.orig
	wl := mgr + m.layout.defaultWL
	inc := wl + wlIncludeSet
	for _, err := range []error{
		mem.PutU64(m.g.proc, inc+setBuffer, o.buffer),
		mem.PutU64(m.g.proc, inc+setCap, o.slots),
		mem.PutU64(m.g.proc, inc+setCount, o.count),
		mem.PutU8(m.g.proc, wl+wlIncludeActive, o.active),
		mem.PutI32(m.g.proc, mgr+m.layout.wlIDScenario, o.wlScenario),
		mem.PutI32(m.g.proc, mgr+m.layout.wlIDSandbox, o.wlSandbox),
		mem.PutU8(m.g.proc, mgr+m.layout.poolDirty, 0),
	} {
		if err != nil {
			m.g.log.Warn("market gate: restore failed", "market", m.layout.name, "error", err)
			return false
		}
	}
	m.orig = nil
	m.g.log.Info("market gate: the game's own include-set is back", "market", m.layout.name)
	return true
}

// Shutdown restores the market, then frees our buffer only if nothing can
// still point at it; otherwise it leaks the buffer, which the game reclaims
// when it exits.
func (m *MarketGate) Shutdown() {
	restored := m.Restore()
	parkGone := m.manager() == 0
	if m.buffer != 0 && (restored || parkGone) {
		_ = m.g.proc.Free(m.buffer)
	} else if m.buffer != 0 {
		m.g.log.Warn("market gate: leaving the include-set buffer allocated rather than risk the game freeing it")
	}
	m.buffer, m.capacity = 0, 0
}
