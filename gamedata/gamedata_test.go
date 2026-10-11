package gamedata

import (
	"slices"
	"strings"
	"testing"
)

func TestTablesAreValid(t *testing.T) {
	if err := Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestLocationNamesAreUnique(t *testing.T) {
	names := map[string]LocationID{}
	for _, l := range Locations {
		if other, dup := names[l.LocationName()]; dup {
			t.Errorf("locations %d and %d are both %q", other, l.ID, l.LocationName())
		}
		names[l.LocationName()] = l.ID
	}
}

func TestDerivedNamesMatchTheApworld(t *testing.T) {
	cases := map[LocationID][2]string{
		24000: {"Aardvark - Research Welfare 1", "welfare1_aardvark"},
		23000: {"Aardvark - Conservation Release", "cr_aardvark"},
		22000: {"Aardvark - First Acquisition", "fa_aardvark"},
		20000: {"Guests - 1000", "guests_1000"},
	}
	for _, l := range Locations {
		want, ok := cases[l.ID]
		if !ok {
			continue
		}
		if got := [2]string{l.LocationName(), l.StringID()}; got != want {
			t.Errorf("location %d: %v, want %v", l.ID, got, want)
		}
	}
	for _, l := range Locations {
		if l.Kind == WelfareResearch {
			s, _ := SpeciesByKey(l.Species)
			if (s.Interactivity == Exhibit) != strings.HasPrefix(l.StringID(), "e_") {
				t.Errorf("location %d: %s", l.ID, l.StringID())
			}
		}
	}
}

// Each case breaks one invariant on a copy of the real tables.
func TestValidateRefuses(t *testing.T) {
	cases := map[string]func(*tables){
		"a duplicate item id": func(tb *tables) { tb.items[1].ID = tb.items[0].ID },
		"a permit for an unknown species": func(tb *tables) {
			i := slices.IndexFunc(tb.items, func(it Item) bool { return it.Effect.Kind == Permit })
			tb.items[i].Effect.Species = "dodo"
		},
		"an effect with a stray field": func(tb *tables) {
			i := slices.IndexFunc(tb.items, func(it Item) bool { return it.Effect.Kind == WaterTools })
			tb.items[i].Effect.Content = "x"
		},
		"a species without its release": func(tb *tables) {
			tb.locations = slices.DeleteFunc(tb.locations, func(l Location) bool { return l.Kind == ConservationRelease && l.Species == "aardvark" })
		},
		"a welfare level the species does not have": func(tb *tables) {
			i := slices.IndexFunc(tb.locations, func(l Location) bool {
				s, _ := SpeciesByKey(l.Species)
				return l.Kind == WelfareResearch && s.Interactivity == Exhibit
			})
			tb.locations[i].Level = 6
		},
		"a released domesticated species": func(tb *tables) {
			i := slices.IndexFunc(tb.species, func(s Species) bool { return s.Key == "aardvark" })
			tb.species[i].Domesticated = true
		},
		"a milestone with a species": func(tb *tables) {
			i := slices.IndexFunc(tb.locations, func(l Location) bool { return l.Kind == Milestone })
			tb.locations[i].Species = "aardvark"
		},
	}
	for name, breakIt := range cases {
		t.Run(name, func(t *testing.T) {
			tb := tables{species: slices.Clone(AllSpecies), items: slices.Clone(Items), locations: slices.Clone(Locations)}
			breakIt(&tb)
			if err := tb.validate(); err == nil {
				t.Error("validate accepted it")
			}
		})
	}
}
