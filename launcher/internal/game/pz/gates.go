package pz

import (
	"maps"
	"slices"

	"github.com/m-this/pz-archipelago/gamedata"
	"github.com/m-this/pz-archipelago/launcher/internal/game/hook"
	"github.com/m-this/pz-archipelago/launcher/internal/game/mem"
)

// ResearchGate stops research of a category from starting until its
// facility arrives: animal research needs the Research Centre, mechanic
// research the Workshop. The detour skips the status write that starts it.
type ResearchGate struct {
	g       *Game
	scratch uint64
	last    []uint8
	written bool
}

// The record register at the research-start site is r15.
const researchStartCatModRM = 0x4F

func (g *Game) NewResearchGate() *ResearchGate { return &ResearchGate{g: g} }

func (r *ResearchGate) ensure() bool {
	if r.scratch != 0 {
		return true
	}
	site, ok := r.g.Resolve(siteResearchStart)
	if !ok {
		r.g.log.Warn("research gate: the start site is not where it was, the game may have been patched")
		return false
	}
	orig := siteResearchStart.Orig
	if err := r.g.hooks.Install(siteResearchStart.Name, site, orig, func(rg, sc, res uint64) []byte {
		return hook.ResearchGate(rg, sc, res, orig, researchStartCatModRM)
	}); err != nil {
		r.g.log.Warn("research gate not installed", "error", err)
		return false
	}
	r.scratch, _ = r.g.hooks.Scratch(siteResearchStart.Name)
	r.g.log.Info("research gate installed", "site", site)
	return true
}

// Reconcile blocks the categories whose facility has not arrived, and puts
// any of their research already running back to researchable.
func (r *ResearchGate) Reconcile(researchCentre, workshop bool) bool {
	var gated []uint8
	if !workshop {
		gated = append(gated, CategoryMechanic)
	}
	if !researchCentre {
		gated = append(gated, CategoryAnimal)
	}
	if !r.ensure() {
		return false
	}
	if r.written && slices.Equal(gated, r.last) {
		return true
	}
	for i, c := range gated {
		if err := mem.PutU8(r.g.proc, r.scratch+hook.ResearchGateCats+uint64(i), c); err != nil {
			r.g.log.Warn("research gate: cannot write the gated categories", "error", err)
			return false
		}
	}
	if err := mem.PutU32(r.g.proc, r.scratch+hook.ResearchGateCount, uint32(len(gated))); err != nil {
		return false
	}
	r.stopRunning(gated)
	r.last, r.written = gated, true
	cats := make([]int, len(gated))
	for i, c := range gated {
		cats[i] = int(c)
	}
	r.g.log.Info("research gate", "gated_categories", cats)
	return true
}

func (r *ResearchGate) stopRunning(gated []uint8) {
	if len(gated) == 0 {
		return
	}
	snap, ok := r.g.Research.Snapshot()
	if !ok {
		return
	}
	for _, rec := range snap.Records() {
		if slices.Contains(gated, rec.Category) && rec.Status == statusResearching {
			_ = r.g.Research.SetStatus(rec, statusResearchable)
		}
	}
}

// PermitGate aborts the purchase of any species whose permit, or the Water
// Habitat Tools it needs, has not arrived. The detour compares the
// listing's species handle against the blocked set the client writes.
type PermitGate struct {
	g          *Game
	scratch    uint64
	blocked    map[gamedata.SpeciesKey]bool
	synced     bool
	unresolved int
}

// The fail-return of the purchase function, from the hook site.
const permitFailDelta = 0x2E

func (g *Game) NewPermitGate() *PermitGate { return &PermitGate{g: g} }

func (p *PermitGate) ensure() bool {
	if p.scratch != 0 {
		return true
	}
	site, ok := p.g.Resolve(sitePermit)
	if !ok {
		p.g.log.Warn("permit gate: the purchase site is not where it was, the game may have been patched")
		return false
	}
	orig := sitePermit.Orig
	fail := site - permitFailDelta
	if err := p.g.hooks.Install(sitePermit.Name, site, orig, func(rg, sc, res uint64) []byte {
		return hook.Permit(rg, sc, res, fail, orig)
	}); err != nil {
		p.g.log.Warn("permit gate not installed", "error", err)
		return false
	}
	p.scratch, _ = p.g.hooks.Scratch(sitePermit.Name)
	p.g.log.Info("permit gate installed", "site", site)
	return true
}

// Installed reports whether the purchase detour is in place, installing it
// if it can.
func (p *PermitGate) Installed() bool { return p.ensure() }

// Reconcile blocks exactly the species in blocked. The market gate keeps
// species off the market meanwhile.
func (p *PermitGate) Reconcile(blocked map[gamedata.SpeciesKey]bool) bool {
	if !p.ensure() {
		return false
	}
	// A species not in the research map yet cannot be blocked by handle; the
	// set is rewritten every tick until all of them resolve.
	if p.synced && maps.Equal(blocked, p.blocked) && p.unresolved == 0 {
		return true
	}
	snap, ok := p.g.Research.Snapshot()
	if !ok {
		return false
	}
	var handles []uint32
	unresolved := 0
	for _, key := range slices.Sorted(maps.Keys(blocked)) {
		if h, ok := p.g.Research.Handle(key, snap); ok {
			handles = append(handles, h)
		} else {
			unresolved++
		}
	}
	if len(handles) > hook.PermitBlockedMax {
		handles = handles[:hook.PermitBlockedMax]
	}
	for i, h := range handles {
		if err := mem.PutU32(p.g.proc, p.scratch+hook.PermitScratchIDs+uint64(i)*4, h); err != nil {
			return false
		}
	}
	if err := mem.PutU32(p.g.proc, p.scratch+hook.PermitScratchCount, uint32(len(handles))); err != nil {
		return false
	}
	if unresolved != p.unresolved {
		p.unresolved = unresolved
		p.g.log.Info("permit gate: species not in this zoo's research map yet are kept off the market instead",
			"blocked_by_handle", len(handles), "not_resolvable", unresolved)
	}
	p.blocked, p.synced = maps.Clone(blocked), true
	return true
}
