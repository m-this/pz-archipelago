package play

import (
	"encoding/json"
	"testing"

	"github.com/m-this/pz-archipelago/gamedata"
	"github.com/m-this/pz-archipelago/launcher/internal/game/pz"
)

func itemID(t *testing.T, name string) int64 {
	t.Helper()
	for _, it := range gamedata.Items {
		if it.Name == name {
			return int64(it.ID)
		}
	}
	t.Fatalf("no item %q", name)
	return 0
}

func TestSummarise(t *testing.T) {
	raw := json.RawMessage(`{"filler_amounts_cash": 1000, "goal": {"type": "breed", "args": {"required_breed": ["gpanda"]}}}`)
	slot, err := parseSlotData(raw)
	if err != nil {
		t.Fatal(err)
	}
	items := []int64{
		itemID(t, "Permit: Hippopotamus"), itemID(t, "Permit: Aardvark"),
		itemID(t, "Cash Inject Small"), itemID(t, "Cash Inject Large"), itemID(t, "Conservation Credits Medium"),
		itemID(t, "Progressive Barrier Level"), itemID(t, "Progressive Barrier Level"),
		itemID(t, "Progressive Breeding Level"), itemID(t, "Workshop"),
		itemID(t, "Food Shops Pizza Pen"), 99999999,
	}
	r := Summarise(items, slot)
	if r.Cash != 500+2000 || r.Credits != 200 {
		t.Errorf("money %v %v", r.Cash, r.Credits)
	}
	if r.BarrierLevels != 2 || r.Levels[pz.FamilyBreeding] != 1 || !r.Workshop || r.ResearchCentre {
		t.Errorf("summary %+v", r)
	}
	if len(r.MechanicRewards) != 1 || len(r.AnimalRewards) != 0 {
		t.Errorf("rewards %v %v", r.MechanicRewards, r.AnimalRewards)
	}
	blocked := r.Blocked()
	if blocked["aardvark"] || !blocked["hippo"] || !blocked["gpanda"] {
		t.Errorf("hippo needs water tools too; aardvark is bought; panda has no permit: %v", blocked)
	}
	r.WaterTools = true
	if r.Blocked()["hippo"] {
		t.Error("hippo with its permit and water tools")
	}
	goal := GoalLocations(slot)
	if len(goal) != 1 {
		t.Fatalf("goal %v", goal)
	}
}
