package shell

import "github.com/m-this/pz-archipelago/launcher/internal/ovl"

// The AP scenario's code is Scenario_22_Empty, which has no mechanic
// research config and so would start with all of it done. Renaming
// scenario_04's all-locked config onto it starts it at zero; the scenario_04
// campaign loses its config, which upstream accepted. Creating a new config
// crashed the game; a rename keeps the vanilla bytes.
const (
	mechanicConfigFrom = "scenario_04.scenariomechanicresearchsettings"
	mechanicConfigTo   = "scenario_22_empty.scenariomechanicresearchsettings"
	// Listing a gate in this topology is what interns its name, without
	// which the gate rows in c0research crash the game.
	noneResearchable = "default_off_noneresearchable.mechanicresearchsettings"
)

// PatchGameMain applies the shell to the player's own GameMain ovl.
func PatchGameMain(f *ovl.File) error {
	if err := f.Rename(mechanicConfigFrom, mechanicConfigTo); err != nil {
		return err
	}
	return f.AppendResearch(noneResearchable, GateNames())
}
