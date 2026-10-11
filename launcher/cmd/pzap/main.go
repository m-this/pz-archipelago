// Command pzap is the one exe a Planet Zoo Archipelago player runs. For now
// it checks the game data it carries and reports its version; the installer,
// the client and the page arrive in the milestones DRAFT.md lists.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/m-this/pz-archipelago/gamedata"
)

// version is set at link time from apworld/planetzoo/archipelago.json.
var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	if err := gamedata.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, "pzap: the game data this build carries is broken:", err)
		os.Exit(1)
	}
	fmt.Printf("pzap %s: %d species, %d items, %d locations\n",
		version, len(gamedata.AllSpecies), len(gamedata.Items), len(gamedata.Locations))
}
