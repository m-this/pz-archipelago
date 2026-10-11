// Package poptracker installs PopTracker and the Planet Zoo pack, and opens
// them connected to the room.
package poptracker

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/m-this/pz-archipelago/tracker"
)

// The PopTracker release the launcher installs, pinned by its digest.
const (
	Version      = "0.35.4"
	archiveURL   = "https://github.com/black-sliver/PopTracker/releases/download/v0.35.4/poptracker_0-35-4_win64.zip"
	archiveSHA   = "3c0f6220e83d4b79e044b5c8eb4602ed517c9734c7e9507c1eb7687021442d1c"
	archiveLimit = 64 << 20
	exeName      = "poptracker.exe"
	packDir      = "planetzoo"
)

// Install is a PopTracker installed under one directory.
type Install struct {
	Dir string
}

func (i Install) root() string { return filepath.Join(i.Dir, Version) }

func (i Install) exe() string { return filepath.Join(i.root(), "poptracker", exeName) }

func (i Install) pack() string { return filepath.Join(i.Dir, "packs", packDir) }

// Installed reports whether this version is in place.
func (i Install) Installed() bool {
	_, err := os.Stat(i.exe())
	return err == nil
}

// Ensure downloads and unpacks PopTracker when it is missing, then writes the
// pack, which a new launcher may have changed.
func (i Install) Ensure(ctx context.Context, client *http.Client) error {
	if !i.Installed() {
		archive, err := download(ctx, client)
		if err != nil {
			return err
		}
		if err := unzip(archive, i.root()); err != nil {
			return err
		}
	}
	return i.writePack()
}

func download(ctx context.Context, client *http.Client) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, archiveURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download PopTracker: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download PopTracker: %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, archiveLimit))
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(body)
	if hex.EncodeToString(sum[:]) != archiveSHA {
		return nil, errors.New("the PopTracker download does not match its pinned digest")
	}
	return body, nil
}

// unzip extracts into dir, refusing any entry that would land outside it.
func unzip(archive []byte, dir string) error {
	r, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return err
	}
	tmp := dir + ".partial"
	_ = os.RemoveAll(tmp)
	for _, f := range r.File {
		target := filepath.Join(tmp, filepath.FromSlash(f.Name))
		if !strings.HasPrefix(target, filepath.Clean(tmp)+string(os.PathSeparator)) {
			return fmt.Errorf("archive entry %q escapes the install folder", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := extract(f, target); err != nil {
			return err
		}
	}
	_ = os.RemoveAll(dir)
	return os.Rename(tmp, dir)
}

func extract(f *zip.File, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	in, err := f.Open()
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(target)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, io.LimitReader(in, archiveLimit)); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func (i Install) writePack() error {
	dest := i.pack()
	_ = os.RemoveAll(dest)
	return fs.WalkDir(tracker.Pack, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dest, filepath.FromSlash(path))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		body, err := fs.ReadFile(tracker.Pack, path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, body, 0o644)
	})
}

// Room is what PopTracker connects to.
type Room struct {
	Address, Slot, Password string
}

// Args is PopTracker's command line: the room, then the pack and its variant.
func (i Install) Args(room Room) []string {
	var args []string
	if room.Address != "" {
		args = append(args, "--ap-host", room.Address, "--ap-slot", room.Slot)
		if room.Password != "" {
			args = append(args, "--ap-password", room.Password)
		}
	}
	return append(args, "--pack-variant", tracker.Variant, i.pack())
}

// Open starts PopTracker; it keeps running after the launcher exits, so the
// context does not stop it.
func (i Install) Open(ctx context.Context, room Room) error {
	cmd := exec.CommandContext(context.WithoutCancel(ctx), i.exe(), i.Args(room)...)
	cmd.Dir = filepath.Dir(i.exe())
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open PopTracker: %w", err)
	}
	return cmd.Process.Release()
}
