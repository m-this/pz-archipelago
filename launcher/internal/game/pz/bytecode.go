package pz

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"slices"

	"github.com/m-this/pz-archipelago/launcher/internal/game/mem"
)

// Two gates patch Lua bytecode the game has loaded into its heap. A loaded
// code array is byte-identical to the compiled chunk, so it is found by its
// bytes and rewritten in place; the function picks the change up the next
// time it runs. Both defer their first heap sweep off the first tick, and
// wait a few ticks after a failed one.
const bytecodeRetryTicks = 8

type bytecodeFinder struct {
	firstDeferred bool
	cooldown      int
}

// due reports whether a sweep should run this tick.
func (f *bytecodeFinder) due() bool {
	if !f.firstDeferred {
		f.firstDeferred = true
		return false
	}
	if f.cooldown > 0 {
		f.cooldown--
		return false
	}
	return true
}

func (f *bytecodeFinder) failed() { f.cooldown = bytecodeRetryTicks }

func mustHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

// TerrainEditUIMode main.2 decides which terrain tools are enabled. Each
// tool's disabled flag is loaded by a GETTABLE; replacing it with a constant
// LOADBOOL forces the flag. The water tools' flag is loaded at byte 0x0C.
var terrainMain2 = mustHex(
	"8b000000cb00000007014000474140009c0180018d814003cb810100ca01c181ca81c182ca01c283" +
		"220100001e80008001820200224200001e00008001c20200ca018284ca4143861b020002ca010287" +
		"cac0010385010000a20100001e0004809c0180018d814003cb810100cac1c381ca01c482ca41c483" +
		"220100001e80008001820200224200001e00008001c20200ca018284ca4143861b020002ca010287" +
		"cac001039c0180018d814003cb010100ca81c481cac1c482ca01c583ca414386cac001039c018001" +
		"8d814003cb810100ca41c581ca81c582cac1c583620100001e80008001820200224200001e000080" +
		"01c20200ca018284ca4143861b028002ca010287cac001038ac0008c8b018000cb810000ca41438d" +
		"ca41808dab4180008a80818c870147008c41470300020001a441800126008000")

const waterToolsByte = 0x0C

// loadBool is `LOADBOOL R<a>, value, 0` in Lua 5.3.
func loadBool(a uint32, value bool) []byte {
	ins := 3 | (a&0xFF)<<6
	if value {
		ins |= 1 << 23
	}
	return binary.LittleEndian.AppendUint32(nil, ins)
}

// TerrainGate greys the water tools until the Water Habitat Tools arrive,
// and force-enables them after.
type TerrainGate struct {
	g       *Game
	find    bytecodeFinder
	addrs   []uint64
	orig    []byte
	applied *bool
}

func (g *Game) NewTerrainGate() *TerrainGate { return &TerrainGate{g: g} }

func (t *TerrainGate) located() bool {
	if len(t.addrs) > 0 {
		valid := true
		for _, a := range t.addrs {
			if head, err := t.g.proc.Read(a, 8); err != nil || !bytes.Equal(head, terrainMain2[:8]) {
				valid = false
			}
		}
		if valid {
			return true
		}
	}
	if !t.find.due() {
		return false
	}
	addrs, err := mem.FindBytes(t.g.proc, terrainMain2)
	t.orig, t.applied = nil, nil
	if err != nil || len(addrs) == 0 {
		t.addrs = nil
		t.find.failed()
		return false
	}
	t.addrs = addrs
	t.g.log.Info("terrain gate: found the terrain menu bytecode", "copies", len(addrs))
	return true
}

// Reconcile greys or enables the water tools.
func (t *TerrainGate) Reconcile(waterTools bool) bool {
	if !t.located() {
		return false
	}
	disabled := !waterTools
	if t.applied != nil && *t.applied == disabled {
		return true
	}
	if t.orig == nil {
		orig, err := t.g.proc.Read(t.addrs[0]+waterToolsByte, 4)
		if err != nil {
			return false
		}
		t.orig = orig
	}
	patch := loadBool(binary.LittleEndian.Uint32(t.orig)>>6&0xFF, disabled)
	for _, a := range t.addrs {
		if err := t.g.proc.Write(a+waterToolsByte, patch); err != nil {
			t.g.log.Warn("terrain gate: write failed", "error", err)
		}
	}
	t.applied = &disabled
	t.g.log.Info("terrain gate", "water_tools_disabled", disabled)
	return true
}

