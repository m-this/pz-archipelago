package gamedata

import (
	"errors"
	"fmt"
)

type tables struct {
	species   []Species
	items     []Item
	locations []Location
}

// Validate checks every invariant the tables rest on. The apworld and the
// client both trust them, so a broken table must fail here and not in a room.
func Validate() error {
	return tables{species: AllSpecies, items: Items, locations: Locations}.validate()
}

func (t tables) validate() error {
	var errs []error
	species := make(map[SpeciesKey]Species, len(t.species))
	for _, s := range t.species {
		if _, dup := species[s.Key]; dup {
			errs = append(errs, fmt.Errorf("species %q: declared twice", s.Key))
		}
		species[s.Key] = s
		if s.Label == "" || s.EngineToken == "" || s.Interactivity == 0 {
			errs = append(errs, fmt.Errorf("species %q: label, engine token and interactivity are required", s.Key))
		}
	}
	errs = append(errs, t.validateItems(species)...)
	errs = append(errs, t.validateLocations(species)...)
	return errors.Join(errs...)
}

func (t tables) validateItems(species map[SpeciesKey]Species) []error {
	var errs []error
	ids := map[ItemID]bool{}
	names := map[string]bool{}
	permits := map[SpeciesKey]int{}
	for _, it := range t.items {
		if ids[it.ID] || names[it.Name] {
			errs = append(errs, fmt.Errorf("item %d %q: id or name declared twice", it.ID, it.Name))
		}
		ids[it.ID], names[it.Name] = true, true
		if it.Classification == 0 || it.Quantity < 1 {
			errs = append(errs, fmt.Errorf("item %d: classification and a quantity of at least one are required", it.ID))
		}
		if err := it.Effect.validate(species); err != nil {
			errs = append(errs, fmt.Errorf("item %d: %w", it.ID, err))
		}
		if it.Effect.Kind == Permit {
			permits[it.Effect.Species]++
		}
	}
	for key := range species {
		if permits[key] != 1 {
			errs = append(errs, fmt.Errorf("species %q: %d permits, want one", key, permits[key]))
		}
	}
	return errs
}

// validate checks that the effect sets the field its kind names and no other.
func (e Effect) validate(species map[SpeciesKey]Species) error {
	want := Effect{Kind: e.Kind}
	switch e.Kind {
	case Permit:
		if _, ok := species[e.Species]; !ok {
			return fmt.Errorf("permit for unknown species %q", e.Species)
		}
		want.Species = e.Species
	case FacilityUnlock:
		want.Facility = e.Facility
	case Progressive:
		want.Family = e.Family
	case Cash, ConservationCredits:
		want.Size = e.Size
	case ResearchReward:
		want.Content = e.Content
	case WaterTools, ConservationProgram:
	default:
		return fmt.Errorf("unknown effect kind %d", e.Kind)
	}
	if e != want || (e.Kind == FacilityUnlock && e.Facility == 0) || (e.Kind == Progressive && e.Family == 0) ||
		(e.Kind == ResearchReward && e.Content == "") || ((e.Kind == Cash || e.Kind == ConservationCredits) && e.Size == 0) {
		return fmt.Errorf("effect %+v: sets a field its kind does not use, or misses the one it does", e)
	}
	return nil
}

type speciesCheck struct {
	kind  LocationKind
	key   SpeciesKey
	level int8
}

func (t tables) validateLocations(species map[SpeciesKey]Species) []error {
	var errs []error
	ids := map[LocationID]bool{}
	seen := map[speciesCheck]bool{}
	for _, l := range t.locations {
		if ids[l.ID] {
			errs = append(errs, fmt.Errorf("location %d: declared twice", l.ID))
		}
		ids[l.ID] = true
		if err := l.validate(species); err != nil {
			errs = append(errs, fmt.Errorf("location %d: %w", l.ID, err))
			continue
		}
		if l.Species != "" {
			check := speciesCheck{l.Kind, l.Species, l.Level}
			if seen[check] {
				errs = append(errs, fmt.Errorf("location %d: %+v declared twice", l.ID, check))
			}
			seen[check] = true
		}
	}
	for _, s := range species {
		for _, want := range expectedChecks(s) {
			if !seen[want] {
				errs = append(errs, fmt.Errorf("species %q: no location for %+v", s.Key, want))
			}
		}
	}
	return errs
}

// expectedChecks is every location a species must have: acquire it, breed
// it, release it unless it is domesticated, and research each welfare level.
func expectedChecks(s Species) []speciesCheck {
	checks := []speciesCheck{{FirstAcquisition, s.Key, 0}, {FirstBreeding, s.Key, 0}}
	if !s.Domesticated {
		checks = append(checks, speciesCheck{ConservationRelease, s.Key, 0})
	}
	for level := range s.WelfareLevels() {
		checks = append(checks, speciesCheck{WelfareResearch, s.Key, level + 1})
	}
	return checks
}

func (l Location) validate(species map[SpeciesKey]Species) error {
	switch l.Kind {
	case Milestone:
		if l.Metric == 0 || l.Threshold <= 0 || l.Species != "" || l.Name != "" {
			return errors.New("a milestone takes a metric and a threshold, nothing else")
		}
	case MechanicResearch:
		if l.Name == "" || l.Code == "" || l.ResearchKey == "" || l.Species != "" {
			return errors.New("mechanic research takes a name, a code and a research key, nothing else")
		}
	case FirstAcquisition, FirstBreeding, ConservationRelease, WelfareResearch:
		s, ok := species[l.Species]
		if !ok {
			return fmt.Errorf("unknown species %q", l.Species)
		}
		if l.Name != "" || l.Code != "" || l.ResearchKey != "" || l.Metric != 0 {
			return errors.New("a species location takes its species and a level, nothing else")
		}
		if (l.Kind == WelfareResearch) != (l.Level > 0) || l.Level > s.WelfareLevels() {
			return fmt.Errorf("level %d does not fit %s", l.Level, s.Key)
		}
		if l.Kind == ConservationRelease && s.Domesticated {
			return fmt.Errorf("%s is domesticated and cannot be released", s.Key)
		}
	default:
		return fmt.Errorf("unknown kind %d", l.Kind)
	}
	return nil
}
