package shell

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/m-this/pz-archipelago/launcher/internal/ovl"
	"github.com/m-this/pz-archipelago/mod"
)

// PackName is the content pack's folder under win64\ovldata. The engine
// finds packs by scanning that folder, as it finds the DLCs.
const PackName = "PZArchipelago"

// PackManifest is the pack's Manifest.xml.
const PackManifest = `<ContentPack version="1">
  <Name>PZArchipelago</Name>
  <ID>e58f7b9c-1d2a-4a77-8c3e-aa90b4f1c2d7</ID>
  <Version>1</Version>
  <Type>Game</Type>
</ContentPack>
`

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// BuildPack builds the content pack's Main.ovl: the career entry, the
// scenario script and its settings.
func BuildPack() ([]byte, error) {
	return buildDir("pack")
}

// BuildLoc builds the pack's Loc.ovl, the strings the career menu shows.
func BuildLoc() ([]byte, error) {
	return buildDir("pack_loc")
}

func buildDir(dir string) ([]byte, error) {
	entries, err := fs.ReadDir(mod.Sources, dir)
	if err != nil {
		return nil, err
	}
	var b ovl.Builder
	for _, e := range entries {
		raw, err := fs.ReadFile(mod.Sources, path.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		ext := path.Ext(e.Name())
		base := strings.TrimSuffix(e.Name(), ext)
		switch ext {
		case ".lua":
			b.AddLua(base, raw)
		case ".txt":
			// A byte order mark shows up in the menu as part of the string.
			if bytes.HasPrefix(raw, utf8BOM) {
				return nil, fmt.Errorf("mod/%s/%s starts with a byte order mark", dir, e.Name())
			}
			b.AddText(base, string(raw))
		case ".animalresearchstartunlockedsettings":
			if err := checkNoStates(raw); err != nil {
				return nil, fmt.Errorf("mod/%s/%s: %w", dir, e.Name(), err)
			}
			b.AddEmptyResearchStart(base)
		default:
			return nil, fmt.Errorf("mod/%s/%s: no builder for %s", dir, e.Name(), ext)
		}
	}
	f, err := b.Build()
	if err != nil {
		return nil, err
	}
	out, _, err := f.Write()
	return out, err
}

// checkNoStates refuses a research start config that unlocks anything: the
// builder writes the empty one only.
func checkNoStates(raw []byte) error {
	var root struct {
		States struct {
			Children []struct{ XMLName xml.Name } `xml:",any"`
		} `xml:"states"`
	}
	if err := xml.Unmarshal(raw, &root); err != nil {
		return err
	}
	if len(root.States.Children) > 0 {
		return fmt.Errorf("%d start states: only an empty config can be built", len(root.States.Children))
	}
	return nil
}
