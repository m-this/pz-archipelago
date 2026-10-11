// Command importapworld rewrites the generated gamedata tables from an
// upstream Planet Zoo apworld, the tinkieZo/ArchipelagoPZ worlds/planetzoo
// directory. It is how an upstream change reaches this repository: run it,
// read the diff, keep what is right. The tables, not the upstream files, are
// the source of truth afterwards.
//
// Usage: go run ./gamedata/cmd/importapworld <worlds/planetzoo> gamedata
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: importapworld <worlds/planetzoo> <gamedata dir>")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, "importapworld:", err)
		os.Exit(1)
	}
}
