//go:build !windows

package steam

import (
	"os"
	"path/filepath"
)

// Root is the Steam folder of a Linux Steam client, where Planet Zoo runs
// under Proton.
func Root() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "Steam"), nil
}
