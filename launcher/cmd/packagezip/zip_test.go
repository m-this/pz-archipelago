package main

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func write(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func names(t *testing.T, p string) []string {
	t.Helper()
	r, err := zip.OpenReader(p)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	var out []string
	for _, f := range r.File {
		out = append(out, f.Name)
	}
	return out
}

func TestAPWorldIsTheModuleWithItsManifestStamped(t *testing.T) {
	dir := t.TempDir()
	world := filepath.Join(dir, "planetzoo")
	write(t, filepath.Join(world, "__init__.py"), "")
	write(t, filepath.Join(world, "data", "meta.json"), "{}")
	write(t, filepath.Join(world, "test", "test_runs.py"), "")
	write(t, filepath.Join(world, "__pycache__", "x.pyc"), "")
	write(t, filepath.Join(world, ".apignore"), "test/")
	write(t, filepath.Join(world, "archipelago.json"), `{"game": "Planet Zoo", "version": 1}`)
	out := filepath.Join(dir, "out", "planetzoo.apworld")

	if err := run([]string{"apworld", world, out, "-container-version", "7"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"planetzoo/__init__.py", "planetzoo/data/meta.json", "planetzoo/archipelago.json"}
	if got := names(t, out); !slices.Equal(got, want) {
		t.Errorf("entries %v, want %v", got, want)
	}

	r, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	entry, err := r.File[2].Open()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = entry.Close() }()
	var manifest map[string]any
	if err := json.NewDecoder(entry).Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	if manifest["version"] != 7.0 || manifest["compatible_version"] != 7.0 || manifest["game"] != "Planet Zoo" {
		t.Errorf("manifest %v", manifest)
	}
}

func TestTreeIsTheDirectoryAsItStands(t *testing.T) {
	dir := t.TempDir()
	tree := filepath.Join(dir, "pack")
	write(t, filepath.Join(tree, "manifest.json"), "{}")
	write(t, filepath.Join(tree, "scripts", "init.lua"), "")
	out := filepath.Join(dir, "pack.zip")

	if err := run([]string{"tree", tree, out}); err != nil {
		t.Fatal(err)
	}
	want := []string{"manifest.json", "scripts/init.lua"}
	if got := names(t, out); !slices.Equal(got, want) {
		t.Errorf("entries %v, want %v", got, want)
	}
}
