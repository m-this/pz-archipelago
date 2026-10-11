// Package steam finds the Planet Zoo install.
package steam

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
)

// AppID is Planet Zoo's Steam app id.
const AppID = 703080

// ErrNotFound means no Steam library holds Planet Zoo.
var ErrNotFound = errors.New("planet zoo is not installed in any Steam library")

// Libraries lists the Steam library folders, the Steam folder itself first.
func Libraries(steamRoot string) ([]string, error) {
	raw, err := os.ReadFile(filepath.Join(steamRoot, "steamapps", "libraryfolders.vdf"))
	if errors.Is(err, fs.ErrNotExist) {
		return []string{steamRoot}, nil
	}
	if err != nil {
		return nil, err
	}
	root, err := ParseVDF(string(raw))
	if err != nil {
		return nil, fmt.Errorf("libraryfolders.vdf: %w", err)
	}
	libraries := []string{steamRoot}
	folders := root.Child("libraryfolders")
	if folders == nil {
		return libraries, nil
	}
	var numbers []int
	for key := range folders.Children {
		if n, err := strconv.Atoi(key); err == nil {
			numbers = append(numbers, n)
		}
	}
	slices.Sort(numbers)
	for _, n := range numbers {
		entry := folders.Child(strconv.Itoa(n))
		path := entry.Value
		if p := entry.Child("path"); p != nil {
			path = p.Value
		}
		if path != "" && !sameDir(path, steamRoot) {
			libraries = append(libraries, path)
		}
	}
	return libraries, nil
}

func sameDir(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}

// FindGame returns the Planet Zoo folder in the first library that has its
// manifest.
func FindGame(steamRoot string) (string, error) {
	libraries, err := Libraries(steamRoot)
	if err != nil {
		return "", err
	}
	manifest := fmt.Sprintf("appmanifest_%d.acf", AppID)
	for _, lib := range libraries {
		raw, err := os.ReadFile(filepath.Join(lib, "steamapps", manifest))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		acf, err := ParseVDF(string(raw))
		if err != nil {
			return "", fmt.Errorf("%s: %w", manifest, err)
		}
		dir := "Planet Zoo"
		if n := acf.Child("AppState", "installdir"); n != nil && n.Value != "" {
			dir = n.Value
		}
		return filepath.Join(lib, "steamapps", "common", dir), nil
	}
	return "", ErrNotFound
}