// Shutdown puts the original instruction back.
func (t *TerrainGate) Shutdown() {
	if t.orig == nil {
		return
	}
	for _, a := range t.addrs {
		if head, err := t.g.proc.Read(a, 8); err == nil && bytes.Equal(head, terrainMain2[:8]) {
			_ = t.g.proc.Write(a+waterToolsByte, t.orig)
		}
	}
	t.addrs, t.orig, t.applied = nil, nil, nil
}

// ExhibitInfoPopUp.lua decides which exhibit enrichment tiers show and which
// of their items read as applied, from a map the client cannot write. Two
// windows of its bytecode are rewritten to compare the tier against the
// received Progressive Exhibit Enrichment count instead.
var (
	exhibitMain54 = mustHex(
		"460040004740c0008600400087804001c7c0400001010100a400800164400000460040004780c000" +
			"87c04000c1000100648080018600400087404101c00080000181010041c10100a44000028b000000" +
			"c1000200074142001c01000241010200e8800280c7814200c7c1c203cc01c3034742420047828104" +
			"e48180011c0280035c020001200082041e00008080008003e7c0fc7fc50080000781420007c14202" +
			"0c41430224010001e40001001e00028007c201011f8043041e00008003420000030280004cc24300" +
			"c00280030003000464420002e98000006a01fd7f26008000")
	exhibitMain32 = mustHex(
		"470040001f40c0001e0000802600800047804000624000001e00018047c040004700c10062400000" +
			"1e000080414001008780410087400001c7c04100c700c201cc40c20147814200e480800106c14200" +
			"070143024741430081810300c1c103002481000247c140005c0180026100c1021e00008043410000" +
			"4301800086c1420087014403c00100020142040041820400620100001e80008087c24401a2420000" +
			"1e00008087024501c14205005dc28204a441000286c1420087014403c00100020182050062010000" +
			"1e80008047c24501624200001e00008047024601a441000286c1420087014303c741430001820300" +
			"41420600a4810002c6c14200c701c403000200034182060087c2c601e4410002c7c14100c701c203" +
			"cc01c70347824200e481800107c24100070242040c42470480028000248280014302800085028000" +
			"c7c24100c702c205cc82c705e4020001a40201001ec00b80c78303045f40c0071e800a80c6c34200" +
			"c703c3070744430041c4070081040800c00400079dc40409c1440800e48380020504800047c44100" +
			"4704c2084c84c808c00480000005000764040002240401001e40058046c542004705c30a80058007" +
			"c1c508000706490add05860b6485800186c542008705440bc005800a014609004706490a47468603" +
			"5f40c00c1e00018041c608008706490a5d86860c624600001e00008041460100a445000229840000" +
			"aac4f97f1e400080430200001e400080a98200002a43f37f0ac0499326008000")
)

const exhibitMaxTier = 3

func abc(op, a, b, c uint32) uint32 { return op | (a&0xFF)<<6 | (c&0x1FF)<<14 | (b&0x1FF)<<23 }

func luaJmp(sbx int32) uint32 { return 30 | uint32((sbx+0x1FFFF)&0x3FFFF)<<14 }

func packIns(ins ...uint32) []byte {
	var out []byte
	for _, i := range ins {
		out = binary.LittleEndian.AppendUint32(out, i)
	}
	return out
}

// variant54 computes R8 = (R7 <= n) over main.54's pc 44-48.
func variant54(n int) []byte {
	switch {
	case n <= 0:
		return packIns(abc(3, 8, 0, 0), luaJmp(0), luaJmp(0), luaJmp(0), luaJmp(0))
	case n == 1:
		return packIns(abc(31, 1, 7, 0x108), luaJmp(1), abc(3, 8, 0, 1), abc(3, 8, 1, 0), luaJmp(0))
	case n == 2:
		return packIns(abc(13, 8, 0x108, 0x108), abc(33, 1, 7, 8), luaJmp(1), abc(3, 8, 0, 1), abc(3, 8, 1, 0))
	}
	return packIns(abc(3, 8, 1, 0), luaJmp(0), luaJmp(0), luaJmp(0), luaJmp(0))
}

