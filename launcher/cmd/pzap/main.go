// Command pzap is the one exe a Planet Zoo Archipelago player runs. For now
// it installs and removes the AP scenario shell from the command line; the
// client and the page arrive in the milestones DRAFT.md lists.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/m-this/pz-archipelago/gamedata"
	"github.com/m-this/pz-archipelago/launcher/internal/install"
	"github.com/m-this/pz-archipelago/launcher/internal/oodle"
	"github.com/m-this/pz-archipelago/launcher/internal/ovl"
	"github.com/m-this/pz-archipelago/launcher/internal/steam"
)

// version is set at link time from apworld/planetzoo/archipelago.json.
var version = "dev"

const usage = `usage: pzap [-game-dir DIR] <command>

commands:
  status    say whether the AP scenario shell is installed
  install   install it, or bring it up to date (close the game first)
  restore   put the vanilla game files back
  version   print the version
`

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "pzap:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("pzap", flag.ContinueOnError)
	flags.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	gameDir := flags.String("game-dir", "", "the Planet Zoo folder, when Steam does not know it")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return errors.New("one command expected")
	}
	command := flags.Arg(0)
	switch command {
	case "version":
		fmt.Println(version)
		return nil
	case "status", "install", "restore":
	default:
		flags.Usage()
		return fmt.Errorf("unknown command %q", command)
	}
	if err := gamedata.Validate(); err != nil {
		return fmt.Errorf("the game data this build carries is broken: %w", err)
	}
	game, err := findGame(*gameDir)
	if err != nil {
		return err
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	var st install.Status
	switch command {
	case "status":
		st, err = game.Status()
	case "install":
		st, err = installShell(ctx, game, log)
	case "restore":
		st, err = game.Restore()
	}
	if err != nil {
		return err
	}
	fmt.Printf("%s: %s (%s)\n", game.Dir, st.State, st.Detail)
	return nil
}

func findGame(dir string) (install.Game, error) {
	if dir != "" {
		return install.Game{Dir: dir}, nil
	}
	root, err := steam.Root()
	if err != nil {
		return install.Game{}, err
	}
	dir, err = steam.FindGame(root)
	if err != nil {
		return install.Game{}, fmt.Errorf("%w; pass -game-dir", err)
	}
	return install.Game{Dir: dir}, nil
}

// installShell loads the game's Oodle, which reading its files may need.
func installShell(ctx context.Context, game install.Game, log *slog.Logger) (install.Status, error) {
	codecs := ovl.Codecs{}
	if _, err := os.Stat(game.OodleDLL()); err == nil {
		codec, err := oodle.Load(game.OodleDLL())
		if err != nil {
			log.WarnContext(ctx, "oodle unavailable; an Oodle-compressed file will fail to read", slog.String("error", err.Error()))
		} else {
			defer func() { _ = codec.Close() }()
			codecs[ovl.Oodle] = codec
		}
	}
	return game.Install(ctx, codecs, log)
}
