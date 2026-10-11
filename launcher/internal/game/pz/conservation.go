package pz

import (
	"maps"
	"slices"

	"github.com/m-this/pz-archipelago/gamedata"
)

// A pending exhibit release with no species found after this many ticks is
// given up on; the release still counts.
const exhibitGiveUpTicks = 20

// Conservation works out which species each release to the wild released.
// A habitat release is attributed by the handle its capture recorded. An
// exhibit release from storage is attributed by the id its capture
// recorded, or by the id leaving the owned set; one from a habitat by the
// placed census dropping, which happens a few seconds after the release.
// Upstream's memory/triggers.py.
type Conservation struct {
	g        *Game
	births   *Births
	releases *Releases
	Released map[gamedata.SpeciesKey]bool

	lastHabitat   uint32
	lastExhibit   uint32
	baseline      map[uint32]uint32
	pending       int
	pendingTicks  int
	prevIDs       map[uint32]bool
	idSpecies     map[uint32]gamedata.SpeciesKey
	captureCursor uint32
	captured      map[uint32]bool
	unresolved    map[uint32]bool
	warnedHabitat bool
}

func (g *Game) NewConservation(births *Births, releases *Releases) *Conservation {
	return &Conservation{
		g: g, births: births, releases: releases, Released: map[gamedata.SpeciesKey]bool{},
		idSpecies: map[uint32]gamedata.SpeciesKey{}, captured: map[uint32]bool{}, unresolved: map[uint32]bool{},
	}
}

// Poll updates Released.
func (c *Conservation) Poll() {
	c.births.SweepRoster()
	if n := c.releases.HabitatCount(); n > c.lastHabitat {
		c.lastHabitat = n
		if key, ok := c.attributeHabitat(); ok {
			c.Released[key] = true
		} else if !c.warnedHabitat {
			c.warnedHabitat = true
			c.g.log.Info("a habitat release could not be attributed to a species; its conservation check will not fire")
		}
	}
	c.pollExhibit()
}

func (c *Conservation) attributeHabitat() (gamedata.SpeciesKey, bool) {
	handle := c.releases.LastHandle()
	if handle == 0 {
		return "", false
	}
	if key, ok := c.births.Species[handle]; ok {
		return key, true
	}
	snap, ok := c.g.Research.Snapshot()
	if !ok {
		return "", false
	}
	keys := c.g.Research.HandleKeys(snap)
	var entities []uint64
	if mgr := c.releases.LastManager(); mgr != 0 {
		if e, ok := c.g.Animals.Entity(mgr, handle); ok {
			entities = append(entities, e)
		}
		if e, ok := c.g.Animals.EntityByManager(mgr, handle); ok {
			entities = append(entities, e)
		}
	}
	if c.births.lastZoo != 0 {
		if e, ok := c.g.Animals.Entity(c.births.lastZoo, handle); ok {
			entities = append(entities, e)
		}
	}
	for _, e := range entities {
		if sh, ok := c.g.Animals.SpeciesHandle(e); ok {
			if key, ok := keys[sh]; ok {
				return key, true
			}
		}
	}
	return "", false
}

func (c *Conservation) pollExhibit() {
	n := c.releases.ExhibitCount()
	if n > c.lastExhibit {
		c.pending += int(n - c.lastExhibit)
		c.g.log.Info("exhibit release detected", "count", n)
	}
	c.lastExhibit = n
	c.drainCaptures()
	c.diffIDs()
	census, ok := c.g.Animals.ExhibitCensus(c.g.Animals.ExhibitManager())
	if ok {
		if c.baseline == nil {
			c.baseline = census
		} else if c.pending > 0 {
			c.attributeDrops(census)
		}
	}
	c.retire(census, ok)
}

func (c *Conservation) holder() uint64 {
	if h := c.releases.ExhibitHolder(); h != 0 {
		return h
	}
	return c.g.Animals.DefHolder(c.g.Animals.ExhibitManager())
}

