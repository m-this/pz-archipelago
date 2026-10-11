package install

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/m-this/pz-archipelago/launcher/internal/ovl"
	"github.com/m-this/pz-archipelago/launcher/internal/shell"
)

// fakeGame lays out a Planet Zoo folder from the miniature ovls the other
// packages test against. The GameMain miniature has no
// .scenariomechanicresearchsettings, which cobra-tools cannot create, so its
// patch renames the stand-in the ovl tests use.
func fakeGame(t *testing.T) (Game, map[string][]byte) {
	t.Helper()
	g := Game{Dir: t.TempDir()}
	vanilla := map[string][]byte{
		"Content0": readFile(t, filepath.Join("..", "shell", "testdata", "content0.ovl")),
		"GameMain": readFile(t, filepath.Join("..", "ovl", "testdata", "cobra-gm-vanilla.ovl")),
	}
	for name, body := range vanilla {
		writeFile(t, g.ovldata(name, "Main.ovl"), body)
	}
	writeFile(t, g.ovldata("Content0", "Localised", "English", "UnitedKingdom", "Loc.ovl"), []byte("vanilla loc"))

	saved := targets
	t.Cleanup(func() { targets = saved })
	targets = []target{saved[0], {"GameMain", func(_ context.Context, f *ovl.File, _ string) error {
		if err := f.Rename("scenario_04.mechanicresearchsettings", "scenario_22_empty.mechanicresearchsettings"); err != nil {
			return err
		}
		return f.AppendResearch("default_off_noneresearchable.mechanicresearchsettings", shell.GateNames())
	}}}
	return g, vanilla
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func writeFile(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func wantState(t *testing.T, g Game, want State) {
	t.Helper()
	st, err := g.Status()
	if err != nil {
		t.Fatal(err)
	}
	if st.State != want {
		t.Fatalf("state %s (%s), want %s", st.State, st.Detail, want)
	}
}

func install(t *testing.T, g Game) {
	t.Helper()
	if _, err := g.Install(context.Background(), nil, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatal(err)
	}
}

func TestInstallAndRestore(t *testing.T) {
	g, vanilla := fakeGame(t)
	wantState(t, g, Vanilla)

	install(t, g)
	wantState(t, g, Installed)
	patched := map[string][]byte{}
	for name, body := range vanilla {
		live := readFile(t, g.ovldata(name, "Main.ovl"))
		if bytes.Equal(live, body) {
			t.Errorf("%s was not patched", name)
		}
		if !bytes.Equal(readFile(t, g.ovldata(name, "Main.ovl"+backupSuffix)), body) {
			t.Errorf("%s backup is not the vanilla file", name)
		}
		patched[name] = live
	}
	for _, path := range []string{"Manifest.xml", "Main.ovl", filepath.Join("Localised", "English", "UnitedKingdom", "Loc.ovl")} {
		if _, err := os.Stat(filepath.Join(g.pack(), path)); err != nil {
			t.Errorf("content pack: %v", err)
		}
	}

	// Installing over an install rebuilds from the backups, to the same bytes.
	install(t, g)
	wantState(t, g, Installed)
	for name, body := range patched {
		if !bytes.Equal(readFile(t, g.ovldata(name, "Main.ovl")), body) {
			t.Errorf("%s: a second install built different bytes", name)
		}
	}

	if _, err := g.Restore(); err != nil {
		t.Fatal(err)
	}
	wantState(t, g, Vanilla)
	for name, body := range vanilla {
		if !bytes.Equal(readFile(t, g.ovldata(name, "Main.ovl")), body) {
			t.Errorf("%s was not restored", name)
		}
	}
	if _, err := os.Stat(g.pack()); !os.IsNotExist(err) {
		t.Error("the content pack is still there")
	}
}

func TestStatusAfterSteamAndUpdates(t *testing.T) {
	g, vanilla := fakeGame(t)
	install(t, g)

	// A Steam file check puts vanilla back over one file.
	writeFile(t, g.ovldata("GameMain", "Main.ovl"), vanilla["GameMain"])
	wantState(t, g, Stale)
	install(t, g)
	wantState(t, g, Installed)

	// A game update replaces a file with one neither hash knows.
	writeFile(t, g.ovldata("Content0", "Main.ovl"), append(bytes.Clone(vanilla["Content0"]), 0))
	wantState(t, g, GameUpdated)

	// The pack is part of the install.
	writeFile(t, g.ovldata("Content0", "Main.ovl"), vanilla["Content0"])
	install(t, g)
	if err := os.Remove(g.packOVL()); err != nil {
		t.Fatal(err)
	}
	wantState(t, g, Stale)
}

func TestAmbiguousWithoutReceipt(t *testing.T) {
	g, _ := fakeGame(t)
	writeFile(t, g.ovldata("Content0", "Main.ovl"+backupSuffix), []byte("some other vanilla"))
	wantState(t, g, Ambiguous)
	if _, err := g.Install(context.Background(), nil, slog.New(slog.DiscardHandler)); err == nil {
		t.Error("installed over an ambiguous game")
	}
}

func TestNoGame(t *testing.T) {
	wantState(t, Game{Dir: t.TempDir()}, NoGame)
}
