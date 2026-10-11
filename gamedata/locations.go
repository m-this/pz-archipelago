package gamedata

import (
	"fmt"
	"strconv"
)

// LocationKind is what the player does in the game to send the check.
type LocationKind uint8

const (
	Milestone LocationKind = iota + 1
	MechanicResearch
	FirstAcquisition
	FirstBreeding
	ConservationRelease
	WelfareResearch
)

type Metric uint8

const (
	GuestCount Metric = iota + 1
	ZooRating
)

// Location holds the fields its Kind needs. A species location names its
// species and, for welfare research, the level; its name and its apworld
// string id follow from those.
type Location struct {
	ID      LocationID
	Kind    LocationKind
	Species SpeciesKey
	Level   int8

	Metric    Metric
	Threshold int32

	// A mechanic research location carries its own name, the apworld's code
	// for it, and the engine name of the research the client watches. The
	// apworld's codes are not unique: four staff facilities share one.
	Name        string
	Code        string
	ResearchKey string
}

// LocationName is the name the room knows the location by.
func (l Location) LocationName() string {
	switch l.Kind {
	case Milestone:
		if l.Metric == GuestCount {
			return fmt.Sprintf("Guests - %d", l.Threshold)
		}
		return fmt.Sprintf("Zoo Rating - %d", l.Threshold)
	case MechanicResearch:
		return l.Name
	case FirstAcquisition:
		return l.speciesLabel() + " - First Acquisition"
	case FirstBreeding:
		return l.speciesLabel() + " - First Breeding"
	case ConservationRelease:
		return l.speciesLabel() + " - Conservation Release"
	case WelfareResearch:
		return fmt.Sprintf("%s - Research Welfare %d", l.speciesLabel(), l.Level)
	}
	panic(fmt.Sprintf("location %d: unknown kind %d", l.ID, l.Kind))
}

// StringID is the apworld's own key for the location.
func (l Location) StringID() string {
	switch l.Kind {
	case Milestone:
		if l.Metric == GuestCount {
			return "guests_" + strconv.Itoa(int(l.Threshold))
		}
		return "zoo_rating" + strconv.Itoa(int(l.Threshold))
	case MechanicResearch:
		return l.Code
	case FirstAcquisition:
		return "fa_" + string(l.Species)
	case FirstBreeding:
		return "fb_" + string(l.Species)
	case ConservationRelease:
		return "cr_" + string(l.Species)
	case WelfareResearch:
		prefix := ""
		if s, _ := SpeciesByKey(l.Species); s.Interactivity == Exhibit {
			prefix = "e_"
		}
		return fmt.Sprintf("%swelfare%d_%s", prefix, l.Level, l.Species)
	}
	panic(fmt.Sprintf("location %d: unknown kind %d", l.ID, l.Kind))
}

func (l Location) speciesLabel() string {
	s, ok := SpeciesByKey(l.Species)
	if !ok {
		panic(fmt.Sprintf("location %d: unknown species %q", l.ID, l.Species))
	}
	return s.Label
}