// variant32 breaks main.32's level loop at the first tier above n.
func variant32(n int) []byte {
	switch {
	case n <= 0:
		return packIns(luaJmp(45), luaJmp(0), luaJmp(0))
	case n == 1:
		return packIns(abc(1, 15, 0, 4), abc(33, 0, 14, 15), luaJmp(43))
	case n == 2:
		return packIns(abc(13, 15, 0x104, 0x104), abc(33, 0, 14, 15), luaJmp(43))
	}
	return packIns(luaJmp(2), luaJmp(0), luaJmp(0))
}

type enrichSite struct {
	name    string
	code    []byte
	off     int
	size    int
	variant func(int) []byte
}

var enrichSites = []enrichSite{
	{name: "main.54", code: exhibitMain54, off: 44 * 4, size: 5 * 4, variant: variant54},
	{name: "main.32", code: exhibitMain32, off: 86 * 4, size: 3 * 4, variant: variant32},
}

func (s enrichSite) prefix() []byte { return s.code[:s.off] }

func (s enrichSite) orig() []byte { return s.code[s.off : s.off+s.size] }

// known is every pattern the window may hold: the original or one of ours.
// Anything else is left alone.
func (s enrichSite) known(b []byte) bool {
	if bytes.Equal(b, s.orig()) {
		return true
	}
	for n := range exhibitMaxTier + 1 {
		if bytes.Equal(b, s.variant(n)) {
			return true
		}
	}
	return false
}

// EnrichGate holds exhibit enrichment tiers to the received count.
type EnrichGate struct {
	g       *Game
	find    bytecodeFinder
	addrs   map[string][]uint64
	applied int
}

func (g *Game) NewEnrichGate() *EnrichGate { return &EnrichGate{g: g, applied: -1} }

func (e *EnrichGate) located() bool {
	if e.addrs != nil && e.valid() {
		return true
	}
	if !e.find.due() {
		return false
	}
	e.applied = -1
	found := map[string][]uint64{}
	for _, s := range enrichSites {
		addrs, err := mem.FindBytes(e.g.proc, s.prefix())
		if err != nil || len(addrs) == 0 {
			e.addrs = nil
			e.find.failed()
			return false
		}
		found[s.name] = addrs
	}
	e.addrs = found
	e.g.log.Info("exhibit enrichment gate: found the exhibit panel bytecode")
	return true
}

func (e *EnrichGate) valid() bool {
	for _, s := range enrichSites {
		for _, a := range e.addrs[s.name] {
			if head, err := e.g.proc.Read(a, 8); err != nil || !bytes.Equal(head, s.code[:8]) {
				return false
			}
		}
	}
	return true
}

// Reconcile shows tiers up to count.
func (e *EnrichGate) Reconcile(count int) bool {
	if !e.located() {
		return false
	}
	n := max(0, min(exhibitMaxTier, count))
	if e.applied == n {
		return true
	}
	for _, s := range enrichSites {
		blob := s.variant(n)
		for _, a := range e.addrs[s.name] {
			cur, err := e.g.proc.Read(a+uint64(s.off), s.size)
			if err != nil || !s.known(cur) {
				e.g.log.Warn("exhibit enrichment gate: unexpected bytes, leaving them", "site", s.name)
				continue
			}
			_ = e.g.proc.Write(a+uint64(s.off), blob)
		}
	}
	e.applied = n
	e.g.log.Info("exhibit enrichment gate", "tiers_unlocked", n)
	return true
}

// Shutdown restores the original bytecode.
func (e *EnrichGate) Shutdown() {
	if e.addrs == nil || !e.valid() {
		return
	}
	for _, s := range enrichSites {
		for _, a := range e.addrs[s.name] {
			if cur, err := e.g.proc.Read(a+uint64(s.off), s.size); err == nil && s.known(cur) && !bytes.Equal(cur, s.orig()) {
				_ = e.g.proc.Write(a+uint64(s.off), slices.Clone(s.orig()))
			}
		}
	}
	e.addrs, e.applied = nil, -1
}
