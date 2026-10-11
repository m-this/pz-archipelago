// Package shell is the AP scenario shell: what the installer builds from the
// mod sources and changes in the player's own game files. The recipe is
// tinkieZo's Planet-Zoo-AP pz_ap_client/ovl.py, live-proven in their client.
//
// Every build content that mechanic research unlocks (barriers, shops,
// themes, blueprints, transport, staff facilities, power) is re-pointed at a
// "gate": a research item nobody can research. The client completes a gate
// when the AP item for it arrives. The content's own research item is left
// alone and stays the AP location the player researches.
package shell

import (
	"slices"
	"strings"

	"github.com/m-this/pz-archipelago/gamedata"
)

// Research item ids 50000 and 50001 gate the basic research centre and
// workshop; 50002 and 50003 are vanilla blueprint items reused for barrier
// grades 1 and 3; 50004 to 50007 are minted for grades 2, 4, 5 and 6; one
// more is minted per mechanic content from 50008 on.
const (
	gateResearchCentre = 50000
	gateWorkshop       = 50001
	contentGateFirst   = 50008
)

type gate struct {
	name string
	id   int64
}

var barrierGates = []gate{
	{"ApBarrierGate2", 50004},
	{"ApBarrierGate4", 50005},
	{"ApBarrierGate5", 50006},
	{"ApBarrierGate6", 50007},
}

// barrierGrade maps each habitat boundary type to the gate of its grade.
var barrierGrade = map[string]int64{
	"Hedge": 50002, "Glass": 50003, "Wood_Logs": 50003,
	"Glass_One_Way": 50004, "ChainLink": 50004, "Corrugated": 50004,
	"Steel_Mesh": 50005, "Thick_Glass": 50005,
	"Gabion": 50006, "Brick_Red": 50006,
	"Concrete": 50007, "Electric": 50007,
}

var facilityRooms = []struct {
	gate int64
	part string
}{
	{gateResearchCentre, "RS_Room_4x4"},
	{gateWorkshop, "WS_Room_4x4"},
}

var facilityBlueprints = []struct {
	gate  int64
	where string
}{
	{gateResearchCentre, "lower(Title) like '%research_centre%' or lower(File) like '%research%centre%'"},
	{gateWorkshop, "lower(Title) like '%workshop%' or lower(File) like '%workshop%'"},
}

// The "you need a research centre / workshop" alerts would drop the player
// straight into placing one, around the gated build menu.
var placementAlerts = []string{"NoResearchCentre", "NoWorkshop"}

var mechanicKeywords = []string{
	"foodshops", "drinkshops", "souvenirshops", "themesets", "habitats",
	"shelters", "stafffacilities", "transport", "power",
}

func normalised(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		case r >= 'A' && r <= 'Z':
			return r - 'A' + 'a'
		}
		return -1
	}, s)
}

// mechanicContents is every research reward that mechanic research unlocks,
// sorted, so the gate ids come out the same on every install.
func mechanicContents() []string {
	var contents []string
	for _, it := range gamedata.Items {
		if it.Effect.Kind != gamedata.ResearchReward {
			continue
		}
		n := normalised(it.Effect.Content)
		if slices.ContainsFunc(mechanicKeywords, func(k string) bool { return strings.Contains(n, k) }) {
			contents = append(contents, it.Effect.Content)
		}
	}
	slices.Sort(contents)
	return slices.Compact(contents)
}

// contentGates mints one gate per mechanic content.
func contentGates() []gate {
	contents := mechanicContents()
	gates := make([]gate, len(contents))
	for i, c := range contents {
		gates[i] = gate{name: "ApGate" + c, id: contentGateFirst + int64(i)}
	}
	return gates
}

// GateNames is every minted gate, as the GameMain topology must intern them.
func GateNames() []string {
	var names []string
	for _, g := range barrierGates {
		names = append(names, g.name)
	}
	for _, g := range contentGates() {
		names = append(names, g.name)
	}
	return names
}
