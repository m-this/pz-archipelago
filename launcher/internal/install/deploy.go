package install

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/m-this/pz-archipelago/launcher/internal/shell"
)

// deploy backs up the vanilla files that need it, writes every new file
// beside its target, and then moves them into place. If a move fails, the
// targets already moved are put back from their backups.
func (g Game) deploy(builds []built, pack, loc []byte) error {
	for _, b := range builds {
		if !b.refresh {
			continue
		}
		if err := copyFile(g.ovl(b.target), g.ovl(b.target)+backupSuffix); err != nil {
			return fmt.Errorf("backing up %s: %w (game files untouched)", g.ovl(b.target), err)
		}
		if b.ovs != nil {
			if err := copyFile(g.ovs(b.target), g.ovs(b.target)+backupSuffix); err != nil {
				return fmt.Errorf("backing up %s: %w (game files untouched)", g.ovs(b.target), err)
			}
		}
	}
	var moves [][2]string
	for _, b := range builds {
		for _, f := range []struct {
			path string
			body []byte
		}{{g.ovl(b.target), b.ovl}, {g.ovs(b.target), b.ovs}} {
			if f.body == nil {
				continue
			}
			path := f.path
			if err := os.WriteFile(path+newSuffix, f.body, 0o644); err != nil {
				removeNew(moves)
				return fmt.Errorf("writing %s: %w (game files untouched)", path, err)
			}
			moves = append(moves, [2]string{path + newSuffix, path})
		}
	}
	if err := g.writePack(pack, loc); err != nil {
		removeNew(moves)
		return fmt.Errorf("writing the content pack: %w (game files untouched)", err)
	}
	for i, m := range moves {
		if err := os.Rename(m[0], m[1]); err != nil {
			rollback(moves[:i])
			removeNew(moves[i:])
			return fmt.Errorf("replacing %s: %w; the files already replaced were restored from their backups", m[1], err)
		}
	}
	return nil
}

func removeNew(moves [][2]string) {
	for _, m := range moves {
		_ = os.Remove(m[0])
	}
}

func rollback(done [][2]string) {
	for _, m := range done {
		_ = copyFile(m[1]+backupSuffix, m[1])
	}
}

func (g Game) writePack(pack, loc []byte) error {
	dir := g.pack()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "Manifest.xml"), []byte(shell.PackManifest), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "Main.ovl"), pack, 0o644); err != nil {
		return err
	}
	leaves, err := g.localisedLeaves()
	if err != nil {
		return err
	}
	for _, leaf := range leaves {
		target := filepath.Join(dir, "Localised", leaf)
		if err := os.MkdirAll(target, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(target, "Loc.ovl"), loc, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(from, to string) error {
	body, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	if err := os.WriteFile(to+newSuffix, body, 0o644); err != nil {
		return err
	}
	return os.Rename(to+newSuffix, to)
}

// Restore puts every backup back and removes the content pack and the
// receipt. The backups stay, so the shell can be installed again.
func (g Game) Restore() (Status, error) {
	if err := g.refuseWhileRunning(); err != nil {
		return Status{}, err
	}
	restored := 0
	for _, t := range targets {
		for _, path := range []string{g.ovl(t), g.ovs(t)} {
			_, err := os.Stat(path + backupSuffix)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return Status{}, err
			}
			if err := copyFile(path+backupSuffix, path); err != nil {
				return Status{}, fmt.Errorf("restoring %s: %w", path, err)
			}
			restored++
		}
	}
	if restored == 0 {
		return Status{}, errors.New("no backup to restore; verify the game files in Steam to get vanilla back")
	}
	if err := os.RemoveAll(g.pack()); err != nil {
		return Status{}, err
	}
	if err := os.Remove(g.stamp()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Status{}, err
	}
	return g.Status()
}
