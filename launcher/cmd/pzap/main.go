// Command pzap is the one exe a Planet Zoo Archipelago player runs. With no
// command it serves the launcher page; the commands do the same steps from a
// terminal.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/m-this/pz-archipelago/gamedata"
	"github.com/m-this/pz-archipelago/launcher/internal/apclient"
	"github.com/m-this/pz-archipelago/launcher/internal/game/mem"
	"github.com/m-this/pz-archipelago/launcher/internal/game/pz"
	"github.com/m-this/pz-archipelago/launcher/internal/install"
	"github.com/m-this/pz-archipelago/launcher/internal/play"
	"github.com/m-this/pz-archipelago/launcher/internal/steam"
)

// version is set at link time from apworld/planetzoo/archipelago.json.
var version = "dev"

const usage = `usage: pzap [-game-dir DIR] [command]

With no command, pzap opens the launcher page in the browser.

commands:
  status    say whether the AP scenario shell is installed
  install   install it, or bring it up to date (close the game first)
  restore   put the vanilla game files back
  play      connect to a room and run the client: pzap play -room HOST:PORT -slot NAME
  version   print the version
`

func main() {
	if err := runSignalled(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "pzap:", err)
		os.Exit(1)
	}
}

// runSignalled stops on Ctrl-C, and on the console window closing, which
// Windows delivers as SIGTERM: either way the client removes its patches
// before it exits.
func runSignalled(args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, args)
}

func run(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("pzap", flag.ContinueOnError)
	flags.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	gameDir := flags.String("game-dir", "", "the Planet Zoo folder, when Steam does not know it")
	noBrowser := flags.Bool("no-browser", false, "print the launcher's address instead of opening the browser")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() == 0 {
		return runLauncher(ctx, *noBrowser)
	}
	if flags.NArg() > 1 && flags.Arg(0) != "play" {
		flags.Usage()
		return errors.New("one command expected")
	}
	if flags.NArg() >= 1 && flags.Arg(0) == "play" {
		return runPlay(ctx, flags.Args()[1:])
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

func installShell(ctx context.Context, game install.Game, log *slog.Logger) (install.Status, error) {
	codecs, release := gameCodecs(ctx, game, log)
	defer release()
	return game.Install(ctx, codecs, log)
}

// runPlay runs the Archipelago client against the running game until stopped.
func runPlay(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("pzap play", flag.ContinueOnError)
	room := flags.String("room", "", "the room address, host:port")
	slot := flags.String("slot", "", "the slot name")
	password := flags.String("password", "", "the room password, if it has one")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *room == "" || *slot == "" {
		return errors.New("play needs -room and -slot")
	}
	config, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ap := apclient.New(apclient.Options{Address: *room, Slot: *slot, Password: *password, Logger: log})
	loop := play.New(play.Options{
		Room: ap, StateDir: filepath.Join(config, "pzap", "state"), Logger: log,
		Open: func() (mem.Process, error) { return mem.Open(pz.ProcessName) },
	})
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- loop.Run(ctx) }()
	err = ap.Run(ctx)
	cancel()
	if lerr := <-done; err == nil {
		err = lerr
	}
	return err
}
