// Command export writes the gamedata tables into the apworld's data directory.
// The output is committed; TestCommittedExportIsCurrent fails when it is stale.
//
// Usage: go run ./gamedata/cmd/export apworld/planetzoo/data
package main

import (
	"fmt"
	"os"

	"github.com/m-this/pz-archipelago/gamedata"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: export <dir>")
		os.Exit(2)
	}
	if err := gamedata.Export(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "export:", err)
		os.Exit(1)
	}
}
