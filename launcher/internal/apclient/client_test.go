package apclient

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// room is a fake Archipelago server: it answers the handshake and records
// what the client sends.
type room struct {
	t        *testing.T
	received chan map[string]any
	conn     chan *websocket.Conn
}

func newRoom(t *testing.T) (*room, string) {
	t.Helper()
	r := &room{t: t, received: make(chan map[string]any, 32), conn: make(chan *websocket.Conn, 1)}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		c, err := websocket.Accept(w, req, nil)
		if err != nil {
			return
		}
		r.conn <- c
		ctx := req.Context()
		r.send(ctx, c, map[string]any{"cmd": "RoomInfo", "seed_name": "S1"})
		for {
			_, body, err := c.Read(ctx)
			if err != nil {
				return
			}
			var msgs []map[string]any
			_ = json.Unmarshal(body, &msgs)
			for _, m := range msgs {
				if m["cmd"] == "Connect" {
					r.send(ctx, c, map[string]any{
						"cmd": "Connected", "slot": 3, "checked_locations": []int{20000},
						"slot_data": map[string]any{"starting_money": 75000},
						"players":   []map[string]any{{"slot": 3, "name": "Zoo"}},
						"slot_info": map[string]any{"3": map[string]any{"game": "Planet Zoo"}},
					}, map[string]any{"cmd": "ReceivedItems", "index": 0, "items": []map[string]any{{"item": 10000}, {"item": 13001}}})
				}
				r.received <- m
			}
		}
	}))
	t.Cleanup(srv.Close)
	return r, "ws://" + strings.TrimPrefix(srv.URL, "http://")
}

func (r *room) send(ctx context.Context, c *websocket.Conn, msgs ...map[string]any) {
	body, err := json.Marshal(msgs)
	if err != nil {
		r.t.Error(err)
		return
	}
	_ = c.Write(ctx, websocket.MessageText, body)
}

func (r *room) expect(cmd string) map[string]any {
	r.t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case m := <-r.received:
			if m["cmd"] == cmd {
				return m
			}
		case <-timeout:
			r.t.Fatalf("no %s", cmd)
		}
	}
}

func TestHandshakeItemsAndChecks(t *testing.T) {
	r, url := newRoom(t)
	ctx := t.Context()
	c := New(Options{Address: url, Slot: "Zoo", Logger: slog.New(slog.DiscardHandler)})
	go func() { _ = c.Run(ctx) }()

	connect := r.expect("Connect")
	if connect["game"] != Game || connect["name"] != "Zoo" || connect["items_handling"] != float64(7) {
		t.Errorf("connect %v", connect)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !c.State().Connected || len(c.State().Items) < 2 {
		if time.Now().After(deadline) {
			t.Fatal("never connected")
		}
		time.Sleep(10 * time.Millisecond)
	}
	s := c.State()
	if s.Seed != "S1" || s.Slot != 3 || !slices.Equal(s.Items, []int64{10000, 13001}) || !s.Checked[20000] {
		t.Errorf("state %+v", s)
	}
	c.Check(22000)
	locs, _ := r.expect("LocationChecks")["locations"].([]any)
	if !slices.Contains(locs, any(float64(22000))) {
		t.Errorf("checks %v", locs)
	}
	if !c.Done(22000) || !c.Done(20000) || c.Done(1) {
		t.Error("done")
	}

	// A gap in the item numbering asks for the list again.
	conn := <-r.conn
	r.send(ctx, conn, map[string]any{"cmd": "ReceivedItems", "index": 5, "items": []map[string]any{{"item": 1}}})
	r.expect("Sync")
	c.Goal()
	if st := r.expect("StatusUpdate"); st["status"] != float64(statusGoal) {
		t.Errorf("status %v", st)
	}
}
