package hook

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// python-golden.json is what the Python client's builders return for the
// same arguments (see ../testdata/README.md).
const (
	region = 0x150010000
	resume = 0x140C8216F
	fail   = 0x140C82100
)

func golden(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "testdata", "python-golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var g map[string]json.RawMessage
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatal(err)
	}
	return g
}

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestTrampolinesMatchPython(t *testing.T) {
	g := golden(t)
	cases := map[string][]byte{
		"permit":               Permit(region, region, resume, fail, unhex(t, "0fb68310020000")),
		"release_gate":         ReleaseGate(region, region, resume, unhex(t, "48895c2410")),
		"release_gate_exhibit": ReleaseGate(region, region, resume, unhex(t, "4055535657")),
		"release_species":      ReleaseSpeciesCapture(region, region, resume, unhex(t, "488b4d484889f2")),
		"research_gate":        ResearchGate(region, region, resume, unhex(t, "41c6474902"), 0x4F),
		"insert":               InsertInstrument(region, region, resume, unhex(t, "488bbdd8000000")),
		"exhibit":              ExhibitInstrument(region, region, resume, unhex(t, "488bc44c894820")),
		"exhibit_release":      ExhibitReleaseCapture(region, region, resume, unhex(t, "4d8bbef8000000")),
	}
	for name, got := range cases {
		var want string
		if err := json.Unmarshal(g[name], &want); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if hex.EncodeToString(got) != want {
			t.Errorf("%s:\n got %x\nwant %s", name, got, want)
		}
	}
}
