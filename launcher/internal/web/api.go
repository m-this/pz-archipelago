package web

import (
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"html/template"
	"net/http"
	"strings"

	"github.com/m-this/pz-archipelago/gamedata"
	"github.com/m-this/pz-archipelago/launcher/internal/config"
	"github.com/m-this/pz-archipelago/launcher/internal/play"
	"github.com/m-this/pz-archipelago/launcher/internal/steam"
)

//go:embed static/index.html
var static embed.FS

var page = template.Must(template.ParseFS(static, "static/index.html"))

// State is what the page draws.
type State struct {
	Version  string `json:"version"`
	GameDir  string `json:"game_dir"`
	GameErr  string `json:"game_error,omitempty"`
	Mod      string `json:"mod"`
	ModNote  string `json:"mod_note"`
	Room     string `json:"room"`
	Slot     string `json:"slot"`
	Password bool   `json:"password"`
	Busy     string `json:"busy,omitempty"`
	Error    string `json:"error,omitempty"`

	Session   bool        `json:"session"`
	Connected bool        `json:"connected"`
	Seed      string      `json:"seed,omitempty"`
	Received  int         `json:"received"`
	Checked   int         `json:"checked"`
	Locations int         `json:"locations"`
	RoomError string      `json:"room_error,omitempty"`
	Client    play.Status `json:"client"`

	Tracker  bool     `json:"tracker"`
	Log      []string `json:"log"`
	Messages []string `json:"messages"`
}

func (l *Launcher) state() State {
	l.mu.Lock()
	s := State{
		Version: l.deps.Version, GameDir: l.gameDir, GameErr: l.gameErr,
		Mod: l.mod.State.String(), ModNote: l.mod.Detail,
		Room: l.settings.Room, Slot: l.settings.Slot, Password: l.settings.Password != "",
		Busy: l.busy, Error: l.lastErr, Locations: len(gamedata.Locations),
	}
	if l.mod.State == 0 {
		s.Mod = ""
	}
	sess := l.session
	l.mu.Unlock()
	if sess != nil {
		rs := sess.room.State()
		s.Session, s.Connected, s.Seed, s.RoomError = true, rs.Connected, rs.Seed, rs.Error
		s.Received, s.Checked = len(rs.Items), len(rs.Checked)
		s.Client = sess.loop.Status()
		s.Messages = sess.room.Messages()
	}
	s.Tracker = l.tracker().Installed()
	s.Log = l.deps.Book.Lines()
	return s
}

// Handler serves the page and the API. Every action needs the token the
// page was served with, so another site open in the browser cannot post to
// the launcher.
func (l *Launcher) Handler() http.Handler {
	tokenBytes := make([]byte, 16)
	_, _ = rand.Read(tokenBytes)
	token := hex.EncodeToString(tokenBytes)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_ = page.Execute(w, map[string]string{"Token": token, "Version": l.deps.Version})
	})
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(l.state())
	})
	actions := map[string]func(r *http.Request) error{
		"settings":   l.postSettings,
		"recheck":    func(*http.Request) error { return l.background("checking the mod", l.refreshMod) },
		"install":    func(*http.Request) error { return l.background("installing the mod", l.installMod) },
		"restore":    func(*http.Request) error { return l.background("restoring the game files", l.restoreMod) },
		"connect":    func(*http.Request) error { return l.connect(l.ctx) },
		"disconnect": func(*http.Request) error { l.disconnect(); return nil },
		"game":       func(r *http.Request) error { return l.deps.OpenURL(r.Context(), steam.LaunchURL) },
		"tracker":    func(*http.Request) error { return l.background("opening PopTracker", l.openTracker) },
		"quit": func(*http.Request) error {
			select {
			case <-l.quit:
			default:
				close(l.quit)
			}
			return nil
		},
	}
	for name, act := range actions {
		mux.HandleFunc("POST /api/"+name, func(w http.ResponseWriter, r *http.Request) {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-PZAP-Token")), []byte(token)) != 1 {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			if err := act(r); err != nil {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	}
	return mux
}

type settingsForm struct {
	Room     string  `json:"room"`
	Slot     string  `json:"slot"`
	Password *string `json:"password"`
	GameDir  string  `json:"game_dir"`
}

func (l *Launcher) postSettings(r *http.Request) error {
	var f settingsForm
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<16)).Decode(&f); err != nil {
		return err
	}
	l.mu.Lock()
	s := config.Settings{
		Room: strings.TrimSpace(f.Room), Slot: strings.TrimSpace(f.Slot),
		Password: l.settings.Password, GameDir: strings.TrimSpace(f.GameDir),
	}
	if f.Password != nil {
		s.Password = *f.Password
	}
	gameChanged := s.GameDir != l.settings.GameDir
	l.settings = s
	if gameChanged {
		l.findGame()
	}
	l.mu.Unlock()
	if err := config.Save(l.deps.ConfigDir, s); err != nil {
		return err
	}
	if gameChanged {
		return l.background("checking the mod", l.refreshMod)
	}
	return nil
}
