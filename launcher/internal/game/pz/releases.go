package pz

import (
	"github.com/m-this/pz-archipelago/launcher/internal/game/hook"
	"github.com/m-this/pz-archipelago/launcher/internal/game/mem"
)

// Releases gates and counts releases to the wild. Habitat and exhibit
// animals release through two separate functions; each gets the same gate,
// aborting the release while the Conservation Program is not received. Two
// more detours capture which animal was released, for the per-species
// checks.
type Releases struct {
	g              *Game
	installed      bool
	locked         bool
	habitat        uint64
	species        uint64
	exhibit        uint64
	exhibitSpecies uint64
}

func (g *Game) NewReleases() *Releases { return &Releases{g: g, locked: true} }

func (r *Releases) install(s Site, build func(orig []byte) hook.Builder) uint64 {
	site, ok := r.g.Resolve(s)
	if !ok {
		r.g.log.Info("release site not found, part of release detection is off", "hook", s.Name)
		return 0
	}
	if err := r.g.hooks.Install(s.Name, site, s.Orig, build(s.Orig)); err != nil {
		r.g.log.Info("release hook not installed", "hook", s.Name, "error", err)
		return 0
	}
	scratch, _ := r.g.hooks.Scratch(s.Name)
	return scratch
}

func gateBuilder(orig []byte) hook.Builder {
	return func(r, sc, res uint64) []byte { return hook.ReleaseGate(r, sc, res, orig) }
}

func (r *Releases) ensure() bool {
	if r.installed {
		return true
	}
	r.habitat = r.install(siteRelease, gateBuilder)
	if r.habitat == 0 {
		return false
	}
	r.installed = true
	r.species = r.install(siteReleaseSpecies, func(orig []byte) hook.Builder {
		return func(rg, sc, res uint64) []byte { return hook.ReleaseSpeciesCapture(rg, sc, res, orig) }
	})
	r.exhibit = r.install(siteExhibitRelease, gateBuilder)
	r.exhibitSpecies = r.install(siteExhibitReleaseSpecies, func(orig []byte) hook.Builder {
		return func(rg, sc, res uint64) []byte { return hook.ExhibitReleaseCapture(rg, sc, res, orig) }
	})
	r.writeLock()
	r.g.log.Info("release gate installed", "locked", r.locked, "species_capture", r.species != 0, "exhibit_gate", r.exhibit != 0)
	return true
}

func (r *Releases) writeLock() {
	v := uint32(0)
	if r.locked {
		v = 1
	}
	for _, sc := range []uint64{r.habitat, r.exhibit} {
		if sc != 0 {
			if err := mem.PutU32(r.g.proc, sc+hook.ReleaseScratchLock, v); err != nil {
				r.g.log.Warn("cannot write the release gate", "error", err)
			}
		}
	}
}

// SetLocked opens the gate once the Conservation Program arrives.
func (r *Releases) SetLocked(locked bool) {
	changed := locked != r.locked
	r.locked = locked
	if r.ensure() && changed {
		r.writeLock()
		r.g.log.Info("conservation release gate", "locked", locked)
	}
}

func (r *Releases) count(sc uint64) uint32 {
	if sc == 0 {
		return 0
	}
	n, err := mem.U32(r.g.proc, sc+hook.ReleaseScratchCount)
	if err != nil {
		return 0
	}
	return n
}

// HabitatCount and ExhibitCount are the releases seen this session.
func (r *Releases) HabitatCount() uint32 {
	if !r.ensure() {
		return 0
	}
	return r.count(r.habitat)
}

func (r *Releases) ExhibitCount() uint32 {
	if !r.ensure() {
		return 0
	}
	return r.count(r.exhibit)
}

// LastHandle and LastManager are what the habitat capture last recorded.
func (r *Releases) LastHandle() uint64 {
	if r.species == 0 {
		return 0
	}
	return mem.Ptr(r.g.proc, r.species+hook.ReleaseSpeciesHandle)
}

func (r *Releases) LastManager() uint64 {
	if r.species == 0 {
		return 0
	}
	return mem.Ptr(r.g.proc, r.species+hook.ReleaseSpeciesManager)
}

// ExhibitHolder is the def-map holder the storage capture last saw.
func (r *Releases) ExhibitHolder() uint64 {
	if r.exhibitSpecies == 0 {
		return 0
	}
	return mem.Ptr(r.g.proc, r.exhibitSpecies+hook.ExhibitReleaseHolder)
}

// ExhibitEvents drains the released exhibit animal ids since cursor.
func (r *Releases) ExhibitEvents(cursor uint32) (uint32, []uint32) {
	if r.exhibitSpecies == 0 {
		return cursor, nil
	}
	var ids []uint32
	count, err := drain(r.g.proc, r.exhibitSpecies, cursor, hook.ExhibitReleaseRing, func(slot uint32) error {
		id, err := mem.U32(r.g.proc, r.exhibitSpecies+hook.ExhibitReleaseRingOff+uint64(slot)*4)
		ids = append(ids, id)
		return err
	})
	if err != nil {
		return cursor, nil
	}
	return count, ids
}
