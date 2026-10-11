// Package mod carries the AP scenario shell sources into the launcher. The
// README beside it says what each file does.
package mod

import "embed"

// Sources holds the content pack (pack/), its localisation (pack_loc/) and
// the module injected into Content0 (content0/).
//
//go:embed pack pack_loc content0
var Sources embed.FS
