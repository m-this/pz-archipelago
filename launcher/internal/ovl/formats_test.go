package ovl

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func fixtureBytes(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// The oracle is cobra-tools: cobra-c0-injected.ovl is what its inject made
// of cobra-c0-vanilla.ovl with the two new files. Ours must agree on every
// root struct and every buffer of the replaced files.
func TestReplaceAgreesWithCobraInject(t *testing.T) {
	f, _ := readFixture(t, "cobra-c0-vanilla.ovl")
	want, _ := readFixture(t, "cobra-c0-injected.ovl")
	const fdb, lua = "c0research.fdb", "scenarioscripts.scenarioscriptutils.lua"
	if err := f.ReplaceFDB(fdb, fixtureBytes(t, "c0research-new.fdb")); err != nil {
		t.Fatal(err)
	}
	if err := f.ReplaceLua(lua, fixtureBytes(t, "scriptutils-new.lua")); err != nil {
		t.Fatal(err)
	}
	out, _, err := f.Write()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Read(out, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, rootSize := range map[string]int{fdb: fdbRootSize, lua: luaHeaderWords} {
		compareFile(t, got, want, name, rootSize)
	}
	compareFile(t, got, want, "frontendmenu_test.txt", 0)
}

func compareFile(t *testing.T, got, want *File, name string, rootSize int) {
	t.Helper()
	g, err := got.Locate(name)
	if err != nil {
		t.Fatal(err)
	}
	w, err := want.Locate(name)
	if err != nil {
		t.Fatal(err)
	}
	gr, _ := g.RootBytes()
	wr, _ := w.RootBytes()
	if !bytes.Equal(gr[:rootSize], wr[:rootSize]) {
		t.Errorf("%s: root %x, cobra %x", name, gr[:rootSize], wr[:rootSize])
	}
	if _, err := g.dataEntry(); err != nil {
		return
	}
	gb, err := g.Buffers()
	if err != nil {
		t.Fatal(err)
	}
	wb, err := w.Buffers()
	if err != nil {
		t.Fatal(err)
	}
	if len(gb) != len(wb) {
		t.Fatalf("%s: %d buffers, cobra %d", name, len(gb), len(wb))
	}
	for i := range gb {
		if !bytes.Equal(gb[i], wb[i]) {
			t.Errorf("%s: buffer %d differs", name, i)
		}
	}
	gd, _ := g.dataEntry()
	wd, _ := w.dataEntry()
	gs, ws := g.Content.DataEntries[gd], w.Content.DataEntries[wd]
	if gs.Size1 != ws.Size1 || gs.Size2 != ws.Size2 {
		t.Errorf("%s: data sizes %d+%d, cobra %d+%d", name, gs.Size1, gs.Size2, ws.Size1, ws.Size2)
	}
}
