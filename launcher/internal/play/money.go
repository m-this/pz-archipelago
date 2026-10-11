package play

import (
	"math"

	"github.com/m-this/pz-archipelago/launcher/internal/game/pz"
)

// moneyIO is the part of the game the ledger touches.
type moneyIO interface {
	Read(m pz.Metric) (float64, bool)
	Write(m pz.Metric, v float64) bool
}

// settle adds to the game whatever the received money items total beyond
// what was granted: a reconnect adds nothing, a fresh save gets everything in
// one write, and what the player spent is never taken back. A ledger above
// the total (a reused seed name) is lowered to it without touching the game.
func settle(st slotState, r Received, game moneyIO) slotState {
	for _, k := range []struct {
		metric  pz.Metric
		target  float64
		granted *float64
	}{{pz.Cash, r.Cash, &st.Cash}, {pz.ConservationCredits, r.Credits, &st.Credits}} {
		delta := k.target - *k.granted
		if delta <= 0 {
			*k.granted = math.Min(*k.granted, k.target)
			continue
		}
		cur, ok := game.Read(k.metric)
		if !ok {
			continue
		}
		if game.Write(k.metric, cur+delta) {
			*k.granted = k.target
		}
	}
	return st
}

// Every new save of the AP scenario starts with this cash, baked into the
// scenario. A first-year park holding exactly it is one the client has not
// handled yet: handling it changes the cash.
const (
	bakedCash   = 150000.0
	cashEpsilon = 0.005
	freshYears  = 1
)

// fresh reports whether the park is a new save: in its first year, with the
// baked cash untouched.
func fresh(years int64, yearsKnown bool, game moneyIO) bool {
	if !yearsKnown || years >= freshYears {
		return false
	}
	cash, ok := game.Read(pz.Cash)
	return ok && math.Abs(cash-bakedCash) <= cashEpsilon
}