func (c *Conservation) speciesOf(def ExhibitDef, ok bool) (gamedata.SpeciesKey, bool) {
	if !ok {
		return "", false
	}
	if def.Handle != 0 {
		if snap, ok := c.g.Research.Snapshot(); ok {
			if key, ok := c.g.Research.HandleKeys(snap)[def.Handle]; ok {
				return key, true
			}
		}
	}
	for _, name := range def.Names {
		if key, ok := c.g.Research.SpeciesForName(name); ok {
			return key, true
		}
	}
	return "", false
}

func (c *Conservation) drainCaptures() {
	cursor, ids := c.releases.ExhibitEvents(c.captureCursor)
	c.captureCursor = cursor
	retry := slices.Sorted(maps.Keys(c.unresolved))
	clear(c.unresolved)
	var defs map[uint32]ExhibitDef
	attempt := func(id uint32) {
		key, ok := c.idSpecies[id]
		if !ok {
			if defs == nil {
				defs, _ = c.g.Animals.ExhibitDefs(c.holder())
				if defs == nil {
					defs = map[uint32]ExhibitDef{}
				}
			}
			def, found := defs[id]
			key, ok = c.speciesOf(def, found)
		}
		switch {
		case ok && c.pending > 0:
			c.Released[key] = true
			c.pending--
			c.captured[id] = true
			delete(c.idSpecies, id)
			c.g.log.Info("exhibit storage release attributed", "species", key)
		case c.pending > 0:
			c.unresolved[id] = true
		}
	}
	for _, id := range retry {
		attempt(id)
	}
	for _, id := range ids {
		attempt(id)
	}
}

func (c *Conservation) diffIDs() {
	holder := c.holder()
	if holder == 0 {
		return
	}
	ids, ok := c.g.Animals.ExhibitIDs(holder)
	if !ok {
		return
	}
	var fresh []uint32
	for id := range ids {
		if _, known := c.idSpecies[id]; !known {
			fresh = append(fresh, id)
		}
	}
	if len(fresh) > 0 {
		defs, _ := c.g.Animals.ExhibitDefs(holder)
		for _, id := range fresh {
			def, found := defs[id]
			if key, ok := c.speciesOf(def, found); ok {
				c.idSpecies[id] = key
			}
		}
	}
	if c.prevIDs != nil {
		for id := range c.prevIDs {
			if ids[id] {
				continue
			}
			key, known := c.idSpecies[id]
			delete(c.idSpecies, id)
			if c.captured[id] {
				delete(c.captured, id)
				continue
			}
			if c.pending > 0 && known {
				c.Released[key] = true
				c.pending--
				c.g.log.Info("exhibit release attributed by its id leaving", "species", key)
			}
		}
	}
	c.prevIDs = ids
}

func (c *Conservation) attributeDrops(census map[uint32]uint32) {
	snap, ok := c.g.Research.Snapshot()
	if !ok {
		return
	}
	keys := c.g.Research.HandleKeys(snap)
	for _, handle := range slices.Sorted(maps.Keys(c.baseline)) {
		base := c.baseline[handle]
		for census[handle] < base && c.pending > 0 {
			if key, ok := keys[handle]; ok {
				c.Released[key] = true
				c.g.log.Info("exhibit release attributed by the census", "species", key)
			}
			base--
			c.baseline[handle] = base
			c.pending--
		}
	}
}

func (c *Conservation) retire(census map[uint32]uint32, ok bool) {
	if c.pending > 0 {
		c.pendingTicks++
		if c.pendingTicks >= exhibitGiveUpTicks {
			c.g.log.Info("an exhibit release could not be attributed to a species; its conservation check will not fire")
			c.pending = 0
		}
	}
	if c.pending == 0 {
		c.pendingTicks = 0
		clear(c.unresolved)
		if ok {
			c.baseline = census
		}
	}
}
