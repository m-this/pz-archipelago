package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/m-this/pz-archipelago/launcher/internal/config"
	"github.com/m-this/pz-archipelago/launcher/internal/install"
	"github.com/m-this/pz-archipelago/launcher/internal/logbook"
	"github.com/m-this/pz-archipelago/launcher/internal/oodle"
	"github.com/m-this/pz-archipelago/launcher/internal/ovl"
	"github.com/m-this/pz-archipelago/launcher/internal/steam"
	"github.com/m-this/pz-archipelago/launcher/internal/web"
)

// runLauncher is what a double-click does: serve the page on 127.0.0.1 and
// open it in the browser.
func runLauncher(ctx context.Context, noBrowser bool) error {
	dir, err := config.Dir()
	if err != nil {
		return err
	}
	log, book, logFile, err := logbook.Open(filepath.Join(dir, "logs"))
	if err != nil {
		return err
	}
	defer func() { _ = logFile.Close() }()
	l, err := web.New(ctx, web.Deps{
		ConfigDir: dir, Version: version, Logger: log, Book: book,
		FindGame: func() (string, error) {
			root, err := steam.Root()
			if err != nil {
				return "", err
			}
			return steam.FindGame(root)
		},
		Codecs:  func(ctx context.Context, g install.Game) (ovl.Codecs, func()) { return gameCodecs(ctx, g, log) },
		OpenURL: openURL,
		HTTP:    &http.Client{Timeout: 10 * time.Minute},
	})
	if err != nil {
		return err
	}
	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: l.Handler(), ReadHeaderTimeout: 10 * time.Second}
	served := make(chan error, 1)
	go func() { served <- srv.Serve(listener) }()
	url := "http://" + listener.Addr().String() + "/"
	log.InfoContext(ctx, "the launcher is running; keep this window open while you play", "page", url)
	fmt.Println("Planet Zoo Archipelago is running at", url)
	if !noBrowser {
		if err := openURL(ctx, url); err != nil {
			log.WarnContext(ctx, "cannot open the browser; open the page yourself", "page", url, "error", err)
		}
	}
	select {
	case <-ctx.Done():
	case <-l.Quit():
	case err := <-served:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	log.InfoContext(ctx, "stopping: removing the client's patches from the game")
	l.Close()
	shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return srv.Shutdown(shutdown)
}

// gameCodecs loads the game's Oodle, which reading its files may need.
func gameCodecs(ctx context.Context, game install.Game, log *slog.Logger) (ovl.Codecs, func()) {
	codecs := ovl.Codecs{}
	if _, err := os.Stat(game.OodleDLL()); err != nil {
		return codecs, func() {}
	}
	codec, err := oodle.Load(game.OodleDLL())
	if err != nil {
		log.WarnContext(ctx, "oodle unavailable; an Oodle-compressed file will fail to read", "error", err)
		return codecs, func() {}
	}
	codecs[ovl.Oodle] = codec
	return codecs, func() { _ = codec.Close() }
}
