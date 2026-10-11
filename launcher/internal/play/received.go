// Package play is the client loop: it applies the items the room sent,
// enforces them through the gates, and turns what the player does into
// checks. Upstream's pz_ap_client/client.py.
package play

import (
	"encoding/json"
	"math"
	"strings"

	"github.com/m-this/pz-archipelago/gamedata"
	"github.com/m-this/pz-archipelago/launcher/internal/game/pz"
)

// SlotData is what the apworld puts in the seed.
type SlotData struct {
	StartingMoney             *float64 `json:"starting_money"`
	FillerAmountsCash         *float64 `json:"filler_amounts_cash"`
	FillerAmountsConservation *float64 `json:"filler_amounts_conservation"`
	Goal                      struct {
		Type string `json:"type"`
		Args struct {
			RequiredBreed    []gamedata.SpeciesKey `json:"required_breed"`
			RequiredResearch []string              `json:"required_research"`
		} `json:"args"`
	} `json:"goal"`
}

func parseSlotData(raw json.RawMessage) (SlotData, error) {
	var s SlotData
	if len(raw) == 0 {
		return s, nil
	}
	return s, json.Unmarshal(raw, &s)
}

// Received is everything the received items unlock.
type Received struct {
	Permits          map[gamedata.SpeciesKey]bool
	WaterTools       bool
	ResearchCentre   bool
	Workshop         bool
	Conservation     bool
	BarrierLevels    int
	Levels           map[pz.Family]int
	AnimalRewards    []string
	MechanicRewards  []string
	Cash, Credits    float64
	ExhibitEnrichers int
}

var itemByID = func() map[int64]gamedata.Item {
	m := map[int64]gamedata.Item{}
	for _, it := range gamedata.Items {
		m[int64(it.ID)] = it
	}
	return m
}()

func lookupItem(id int64) (gamedata.Item, bool) {
	it, ok := itemByID[id]
	return it, ok
}

var familyOf = map[gamedata.Family]pz.Family{
	gamedata.Supplement:        pz.FamilySupplement,
	gamedata.Breeding:          pz.FamilyBreeding,
	gamedata.Education:         pz.FamilyEducation,
	gamedata.ExhibitEnrichment: pz.FamilyExhibitEnrichment,
}

// The default money amounts, for a room that does not carry the option:
// the apworld's option defaults, Medium.
const (
	defaultCash    = 500
	defaultCredits = 200
)

func sizeFactor(s gamedata.Size) float64 {
	switch s {
	case gamedata.Small:
		return 0.5
	case gamedata.Large:
		return 2
	case gamedata.Medium:
	}
	return 1
}

func amount(base *float64, fallback float64, s gamedata.Size) float64 {
	b := fallback
	if base != nil {
		b = *base
	}
	return math.Round(b*sizeFactor(s)*100) / 100
}

// isMechanic is whether a research reward is mechanic build content, gated
// by the installer, rather than an animal reward.
func isMechanic(content string) bool {
	n := pz.Normalise(content)
	for _, k := range []string{"foodshops", "drinkshops", "souvenirshops", "themesets", "habitats", "shelters", "stafffacilities", "transport", "power"} {
		if strings.Contains(n, k) {
			return true
		}
	}
	return false
}

// Summarise folds the received items.
func Summarise(items []int64, slot SlotData) Received {
	r := Received{Permits: map[gamedata.SpeciesKey]bool{}, Levels: map[pz.Family]int{}}
	for _, id := range items {
		it, ok := lookupItem(id)
		if !ok {
			continue
		}
		e := it.Effect
		switch e.Kind {
		case gamedata.Permit:
			r.Permits[e.Species] = true
		case gamedata.WaterTools:
			r.WaterTools = true
		case gamedata.FacilityUnlock:
			r.ResearchCentre = r.ResearchCentre || e.Facility == gamedata.ResearchCentre
			r.Workshop = r.Workshop || e.Facility == gamedata.Workshop
		case gamedata.ConservationProgram:
			r.Conservation = true
		case gamedata.Cash:
			r.Cash += amount(slot.FillerAmountsCash, defaultCash, e.Size)
		case gamedata.ConservationCredits:
			r.Credits += amount(slot.FillerAmountsConservation, defaultCredits, e.Size)
		case gamedata.Progressive:
			if e.Family == gamedata.Barrier {
				r.BarrierLevels++
			} else {
				r.Levels[familyOf[e.Family]]++
			}
		case gamedata.ResearchReward:
			if isMechanic(e.Content) {
				r.MechanicRewards = append(r.MechanicRewards, e.Content)
			} else {
				r.AnimalRewards = append(r.AnimalRewards, e.Content)
			}
		}
	}
	return r
}

// Blocked is every species that cannot be bought yet: its permit is missing,
// or it needs water and the Water Habitat Tools are missing.
func (r Received) Blocked() map[gamedata.SpeciesKey]bool {
	out := map[gamedata.SpeciesKey]bool{}
	for _, s := range gamedata.AllSpecies {
		if !r.Permits[s.Key] || (s.WaterNeeded && !r.WaterTools) {
			out[s.Key] = true
		}
	}
	return out
}

// Allowed is every species that can be bought.
func (r Received) Allowed() []gamedata.SpeciesKey {
	blocked := r.Blocked()
	var out []gamedata.SpeciesKey
	for _, s := range gamedata.AllSpecies {
		if !blocked[s.Key] {
			out = append(out, s.Key)
		}
	}
	return out
}

// AnimalRewardUniverse is every animal research reward the item pool holds:
// the ones the reward gate keeps locked until received.
func AnimalRewardUniverse() []string {
	var out []string
	for _, it := range gamedata.Items {
		if it.Effect.Kind == gamedata.ResearchReward && !isMechanic(it.Effect.Content) {
			out = append(out, it.Effect.Content)
		}
	}
	return out
}

// GoalLocations is the locations the goal needs: the first breeding of every
// required species and the research of every required key.
func GoalLocations(s SlotData) []gamedata.LocationID {
	breed := map[gamedata.SpeciesKey]bool{}
	for _, k := range s.Goal.Args.RequiredBreed {
		breed[k] = true
	}
	research := map[string]bool{}
	for _, k := range s.Goal.Args.RequiredResearch {
		research[k] = true
	}
	var out []gamedata.LocationID
	for _, l := range gamedata.Locations {
		switch l.Kind {
		case gamedata.FirstBreeding:
			if breed[l.Species] {
				out = append(out, l.ID)
			}
		case gamedata.WelfareResearch:
			if research["welfare_"+string(l.Species)] {
				out = append(out, l.ID)
			}
		case gamedata.MechanicResearch:
			if research[l.ResearchKey] {
				out = append(out, l.ID)
			}
		case gamedata.Milestone, gamedata.FirstAcquisition, gamedata.ConservationRelease:
		}
	}
	return out
}
