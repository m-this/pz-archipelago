package pz

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"

	"github.com/m-this/pz-archipelago/launcher/internal/game/mem"
)

// Site is a code address the client detours: where it is in the current
// build, the bytes it holds, and a signature that finds it after a patch
// moved it.
type Site struct {
	Name string
	RVA  uint64
	Orig []byte
	AOB  string
	// Frags are the original instructions in order, for a trampoline that
	// interleaves its own work between them; a leaked copy of it does not
	// hold the original contiguously.
	Frags [][]byte
}

func h(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

// The sites, from upstream's memory/signatures.py.
var (
	sitePermit = Site{
		Name: "permit", RVA: 0xA0894E5, Orig: h("0fb68310020000"),
		AOB: "0F B6 83 10 02 00 00 84 C0 74 64 4C 8B 83 08 02 00 00 49 F7 D8 49 8B 97 10 01 00 00",
	}
	siteRelease        = Site{Name: "release", RVA: 0x5D84690, Orig: h("48895c2410")}
	siteReleaseSpecies = Site{
		Name: "release_species", RVA: 0x5D8478F, Orig: h("488b4d484889f2"),
		Frags: [][]byte{h("488b4d48"), h("4889f2")},
		AOB: "48 8B 4D 48 48 89 F2 E8 ?? ?? ?? ?? 48 8B 5C 24 38 31 C0 48 8B 6C 24 40 " +
			"48 8B 74 24 48 48 83 C4 20 5F C3 CC A0 0F A2 C3 FF FF FF FF 48 C7 C2 8B B7 A8 A5",
	}
	siteBirthInsert = Site{
		Name: "birth_insert", RVA: 0xC82168, Orig: h("488bbdd8000000"),
		AOB: "48 8B BD D8 00 00 00 4D 85 F6 75 3E 44 38 B5 F8 00 00 00 75 2A 49 8B 8D F8 04 00 00",
	}
	siteResearchStart = Site{
		Name: "research_start", RVA: 0xE461C6, Orig: h("41c6474902"),
		AOB: "41 C6 47 49 02 48 8B 11 48 3B 51 10 72 11 48 8D 55 40 E8 ?? ?? ?? ?? 8B 75 40",
	}
	siteExhibitInsert = Site{
		Name: "exhibit_insert", RVA: 0xA31F20, Orig: h("488bc44c894820"),
		AOB: "48 8B C4 4C 89 48 20 4C 89 40 18 48 89 50 10 48 89 48 08 55 53 56 57 41 54 41 55 41 56 41 57 48 8D A8 08 F7 FF FF",
	}
	siteExhibitRelease = Site{
		Name: "exhibit_release", RVA: 0x6048940, Orig: h("4055535657"),
		AOB: "40 55 53 56 57 41 56 48 8D AC 24 20 FF FF FF 48 81 EC E0 01 00 00 48 8B 41 20 31 F6 48 8B " +
			"59 10 49 89 CA 48 89 DA 48 2B 10 48 B8 AB AA AA AA AA AA AA 2A",
	}
	siteExhibitReleaseSpecies = Site{
		Name: "exhibit_release_species", RVA: 0x6048A92, Orig: h("4d8bbef8000000"),
		AOB: "4D 8B BE F8 00 00 00 89 9D 10 01 00 00 4D 8D B7 A0 02 00 00",
	}
)

// Sites is every hook site, for the preflight report.
var Sites = []Site{
	sitePermit, siteRelease, siteReleaseSpecies, siteBirthInsert,
	siteResearchStart, siteExhibitInsert, siteExhibitRelease, siteExhibitReleaseSpecies,
}

func (g *Game) moduleSize() uint64 {
	_, size := g.proc.Module()
	if size == 0 {
		return 0x20000000
	}
	return size
}

// detourTarget is where a rel32 jmp at site goes, if site starts with one.
func (g *Game) detourTarget(site uint64) (uint64, bool) {
	head, err := g.proc.Read(site, 5)
	if err != nil || head[0] != 0xE9 {
		return 0, false
	}
	return uint64(int64(site) + 5 + int64(int32(binary.LittleEndian.Uint32(head[1:])))), true
}

// leaked reports whether site holds one of our own detours left by a
// process that did not restore it: a jmp to a trampoline that still holds
// the site's original instructions.
func (g *Game) leaked(s Site, site uint64) bool {
	target, ok := g.detourTarget(site)
	if !ok {
		return false
	}
	tramp, err := g.proc.Read(target, 0x200)
	if err != nil {
		return false
	}
	if bytes.Contains(tramp, s.Orig) {
		return true
	}
	if len(s.Frags) == 0 {
		return false
	}
	pos := 0
	for _, frag := range s.Frags {
		i := bytes.Index(tramp[pos:], frag)
		if i < 0 {
			return false
		}
		pos += i + len(frag)
	}
	return true
}

// dead reports whether site jumps out of the module to memory no longer
// mapped: a detour whose trampoline died with the client that placed it. No
// live hook can point there, so restoring the original is safe.
func (g *Game) dead(site uint64) bool {
	target, ok := g.detourTarget(site)
	if !ok || (target >= g.base && target < g.base+g.moduleSize()) {
		return false
	}
	_, err := g.proc.Read(target, 16)
	return err != nil
}

// recoverLeaked restores a site holding one of our own leaked or dead
// detours.
func (g *Game) recoverLeaked(s Site) bool {
	site := g.base + s.RVA
	if !g.leaked(s, site) && !g.dead(site) {
		return false
	}
	if err := g.proc.Write(site, s.Orig); err != nil {
		return false
	}
	g.log.Info("recovered a detour left by an earlier run", "hook", s.Name, "site", site)
	return true
}

// Resolve is where a site is now: its RVA if the original bytes are there,
// the RVA again after undoing our own leaked detour, or the one place its
// signature matches. ok is false when none holds.
func (g *Game) Resolve(s Site) (addr uint64, ok bool) {
	site := g.base + s.RVA
	if cur, err := g.proc.Read(site, len(s.Orig)); err == nil && bytes.Equal(cur, s.Orig) {
		return site, true
	}
	if g.recoverLeaked(s) {
		return site, true
	}
	if s.AOB != "" {
		if hits := mem.ScanModule(g.proc, mem.MustPattern(s.AOB), 2); len(hits) == 1 {
			return hits[0], true
		}
	}
	return 0, false
}

// The two anchor roots are not referenced from code directly; a unique code
// reference to a global next to them is, and the cluster moves as a block
// when a patch shifts it.
const (
	rootCash  = 0x2944690
	rootGuest = 0x29446A0
)

var (
	rootClusterAOB = mem.MustPattern("48 8D 0D ?? ?? ?? ?? E8 ?? ?? ?? ?? 48 8B D0 48 85 C0 74 15 33 C0 89 42 08")
	rootDelta      = map[uint64]uint64{rootCash: 0x240, rootGuest: 0x250}
)

const (
	rootLEADisp = 3
	rootLEALen  = 7
)

// root is the address of an anchor-chain root global, found through the
// cluster signature, else at its RVA. Cached for the process.
func (g *Game) root(rva uint64) uint64 {
	delta, known := rootDelta[rva]
	if !known {
		return g.base + rva
	}
	if addr, ok := g.roots[rva]; ok {
		return addr
	}
	addr := g.base + rva
	if hits := mem.ScanModule(g.proc, rootClusterAOB, 2); len(hits) == 1 {
		if disp, err := mem.I32(g.proc, hits[0]+rootLEADisp); err == nil {
			addr = uint64(int64(hits[0])+rootLEALen+int64(disp)) + delta
		}
	}
	g.roots[rva] = addr
	return addr
}

// The park: *(*(root)+0x20)+0x658, an embedded object.
const (
	parkFirst  = 0x20
	parkOffset = 0x658
)

// Park is the loaded park object, or 0 when no zoo is loaded.
func (g *Game) Park() uint64 {
	root := g.root(rootGuest)
	return mem.Chain(g.proc, root, 0, parkFirst, parkOffset)
}
