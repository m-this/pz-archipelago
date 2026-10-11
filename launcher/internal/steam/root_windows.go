//go:build windows

package steam

import (
	"errors"

	"golang.org/x/sys/windows/registry"
)

// Root is the Steam folder, from the registry key the Steam client writes.
func Root() (string, error) {
	for _, k := range []struct {
		root  registry.Key
		path  string
		value string
	}{
		{registry.CURRENT_USER, `Software\Valve\Steam`, "SteamPath"},
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Valve\Steam`, "InstallPath"},
	} {
		key, err := registry.OpenKey(k.root, k.path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		value, _, err := key.GetStringValue(k.value)
		_ = key.Close()
		if err == nil && value != "" {
			return value, nil
		}
	}
	return "", errors.New("steam is not installed: no Steam folder in the registry")
}
