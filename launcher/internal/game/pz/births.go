package pz

import (
	"github.com/m-this/pz-archipelago/gamedata"
	"github.com/m-this/pz-archipelago/launcher/internal/game/hook"
)

// Births detects habitat animals entering the zoo, through a detour on the
// add-animal insert, and tells a birth (a newborn) from an acquisition.
type Births struct {
	g       *Game
	scratch uint64
	cursor  uint32
	// lastZoo is the zoo object the insert last saw; releases resolve
	// handles through it.
	lastZoo uint64
	// Species stores {handle: species} of every animal seen entering or on
	// the roster: a released animal is gone from the roster before the next
	// poll, so its species has to be known already.
	Species map[uint64]gamedata.SpeciesKey
	unknown map[uint32]bool
}

func (g *Game) NewBirths() *Births {
	return &Births{g: g, Species: map[uint64]gamedata.SpeciesKey{}, unknown: map[uint32]bool{}}
}

func (b *Births) ensure() bool {
	if b.scratch != 0 {
		return true
	}
	site, ok := b.g.Resolve(siteBirthInsert)
	if !ok {
		b.g.log.Warn("birth detector: the insert site is not where it was, the game may have been patched")
		return false
	}
	orig := siteBirthInsert.Orig
	if err := b.g.hooks.Install(siteBirthInsert.Name, site, orig, func(r, sc, res uint64) []byte {
		return hook.InsertInstrument(r, sc, res, orig)
	}); err != nil {
		b.g.log.Warn("birth detector not installed", "error", err)
		return false
	}
	b.scratch, _ = b.g.hooks.Scratch(siteBirthInsert.Name)
	b.g.log.Info("birth detector installed", "site", site)
	return true
}

// Poll drains the inserts since the last poll.
func (b *Births) Poll() (born, acquired []gamedata.SpeciesKey) {
	if !b.ensure() {
		return nil, nil
	}
	type insert struct{ handle, zoo uint64 }
	var events []insert
	cursor, err := drain(b.g.proc, b.scratch, b.cursor, hook.InsertRing, func(slot uint32) error {
		raw, err := b.g.proc.Read(b.scratch+hook.InsertRingOff+uint64(slot)*hook.InsertRec, hook.InsertRec)
		if err != nil {
			return err
		}
		events = append(events, insert{handle: u64At(raw, 0), zoo: u64At(raw, 0x30)})
		return nil
	})
	if err != nil {
		return nil, nil
	}
	b.cursor = cursor
	if len(events) == 0 {
		return nil, nil
	}
	snap, ok := b.g.Research.Snapshot()
	if !ok {
		return nil, nil
	}
	keys := b.g.Research.HandleKeys(snap)
	for _, e := range events {
		if e.zoo != 0 {
			b.lastZoo = e.zoo
		}
		entity, ok := b.g.Animals.Entity(e.zoo, e.handle)
		if !ok {
			continue
		}
		sh, ok := b.g.Animals.SpeciesHandle(entity)
		if !ok {
			continue
		}
		key, ok := keys[sh]
		if !ok {
			if !b.unknown[sh] {
				b.unknown[sh] = true
				b.g.log.Info("an animal of a species the research map does not hold entered", "species_handle", sh)
			}
			continue
		}
		b.Species[e.handle] = key
		if b.g.Animals.Newborn(entity) {
			b.g.log.Info("birth", "species", key)
			born = append(born, key)
		} else {
			b.g.log.Info("acquisition", "species", key)
			acquired = append(acquired, key)
		}
	}
	return born, acquired
}

// SweepRoster records the species of every owned animal, so a release can be
// attributed after the animal has left.
func (b *Births) SweepRoster() {
	mgr := b.g.Animals.Manager()
	if mgr == 0 {
		return
	}
	snap, ok := b.g.Research.Snapshot()
	if !ok {
		return
	}
	keys := b.g.Research.HandleKeys(snap)
	b.g.Animals.Roster(mgr, func(handle, entity uint64) {
		if sh, ok := b.g.Animals.SpeciesHandle(entity); ok {
			if key, ok := keys[sh]; ok {
				b.Species[handle] = key
			}
		}
	})
}

// Exhibits detects exhibit animals entering, through a detour on their
// construct entry: purchase and birth both pass there, told apart by the
// classifier the detour records.
type Exhibits struct {
	g       *Game
	scratch uint64
	cursor  uint32
	unknown map[string]bool
}

func (g *Game) NewExhibits() *Exhibits { return &Exhibits{g: g, unknown: map[string]bool{}} }

func (e *Exhibits) ensure() bool {
	if e.scratch != 0 {
		return true
	}
	site, ok := e.g.Resolve(siteExhibitInsert)
	if !ok {
		e.g.log.Warn("exhibit detector: the construct site is not where it was, the game may have been patched")
		return false
	}
	orig := siteExhibitInsert.Orig
	if err := e.g.hooks.Install(siteExhibitInsert.Name, site, orig, func(r, sc, res uint64) []byte {
		return hook.ExhibitInstrument(r, sc, res, orig)
	}); err != nil {
		e.g.log.Warn("exhibit detector not installed", "error", err)
		return false
	}
	e.scratch, _ = e.g.hooks.Scratch(siteExhibitInsert.Name)
	e.g.log.Info("exhibit detector installed", "site", site)
	return true
}

// Poll drains the exhibit adds since the last poll.
func (e *Exhibits) Poll() (born, acquired []gamedata.SpeciesKey) {
	if !e.ensure() {
		return nil, nil
	}
	cursor, err := drain(e.g.proc, e.scratch, e.cursor, hook.ExhibitRing, func(slot uint32) error {
		raw, err := e.g.proc.Read(e.scratch+hook.ExhibitRingOff+uint64(slot)*hook.ExhibitRec, hook.ExhibitRec)
		if err != nil {
			return err
		}
		class := uint32(raw[0]) | uint32(raw[1])<<8 | uint32(raw[2])<<16 | uint32(raw[3])<<24
		name := raw[8 : 8+hook.ExhibitNameBytes]
		end := 0
		for end < len(name) && name[end] != 0 {
			end++
		}
		key, ok := e.g.Research.SpeciesForName(string(name[:end]))
		if !ok {
			if !e.unknown[string(name[:end])] {
				e.unknown[string(name[:end])] = true
				e.g.log.Info("an exhibit species token did not name a species", "token", string(name[:end]))
			}
			return nil
		}
		if class == hook.ExhibitBirth {
			e.g.log.Info("exhibit birth", "species", key)
			born = append(born, key)
		} else {
			e.g.log.Info("exhibit acquisition", "species", key)
			acquired = append(acquired, key)
		}
		return nil
	})
	if err == nil {
		e.cursor = cursor
	}
	return born, acquired
}
