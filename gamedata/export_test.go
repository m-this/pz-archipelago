package gamedata

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// The apworld reads the committed copy, so a table change that skipped
// `make export` must fail here.
func TestCommittedExportIsCurrent(t *testing.T) {
	dir := t.TempDir()
	if err := Export(dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{FileItems, FileLocations, FileSpecies} {
		want, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join("..", "apworld", "planetzoo", "data", name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("apworld/planetzoo/data/%s is stale: run make export", name)
		}
	}
}
