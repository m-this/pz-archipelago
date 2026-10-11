package play

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/m-this/pz-archipelago/gamedata"
	"github.com/m-this/pz-archipelago/launcher/internal/apclient"
	"github.com/m-this/pz-archipelago/launcher/internal/game/fakepz"
	"github.com/m-this/pz-archipelago/launcher/internal/game/mem"
	"github.com/m-this/pz-archipelago/launcher/internal/game/pz"
)

// fakeRoom answers the handshake with a slot and items, and records checks.
type fakeRoom struct {
	mu     sync.Mutex
	checks map[int64]bool
	goal   bool
}

func (r *fakeRoom) serve(t *testing.T, items []int64, slotData string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		c, err := websocket.Accept(w, req, nil)
		if err != nil {
			return
		}
		ctx := req.Context()
		send := func(msgs ...map[string]any) {
			body, err := json.Marshal(msgs)
			if err != nil {
				t.Error(err)
				return
			}
			_ = c.Write(ctx, websocket.MessageText, body)
		}
		send(map[string]any{"cmd": "RoomInfo", "seed_name": "LOOP"})
		for {
			_, body, err := c.Read(ctx)
			if err != nil {
				return
			}
			var msgs []map[string]any
			_ = json.Unmarshal(body, &msgs)
			for _, m := range msgs {
				switch m["cmd"] {
				case "Connect":
					var list []map[string]any
					for _, id := range items {
						list = append(list, map[string]any{"item": id})
					}
					send(map[string]any{
						"cmd": "Connected", "slot": 1, "checked_locations": []int{},
						"slot_data": json.RawMessage(slotData), "players": []any{}, "slot_info": map[string]any{},
					},
						map[string]any{"cmd": "ReceivedItems", "index": 0, "items": list})
				case "LocationChecks":
					r.mu.Lock()
					locs, _ := m["locations"].([]any)
					for _, id := range locs {
						if f, ok := id.(float64); ok {
							r.checks[int64(f)] = true
						}
					}
					r.mu.Unlock()
				case "StatusUpdate":
					r.mu.Lock()
					r.goal = true
					r.mu.Unlock()
				}
			}
		}
	}))
	t.Cleanup(srv.Close)
	return "ws://" + strings.TrimPrefix(srv.URL, "http://")
}

func (r *fakeRoom) checked(id gamedata.LocationID) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.checks[int64(id)]
}

func locationNamed(t *testing.T, name string) gamedata.LocationID {
	t.Helper()
	for _, l := range gamedata.Locations {
		if l.LocationName() == name {
			return l.ID
		}
	}
	t.Fatalf("no location %q", name)
	return 0
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestLoopOnAFakeGame(t *testing.T) {
	game := fakepz.New()
	game.SetYears(0)
	game.SetGuests(1000)
	room := &fakeRoom{checks: map[int64]bool{}}
	url := room.serve(t, []int64{itemID(t, "Permit: Aardvark"), itemID(t, "Cash Inject Medium")},
		`{"starting_money": 75000, "filler_amounts_cash": 1000, "goal": {"type": "breed", "args": {"required_breed": ["gpanda"]}}}`)
	log := slog.New(slog.DiscardHandler)
	ap := apclient.New(apclient.Options{Address: url, Slot: "Zoo", Logger: log})
	loop := New(Options{
		Room: ap, StateDir: t.TempDir(), Logger: log, Tick: 20 * time.Millisecond,
		Open: func() (mem.Process, error) { return game, nil },
	})
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = ap.Run(ctx) }()
	done := make(chan struct{})
	go func() { _ = loop.Run(ctx); close(done) }()

	waitFor(t, "the loop to be ready", func() bool { return loop.Status().Ready })
	waitFor(t, "the guest milestones", func() bool { return room.checked(locationNamed(t, "Guests - 1000")) })
	if room.checked(locationNamed(t, "Guests - 1250")) {
		t.Error("a milestone above the guest count fired")
	}
	waitFor(t, "starting money plus the cash item", func() bool { return game.Cash() == 76000 })

	permitSite := uint64(fakepz.Base + 0xA0894E5)
	if head, _ := game.Read(permitSite, 1); head[0] != 0xE9 {
		t.Error("the permit gate is not installed")
	}
	status := loop.Status()
	if !status.InSession || status.Applied != 2 || len(status.Preflight) == 0 {
		t.Errorf("status %+v", status)
	}

	// The player spends; a later tick must not grant again.
	game.SetCash(10)
	time.Sleep(200 * time.Millisecond)
	if game.Cash() != 10 {
		t.Errorf("money granted twice: %v", game.Cash())
	}

	// Leaving the park makes the client idle.
	game.SetParkName("Some Other Zoo")
	waitFor(t, "the client to idle", func() bool { return !loop.Status().InSession })

	cancel()
	<-done
	for _, s := range pz.FakeLayout().Sites {
		if cur, _ := game.Read(fakepz.Base+s.RVA, len(s.Orig)); !bytes.Equal(cur, s.Orig) {
			t.Errorf("site %s not restored on shutdown: %x", s.Name, cur)
		}
	}
}
