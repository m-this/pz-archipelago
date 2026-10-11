package play

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// slotState is what the client remembers of one slot of one seed: how many
// of the received items it applied, and how much money it granted. The
// money ledger is what makes a reconnect grant nothing twice and a fresh
// save grant everything once.
type slotState struct {
	Applied int     `json:"applied"`
	Cash    float64 `json:"cash"`
	Credits float64 `json:"credits"`
}

// store keeps one file per seed, rewritten whole on every change.
type store struct {
	path  string
	slots map[string]slotState
}

func safeName(seed string) string {
	name := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '_'
	}, seed)
	if name == "" {
		return "seed"
	}
	return name
}

func openStore(dir, seed string) (*store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &store{path: filepath.Join(dir, safeName(seed)+".json"), slots: map[string]slotState{}}
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &s.slots); err != nil {
		return nil, fmt.Errorf("%s: %w", s.path, err)
	}
	return s, nil
}

func (s *store) get(slot int) slotState { return s.slots[strconv.Itoa(slot)] }

func (s *store) set(slot int, st slotState) error {
	s.slots[strconv.Itoa(slot)] = st
	raw, err := json.MarshalIndent(s.slots, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
