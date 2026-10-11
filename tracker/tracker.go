// Package tracker carries the PopTracker pack into the launcher, which
// installs it beside PopTracker. The README beside it credits its authors.
package tracker

import "embed"

// Pack is the pack's file tree.
//
//go:embed manifest.json images items layouts locations maps scripts LICENSE README.md
var Pack embed.FS

// Variant is the pack variant the launcher opens.
const Variant = "Map Tracker"
