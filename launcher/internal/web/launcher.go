// Package web is the launcher the player sees: a page served on 127.0.0.1
// that installs the mod, connects to the room, starts the game and opens
// PopTracker. The page reads the state once a second and posts actions.
package web

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/m-this/pz-archipelago/launcher/internal/apclient"
	"github.com/m-this/pz-archipelago/launcher/internal/config"
	"github.com/m-this/pz-archipelago/launcher/internal/game/mem"
	"github.com/m-this/pz-archipelago/launcher/internal/game/pz"
	"github.com/m-this/pz-archipelago/launcher/internal/install"
	"github.com/m-this/pz-archipelago/launcher/internal/logbook"
	"github.com/m-this/pz-archipelago/launcher/internal/ovl"
	"github.com/m-this/pz-archipelago/launcher/internal/play"
	"github.com/m-this/pz-archipelago/launcher/internal/poptracker"
)

// Deps is what the launcher is built from; the command wires the real ones.
type Deps struct {
	ConfigDir string
	Version   string
	Logger    *slog.Logger
	Book      *logbook.Book
	// FindGame locates Planet Zoo through Steam.
	FindGame func() (string, error)
	// Codecs gives the codecs to read the game's files, Oodle included.
	Codecs func(ctx context.Context, game install.Game) (ovl.Codecs, func())
	// OpenURL opens a URL in the desktop's handler: the browser, Steam.
	OpenURL func(ctx context.Context, url string) error
	HTTP    *http.Client
}

// Launcher is the launcher's state.
type Launcher struct {
	// ctx lives as long as the launcher: background actions and the session
	// outlive the request that started them.
	ctx  context.Context //nolint:containedctx // the launcher's lifetime, not a request's
	deps Deps
	quit chan struct{}

	mu       sync.Mutex
	settings config.Settings
	gameDir  string
	gameErr  string
	mod      install.Status
	busy     string
	lastErr  string
	session  *session
}

type session struct {
	room   *apclient.Client
	loop   *play.Loop
	cancel context.CancelFunc
	done   chan struct{}
}

func New(ctx context.Context, deps Deps) (*Launcher, error) {
	settings, err := config.Load(deps.ConfigDir)
	if err != nil {
		return nil, err
	}
	l := &Launcher{ctx: ctx, deps: deps, quit: make(chan struct{}), settings: settings}
	l.findGame()
	if l.gameDir != "" {
		_ = l.background("checking the mod", l.refreshMod)
	}
	return l, nil
}

// Quit is closed when the player asks the launcher to stop.
func (l *Launcher) Quit() <-chan struct{} { return l.quit }

func (l *Launcher) findGame() {
	dir, err := l.settings.GameDir, error(nil)
	if dir == "" {
		dir, err = l.deps.FindGame()
	}
	l.gameDir, l.gameErr = dir, ""
	if err != nil {
		l.gameErr = err.Error()
	}
}

func (l *Launcher) game() (install.Game, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return install.Game{Dir: l.gameDir}, l.gameDir != ""
}

// background runs a long action, one at a time, and records its error.
func (l *Launcher) background(name string, work func(ctx context.Context) error) error {
	l.mu.Lock()
	if l.busy != "" {
		l.mu.Unlock()
		return errors.New("busy: " + l.busy)
	}
	l.busy, l.lastErr = name, ""
	l.mu.Unlock()
	go func() {
		err := work(l.ctx)
		l.mu.Lock()
		l.busy = ""
		if err != nil {
			l.lastErr = err.Error()
			l.deps.Logger.ErrorContext(l.ctx, "action failed", "action", name, "error", err)
		}
		l.mu.Unlock()
	}()
	return nil
}

func (l *Launcher) refreshMod(context.Context) error {
	game, ok := l.game()
	if !ok {
		return errors.New("planet zoo was not found; set its folder")
	}
	st, err := game.Status()
	if err != nil {
		return err
	}
	l.mu.Lock()
	l.mod = st
	l.mu.Unlock()
	return nil
}

func (l *Launcher) installMod(ctx context.Context) error {
	game, ok := l.game()
	if !ok {
		return errors.New("planet zoo was not found; set its folder")
	}
	codecs, release := l.deps.Codecs(ctx, game)
	defer release()
	st, err := game.Install(ctx, codecs, l.deps.Logger)
	l.mu.Lock()
	l.mod = st
	l.mu.Unlock()
	return err
}

func (l *Launcher) restoreMod(context.Context) error {
	game, ok := l.game()
	if !ok {
		return errors.New("planet zoo was not found; set its folder")
	}
	st, err := game.Restore()
	l.mu.Lock()
	l.mod = st
	l.mu.Unlock()
	return err
}

// connect starts the room session and the client loop with the saved
// settings, replacing any session already running.
func (l *Launcher) connect(ctx context.Context) error {
	l.disconnect()
	l.mu.Lock()
	s := l.settings
	l.mu.Unlock()
	if s.Room == "" || s.Slot == "" {
		return errors.New("enter the room address and your slot name first")
	}
	stateDir := filepath.Join(l.deps.ConfigDir, "state")
	ctx, cancel := context.WithCancel(ctx)
	room := apclient.New(apclient.Options{Address: s.Room, Slot: s.Slot, Password: s.Password, Logger: l.deps.Logger})
	loop := play.New(play.Options{
		Room: room, StateDir: stateDir, Logger: l.deps.Logger,
		Open: func() (mem.Process, error) { return mem.Open(pz.ProcessName) },
	})
	sess := &session{room: room, loop: loop, cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(sess.done)
		loopDone := make(chan struct{})
		go func() { _ = loop.Run(ctx); close(loopDone) }()
		if err := room.Run(ctx); err != nil {
			l.mu.Lock()
			l.lastErr = err.Error()
			l.mu.Unlock()
			l.deps.Logger.ErrorContext(ctx, "the room refused the connection", "error", err)
			cancel()
		}
		<-loopDone
	}()
	l.mu.Lock()
	l.session = sess
	l.mu.Unlock()
	return nil
}

// disconnect stops the session and waits for the loop to remove its
// patches from the game.
func (l *Launcher) disconnect() {
	l.mu.Lock()
	sess := l.session
	l.session = nil
	l.mu.Unlock()
	if sess == nil {
		return
	}
	sess.cancel()
	select {
	case <-sess.done:
	case <-time.After(10 * time.Second):
		l.deps.Logger.WarnContext(l.ctx, "the client took too long to stop")
	}
}

func (l *Launcher) tracker() poptracker.Install {
	return poptracker.Install{Dir: filepath.Join(l.deps.ConfigDir, "poptracker")}
}

func (l *Launcher) openTracker(ctx context.Context) error {
	t := l.tracker()
	if err := t.Ensure(ctx, l.deps.HTTP); err != nil {
		return err
	}
	l.mu.Lock()
	s := l.settings
	l.mu.Unlock()
	return t.Open(ctx, poptracker.Room{Address: s.Room, Slot: s.Slot, Password: s.Password})
}

// Close stops the session, leaving the game clean.
func (l *Launcher) Close() { l.disconnect() }
