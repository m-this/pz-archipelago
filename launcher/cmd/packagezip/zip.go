package main

import (
	"archive/zip"
	"compress/flate"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// archive is one ZIP being written, deflated as hard as the Python packager
// did, so the artifacts stay the size players are used to downloading.
type archive struct {
	file   *os.File
	writer *zip.Writer
}

func create(output string) (*archive, error) {
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return nil, err
	}
	file, err := os.Create(output)
	if err != nil {
		return nil, err
	}
	writer := zip.NewWriter(file)
	writer.RegisterCompressor(zip.Deflate, func(out io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(out, flate.BestCompression)
	})
	return &archive{file: file, writer: writer}, nil
}

func (a *archive) close() error {
	if err := a.writer.Close(); err != nil {
		_ = a.file.Close()
		return err
	}
	return a.file.Close()
}

// add copies one file in under name, keeping its modification time.
func (a *archive) add(source, name string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = name
	header.Method = zip.Deflate
	entry, err := a.writer.CreateHeader(header)
	if err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	_, err = io.Copy(entry, in)
	return err
}

func (a *archive) addBytes(name string, body []byte) error {
	entry, err := a.writer.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
	if err != nil {
		return err
	}
	_, err = entry.Write(body)
	return err
}

// filesUnder is every regular file below root, sorted, as (path, relative
// slash-separated name) pairs.
func filesUnder(root string) ([][2]string, error) {
	var files [][2]string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			files = append(files, [2]string{p, filepath.ToSlash(rel)})
		}
		return nil
	})
	slices.SortFunc(files, func(a, b [2]string) int { return strings.Compare(a[1], b[1]) })
	return files, err
}

// buildAPWorld packages a world the way Archipelago's own packager does: the
// module under its own name, the manifest stamped with the container version,
// and the world's .apignore plus the global Python build exclusions applied.
func buildAPWorld(source, output string, containerVersion int) error {
	manifest, err := readManifest(filepath.Join(source, "archipelago.json"))
	if err != nil {
		return err
	}
	manifest["compatible_version"] = containerVersion
	manifest["version"] = containerVersion
	stamped, err := json.Marshal(manifest)
	if err != nil {
		return err
	}

	files, err := filesUnder(source)
	if err != nil {
		return err
	}
	module := filepath.Base(filepath.Clean(source))
	out, err := create(output)
	if err != nil {
		return err
	}
	for _, f := range files {
		if excludedFromAPWorld(f[1]) {
			continue
		}
		if err := out.add(f[0], path.Join(module, f[1])); err != nil {
			_ = out.close()
			return err
		}
	}
	if err := out.addBytes(path.Join(module, "archipelago.json"), stamped); err != nil {
		_ = out.close()
		return err
	}
	if err := out.close(); err != nil {
		return err
	}
	return validateAPWorld(output, module, containerVersion)
}

func excludedFromAPWorld(rel string) bool {
	if rel == "archipelago.json" || rel == ".apignore" {
		return true
	}
	parts := strings.Split(rel, "/")
	if slices.Contains(parts, "test") || slices.Contains(parts, "__pycache__") {
		return true
	}
	ext := path.Ext(rel)
	return ext == ".pyc" || ext == ".pyo"
}

func readManifest(p string) (map[string]any, error) {
	body, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var manifest map[string]any
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}
	return manifest, nil
}

func validateAPWorld(output, module string, containerVersion int) error {
	reader, err := zip.OpenReader(output)
	if err != nil {
		return err
	}
	defer func() { _ = reader.Close() }()
	names := make(map[string]*zip.File, len(reader.File))
	for _, f := range reader.File {
		names[f.Name] = f
	}
	for _, required := range []string{module + "/__init__.py", module + "/archipelago.json"} {
		if _, ok := names[required]; !ok {
			return fmt.Errorf("invalid apworld, missing: %s", required)
		}
	}
	entry, err := names[module+"/archipelago.json"].Open()
	if err != nil {
		return err
	}
	defer func() { _ = entry.Close() }()
	var manifest struct {
		Version int `json:"version"`
	}
	if err := json.NewDecoder(entry).Decode(&manifest); err != nil {
		return err
	}
	if manifest.Version != containerVersion {
		return fmt.Errorf("invalid apworld container version")
	}
	return nil
}

// buildTree zips a directory as it stands.
func buildTree(source, output string) error {
	files, err := filesUnder(source)
	if err != nil {
		return err
	}
	out, err := create(output)
	if err != nil {
		return err
	}
	for _, f := range files {
		if err := out.add(f[0], f[1]); err != nil {
			_ = out.close()
			return err
		}
	}
	return out.close()
}
