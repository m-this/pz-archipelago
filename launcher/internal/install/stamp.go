package install

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// stamp is the install's receipt: the hash of every file before and after,
// so the next run can tell its own install from vanilla and from a game
// update.
type stamp struct {
	Shell string               `json:"shell"`
	Pack  string               `json:"pack"`
	Files map[string]fileStamp `json:"files"`
}

// fileStamp holds sha256 hex digests. OVS fields are empty when the target
// has no .ovs or the install left it untouched.
type fileStamp struct {
	Vanilla    string `json:"vanilla"`
	Patched    string `json:"patched"`
	VanillaOVS string `json:"vanilla_ovs,omitempty"`
	PatchedOVS string `json:"patched_ovs,omitempty"`
}

// readStamp reports false when there is no stamp: no install was made.
func readStamp(path string) (stamp, bool, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return stamp{}, false, nil
	}
	if err != nil {
		return stamp{}, false, err
	}
	var s stamp
	if err := json.Unmarshal(raw, &s); err != nil {
		return stamp{}, false, fmt.Errorf("%s: %w", path, err)
	}
	return s, true, nil
}

func writeStamp(path string, s stamp) error {
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}
