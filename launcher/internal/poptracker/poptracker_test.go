package poptracker

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestUnzipRefusesEscapingEntries(t *testing.T) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, _ := w.Create("../evil.txt")
	_, _ = f.Write([]byte("x"))
	_ = w.Close()
	if err := unzip(buf.Bytes(), filepath.Join(t.TempDir(), "out")); err == nil {
		t.Error("an entry outside the folder was extracted")
	}
}

func TestPackAndArgs(t *testing.T) {
	i := Install{Dir: t.TempDir()}
	if err := i.writePack(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(i.pack(), "manifest.json")); err != nil {
		t.Fatal(err)
	}
	args := i.Args(Room{Address: "archipelago.gg:38281", Slot: "Zoo"})
	want := []string{"--ap-host", "archipelago.gg:38281", "--ap-slot", "Zoo", "--pack-variant", "Map Tracker", i.pack()}
	if !slices.Equal(args, want) {
		t.Errorf("args %v", args)
	}
}
