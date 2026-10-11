// Package install puts the AP scenario shell into a Planet Zoo install and
// takes it out again. It works on the player's own files: it backs up each
// one it changes, builds the changed copy in memory, checks it reads back,
// and only then replaces the original.
package install

import (
	"context"
	"path/filepath"

	"github.com/m-this/pz-archipelago/launcher/internal/ovl"
	"github.com/m-this/pz-archipelago/launcher/internal/shell"
)

// Game is a Planet Zoo folder, the one holding PlanetZoo.exe.
type Game struct {
	Dir string
}

const (
	backupSuffix = ".apbak"
	newSuffix    = ".pzapnew"
	stampName    = "pzap-install.json"
)

// target is one vanilla ovl the shell patches.
type target struct {
	name  string
	patch func(ctx context.Context, f *ovl.File, scratch string) error
}

var targets = []target{
	{"Content0", shell.PatchContent0},
	{"GameMain", func(_ context.Context, f *ovl.File, _ string) error { return shell.PatchGameMain(f) }},
}

func (g Game) ovldata(parts ...string) string {
	return filepath.Join(append([]string{g.Dir, "win64", "ovldata"}, parts...)...)
}

func (g Game) ovl(t target) string { return g.ovldata(t.name, "Main.ovl") }
func (g Game) ovs(t target) string { return g.ovldata(t.name, "Main.ovs") }
func (g Game) pack() string        { return g.ovldata(shell.PackName) }
func (g Game) packOVL() string     { return g.ovldata(shell.PackName, "Main.ovl") }
func (g Game) stamp() string       { return g.ovldata(stampName) }

// Exe is the game's executable, which the folder must have.
func (g Game) Exe() string { return filepath.Join(g.Dir, "PlanetZoo.exe") }

// OodleDLL is the game's copy of Oodle.
func (g Game) OodleDLL() string { return filepath.Join(g.Dir, "oo2core_8_win64.dll") }
