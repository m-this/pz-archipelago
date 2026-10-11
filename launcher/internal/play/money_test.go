package play

import (
	"testing"

	"github.com/m-this/pz-archipelago/launcher/internal/game/pz"
)

type fakeMoney map[string]float64

func (f fakeMoney) Read(m pz.Metric) (float64, bool)  { v, ok := f[m.Name]; return v, ok }
func (f fakeMoney) Write(m pz.Metric, v float64) bool { f[m.Name] = v; return true }

func TestSettleAddsOnlyTheMissingMoney(t *testing.T) {
	game := fakeMoney{"cash": 1000, "conservation_credits": 5}
	st := settle(slotState{}, Received{Cash: 500, Credits: 200}, game)
	if game["cash"] != 1500 || game["conservation_credits"] != 205 || st.Cash != 500 {
		t.Fatalf("first grant: %v %+v", game, st)
	}
	game["cash"] = 300 // the player spent
	st = settle(st, Received{Cash: 500, Credits: 200}, game)
	if game["cash"] != 300 {
		t.Errorf("a reconnect granted again: %v", game["cash"])
	}
	st = settle(st, Received{Cash: 750, Credits: 200}, game)
	if game["cash"] != 550 || st.Cash != 750 {
		t.Errorf("a new item: %v %+v", game["cash"], st)
	}
	st = settle(st, Received{Cash: 100}, game)
	if game["cash"] != 550 || st.Cash != 100 {
		t.Errorf("a smaller total took money back: %v %+v", game["cash"], st)
	}
}

func TestFresh(t *testing.T) {
	game := fakeMoney{"cash": bakedCash}
	if !fresh(0, true, game) || fresh(1, true, game) || fresh(0, false, game) {
		t.Error("fresh")
	}
	game["cash"] = bakedCash - 1
	if fresh(0, true, game) {
		t.Error("a played park read as fresh")
	}
}
