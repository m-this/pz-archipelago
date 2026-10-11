package shell

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"strings"

	"github.com/m-this/pz-archipelago/mod"
)

// version names the edits this package makes beyond the mod sources. Bump it
// when an edit changes, so existing installs read as stale and reinstall.
const version = "pzap-1"

// Digest identifies the shell this build installs: its edits, its gates and
// every mod source. An install whose stamp holds another digest is stale.
func Digest() (string, error) {
	h := sha256.New()
	h.Write([]byte(version + "\x00" + strings.Join(GateNames(), ",") + "\x00"))
	err := fs.WalkDir(mod.Sources, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		body, err := fs.ReadFile(mod.Sources, path)
		if err != nil {
			return err
		}
		h.Write([]byte(path + "\x00"))
		h.Write(body)
		return nil
	})
	return hex.EncodeToString(h.Sum(nil)), err
}
