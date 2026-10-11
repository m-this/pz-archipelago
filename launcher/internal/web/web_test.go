package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/m-this/pz-archipelago/launcher/internal/config"
	"github.com/m-this/pz-archipelago/launcher/internal/install"
	"github.com/m-this/pz-archipelago/launcher/internal/logbook"
	"github.com/m-this/pz-archipelago/launcher/internal/ovl"
)

func newServer(t *testing.T) (*httptest.Server, string, string, *[]string) {
	t.Helper()
	dir := t.TempDir()
	var opened []string
	l, err := New(t.Context(), Deps{
		ConfigDir: dir, Version: "test", Logger: slog.New(slog.DiscardHandler), Book: &logbook.Book{},
		FindGame: func() (string, error) { return "", errors.New("no steam here") },
		Codecs:   func(context.Context, install.Game) (ovl.Codecs, func()) { return nil, func() {} },
		OpenURL:  func(_ context.Context, u string) error { opened = append(opened, u); return nil },
		HTTP:     http.DefaultClient,
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(l.Handler())
	t.Cleanup(srv.Close)
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	token := regexp.MustCompile(`pzap-token" content="([0-9a-f]{32})"`).FindStringSubmatch(string(body))
	if token == nil {
		t.Fatal("the page carries no token")
	}
	return srv, token[1], dir, &opened
}

func post(t *testing.T, srv *httptest.Server, token, action, body string) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/"+action, strings.NewReader(body))
	if token != "" {
		req.Header.Set("X-PZAP-Token", token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

func TestActionsNeedTheToken(t *testing.T) {
	srv, token, _, opened := newServer(t)
	if code := post(t, srv, "", "game", ""); code != http.StatusForbidden {
		t.Errorf("no token: %d", code)
	}
	if code := post(t, srv, "wrong", "game", ""); code != http.StatusForbidden {
		t.Errorf("wrong token: %d", code)
	}
	if code := post(t, srv, token, "game", ""); code != http.StatusNoContent || len(*opened) != 1 || !strings.HasPrefix((*opened)[0], "steam://run/703080") {
		t.Errorf("launch: %d %v", code, *opened)
	}
}

func TestSettingsAreSavedAndThePasswordStaysOut(t *testing.T) {
	srv, token, dir, _ := newServer(t)
	if code := post(t, srv, token, "settings", `{"room":" archipelago.gg:1234 ","slot":"Zoo","password":"secret"}`); code != http.StatusNoContent {
		t.Fatalf("settings: %d", code)
	}
	saved, err := config.Load(dir)
	if err != nil || saved.Room != "archipelago.gg:1234" || saved.Password != "secret" {
		t.Fatalf("saved %+v %v", saved, err)
	}
	// A later save without a password keeps the one saved.
	post(t, srv, token, "settings", `{"room":"archipelago.gg:1234","slot":"Zoo2"}`)
	if saved, _ := config.Load(dir); saved.Password != "secret" || saved.Slot != "Zoo2" {
		t.Errorf("saved %+v", saved)
	}
	resp, err := http.Get(srv.URL + "/api/state")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(raw), "secret") {
		t.Error("the state shows the password")
	}
	var s State
	if err := json.Unmarshal(raw, &s); err != nil || !s.Password || s.Slot != "Zoo2" || s.GameErr == "" {
		t.Errorf("state %+v %v", s, err)
	}
	if code := post(t, srv, token, "install", ""); code != http.StatusNoContent {
		t.Errorf("install without a game should start and fail in the background: %d", code)
	}
}
