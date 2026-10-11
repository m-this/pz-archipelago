package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/m-this/pz-archipelago/launcher/internal/ovl"
	"github.com/m-this/pz-archipelago/launcher/internal/shell"
)

// built is one target's patched copy, ready to deploy.
type built struct {
	target  target
	ovl     []byte
	ovs     []byte // nil when the .ovs stays as it is
	refresh bool   // the live file is the vanilla to back up
	stamp   fileStamp
}

// Install puts the shell in place. Nothing in the game folder changes until
// every file is built and has read back; a failure before that leaves the
// game as it was.
func (g Game) Install(ctx context.Context, codecs ovl.Codecs, log *slog.Logger) (Status, error) {
	if err := g.refuseWhileRunning(); err != nil {
		return Status{}, err
	}
	st, err := g.Status()
	if err != nil {
		return Status{}, err
	}
	if st.State == NoGame || st.State == Ambiguous {
		return st, fmt.Errorf("cannot install: %s", st.Detail)
	}
	old, _, err := readStamp(g.stamp())
	if err != nil {
		return st, err
	}
	scratch, err := os.MkdirTemp("", "pzap-")
	if err != nil {
		return st, err
	}
	defer func() { _ = os.RemoveAll(scratch) }()

	var builds []built
	for _, t := range targets {
		log.InfoContext(ctx, "building patched files", slog.String("target", t.name))
		b, err := g.build(ctx, t, old.Files[t.name], codecs, scratch)
		if err != nil {
			return st, fmt.Errorf("%s: %w (game files untouched)", t.name, err)
		}
		builds = append(builds, b)
	}
	pack, loc, err := buildPack()
	if err != nil {
		return st, fmt.Errorf("content pack: %w (game files untouched)", err)
	}
	log.InfoContext(ctx, "deploying")
	if err := g.deploy(builds, pack, loc); err != nil {
		return st, err
	}
	digest, err := shell.Digest()
	if err != nil {
		return st, err
	}
	receipt := stamp{Shell: digest, Pack: hashBytes(pack), Files: map[string]fileStamp{}}
	for _, b := range builds {
		receipt.Files[b.target.name] = b.stamp
	}
	if err := writeStamp(g.stamp(), receipt); err != nil {
		return st, err
	}
	return g.Status()
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func readOptional(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return b, err
}

// source is the vanilla bytes to patch: the backup when the live file is
// already patched, the live file otherwise.
func (g Game) source(t target, s fileStamp) (ovlBytes, ovsBytes []byte, refresh bool, err error) {
	state, err := g.classify(t, s)
	if err != nil {
		return nil, nil, false, err
	}
	ovlPath, ovsPath := g.ovl(t), g.ovs(t)
	if state == filePatched {
		ovlPath, ovsPath = ovlPath+backupSuffix, ovsPath+backupSuffix
	}
	if ovlBytes, err = os.ReadFile(ovlPath); err != nil {
		return nil, nil, false, err
	}
	if ovsBytes, err = readOptional(ovsPath); err != nil {
		return nil, nil, false, err
	}
	if state == filePatched && hashBytes(ovlBytes) != s.Vanilla {
		return nil, nil, false, fmt.Errorf("the backup %s no longer matches the install receipt; verify the game files in Steam, then install again", ovlPath)
	}
	return ovlBytes, ovsBytes, state != filePatched, nil
}

func (g Game) build(ctx context.Context, t target, s fileStamp, codecs ovl.Codecs, scratch string) (built, error) {
	vanilla, vanillaOVS, refresh, err := g.source(t, s)
	if err != nil {
		return built{}, err
	}
	f, err := ovl.Read(vanilla, vanillaOVS, codecs)
	if err != nil {
		return built{}, err
	}
	if err := t.patch(ctx, f, scratch); err != nil {
		return built{}, err
	}
	patched, patchedOVS, err := f.Write()
	if err != nil {
		return built{}, err
	}
	if err := checkReadsBack(patched, patchedOVS, vanillaOVS, codecs); err != nil {
		return built{}, fmt.Errorf("the patched file does not read back: %w", err)
	}
	b := built{
		target: t, ovl: patched, ovs: patchedOVS, refresh: refresh,
		stamp: fileStamp{Vanilla: hashBytes(vanilla), Patched: hashBytes(patched)},
	}
	if patchedOVS != nil {
		b.stamp.VanillaOVS, b.stamp.PatchedOVS = hashBytes(vanillaOVS), hashBytes(patchedOVS)
	}
	return b, nil
}

// checkReadsBack parses the written file and decodes every archive.
func checkReadsBack(patched, patchedOVS, vanillaOVS []byte, codecs ovl.Codecs) error {
	if patchedOVS == nil {
		patchedOVS = vanillaOVS
	}
	f, err := ovl.Read(patched, patchedOVS, codecs)
	if err != nil {
		return err
	}
	for _, a := range f.Archives {
		if _, err := f.Content(a); err != nil {
			return err
		}
	}
	return nil
}

func buildPack() (pack, loc []byte, err error) {
	if pack, err = shell.BuildPack(); err != nil {
		return nil, nil, err
	}
	loc, err = shell.BuildLoc()
	return pack, loc, err
}

// localisedLeaves lists Content0's language folders, the ones holding a
// Loc.ovl. The pack mirrors them so every language shows its menu entry.
func (g Game) localisedLeaves() ([]string, error) {
	root := g.ovldata("Content0", "Localised")
	var leaves []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && path == root {
				return fs.SkipAll
			}
			return err
		}
		if !d.IsDir() && d.Name() == "Loc.ovl" {
			rel, err := filepath.Rel(root, filepath.Dir(path))
			if err != nil {
				return err
			}
			leaves = append(leaves, rel)
		}
		return nil
	})
	return leaves, err
}
