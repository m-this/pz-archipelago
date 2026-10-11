package gamedata

import "slices"

// Interactivity is how guests meet a species: in a habitat behind barriers,
// or in an exhibit. Exhibit species have one welfare research level fewer.
type Interactivity uint8

const (
	Habitat Interactivity = iota + 1
	Exhibit
)

type Species struct {
	Key   SpeciesKey
	Label string
	// EngineToken is the name the game interns for the species. The client
	// matches births, acquisitions and welfare research against it.
	EngineToken   string
	Interactivity Interactivity
	WaterNeeded   bool
	FenceGrade    int8
	Domesticated  bool
}

// WelfareLevels is how many welfare research levels the species has, each
// one a location.
func (s Species) WelfareLevels() int8 {
	if s.Interactivity == Exhibit {
		return 5
	}
	return 6
}

// SpeciesByKey looks a species up. There are 78; a scan is the fast path.
func SpeciesByKey(key SpeciesKey) (Species, bool) {
	i := slices.IndexFunc(AllSpecies, func(s Species) bool { return s.Key == key })
	if i < 0 {
		return Species{}, false
	}
	return AllSpecies[i], true
}
