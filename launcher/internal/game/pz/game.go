// Package pz is what the client knows about Planet Zoo's memory: where its
// objects are, how to read them, and the gates that enforce the AP items.
// It is a port of tinkieZo's Planet-Zoo-AP pz_ap_client/memory; every
// address and offset comes from there, found by their reverse engineering of
// the current game build. A game patch can move them, which is why every
// hook site carries a signature to re-find it and every reader checks what
// it reads before trusting it.
package pz

import (
	"log/slog"

	"github.com/m-this/pz-archipelago/launcher/internal/game/hook"
	"github.com/m-this/pz-archipelago/launcher/internal/game/mem"
)

// ProcessName is the game's executable.
const ProcessName = "PlanetZoo.exe"

// Game is one attached game process and the readers and gates built on it.
type Game struct {
	proc  mem.Process
	hooks *hook.Manager
	log   *slog.Logger
	base  uint64
	roots map[uint64]uint64

	Registry *Registry
	Research *Research
	Animals  *Animals
}

func New(proc mem.Process, log *slog.Logger) *Game {
	base, _ := proc.Module()
	g := &Game{proc: proc, hooks: hook.NewManager(proc), log: log, base: base, roots: map[uint64]uint64{}}
	g.Registry = &Registry{g: g}
	g.Research = newResearch(g)
	g.Animals = &Animals{g: g}
	return g
}

// Hooks is the detour manager of this process.
func (g *Game) Hooks() *hook.Manager { return g.hooks }

func (g *Game) Proc() mem.Process { return g.proc }

// Close removes every detour this process placed.
func (g *Game) Close() error { return g.hooks.RestoreAll() }

// Heap pointers the game hands out sit in this range; anything else read
// where a pointer should be is garbage.
const (
	heapLo = 0x10000
	heapHi = 1 << 47
)

func isHeap(p uint64) bool { return p > heapLo && p < heapHi }

func isPow2(n uint64) bool { return n != 0 && n&(n-1) == 0 }

// bitmapBytes is the size of an open-addressing map's occupancy bitmap.
func bitmapBytes(slots uint64) uint64 { return ((slots >> 3) + 7) &^ 7 }

func occupied(bitmap []byte, i uint64) bool { return bitmap[i>>3]>>(i&7)&1 == 1 }
