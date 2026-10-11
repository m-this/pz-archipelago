// Package config holds the launcher's settings, saved in the user's config
// folder.
package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Settings is what the page asks for once and remembers.
type Settings struct {
	Room     string `json:"room"`
	Slot     string `json:"slot"`
	Password string `json:"password,omitempty"`
	// GameDir is the Planet Zoo folder, when Steam does not know it.
	GameDir string `json:"game_dir,omitempty"`
}

// Dir is the launcher's folder in the user's config folder.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "pzap"), nil
}

func path(dir string) string { return filepath.Join(dir, "config.json") }

// Load reads the settings; none saved yet is not an error.
func Load(dir string) (Settings, error) {
	var s Settings
	raw, err := os.ReadFile(path(dir))
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	return s, json.Unmarshal(raw, &s)
}

// Save writes the settings. The password is in there in clear, as in every
// Archipelago client; the file is only readable by its owner.
func Save(dir string, s Settings) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path(dir) + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path(dir))
}
