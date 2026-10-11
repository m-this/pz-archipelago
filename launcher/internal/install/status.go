package install

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/m-this/pz-archipelago/launcher/internal/shell"
)

// State is where an install stands.
type State uint8

const (
	// NoGame: the folder holds no Planet Zoo.
	NoGame State = iota + 1
	// Vanilla: no shell in the files, whether never installed or removed by
	// a Steam file check.
	Vanilla
	// Installed: the shell this build carries is in place.
	Installed
	// Stale: a shell is in place, but not this build's, or part of it is
	// missing. Installing again fixes it.
	Stale
	// GameUpdated: a file matches neither its vanilla nor its patched hash;
	// a game patch replaced it. Installing again backs up the new vanilla.
	GameUpdated
	// Ambiguous: no install receipt, yet a file differs from its backup. It
	// may be modified; restore before installing.
	Ambiguous
)

func (s State) String() string {
	switch s {
	case NoGame:
		return "no game"
	case Vanilla:
		return "vanilla"
	case Installed:
		return "installed"
	case Stale:
		return "stale"
	case GameUpdated:
		return "game updated"
	case Ambiguous:
		return "ambiguous"
	}
	return fmt.Sprintf("state %d", s)
}

// Status is a state and why.
type Status struct {
	State  State
	Detail string
}

// fileState classifies one target against the receipt.
type fileState uint8

const (
	fileVanilla fileState = iota + 1
	filePatched
	fileUpdated
)

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (g Game) classify(t target, s fileStamp) (fileState, error) {
	live, err := hashFile(g.ovl(t))
	if err != nil {
		return 0, err
	}
	ovs, err := hashFile(g.ovs(t))
	if err != nil {
		return 0, err
	}
	switch {
	case live == s.Patched && (s.PatchedOVS == "" || ovs == s.PatchedOVS):
		return filePatched, nil
	case live == s.Vanilla && (s.VanillaOVS == "" || ovs == s.VanillaOVS):
		return fileVanilla, nil
	}
	return fileUpdated, nil
}

// Status reads where the install stands.
func (g Game) Status() (Status, error) {
	for _, t := range targets {
		_, err := os.Stat(g.ovl(t))
		if errors.Is(err, fs.ErrNotExist) {
			return Status{NoGame, fmt.Sprintf("no Planet Zoo in %s: %s is missing", g.Dir, g.ovl(t))}, nil
		}
		if err != nil {
			return Status{}, err
		}
	}
	s, ok, err := readStamp(g.stamp())
	if err != nil {
		return Status{}, err
	}
	if !ok {
		return g.statusWithoutStamp()
	}
	states := map[fileState]int{}
	for _, t := range targets {
		state, err := g.classify(t, s.Files[t.name])
		if err != nil {
			return Status{}, err
		}
		states[state]++
	}
	switch {
	case states[fileUpdated] > 0:
		return Status{GameUpdated, "a game update replaced the files the shell changes; installing again rebuilds it"}, nil
	case states[fileVanilla] == len(targets):
		return Status{Vanilla, "the game files are vanilla, the shell was removed"}, nil
	case states[filePatched] != len(targets):
		return Status{Stale, "part of the shell was undone; installing again restores it"}, nil
	}
	return g.statusInstalled(s)
}

func (g Game) statusWithoutStamp() (Status, error) {
	for _, t := range targets {
		backup, err := hashFile(g.ovl(t) + backupSuffix)
		if err != nil {
			return Status{}, err
		}
		live, err := hashFile(g.ovl(t))
		if err != nil {
			return Status{}, err
		}
		if backup != "" && backup != live {
			return Status{Ambiguous, fmt.Sprintf("%s differs from its backup and no install receipt explains it; restore first", g.ovl(t))}, nil
		}
	}
	return Status{Vanilla, "no shell installed"}, nil
}

func (g Game) statusInstalled(s stamp) (Status, error) {
	digest, err := shell.Digest()
	if err != nil {
		return Status{}, err
	}
	if s.Shell != digest {
		return Status{Stale, "an older shell is installed; installing again updates it"}, nil
	}
	pack, err := hashFile(g.packOVL())
	if err != nil {
		return Status{}, err
	}
	if pack == "" || pack != s.Pack {
		return Status{Stale, "the PZArchipelago content pack is missing or changed; installing again restores it"}, nil
	}
	return Status{Installed, "the shell is installed and current"}, nil
}
