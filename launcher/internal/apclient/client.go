// Package apclient holds the Archipelago session: one websocket, reconnected
// for as long as the client runs. It remembers what the room said and what
// it has to tell it; the game loop reads the first and adds to the second.
package apclient

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const (
	readLimitBytes = 32 << 20
	dialTimeout    = 30 * time.Second
	writeTimeout   = 10 * time.Second
	pingEvery      = 30 * time.Second
	backoffFirst   = time.Second
	backoffMax     = 30 * time.Second
	messagesKept   = 200
)

// Game is the name the slot must have been generated for.
const Game = "Planet Zoo"

// ErrRefused wraps a refusal reconnecting cannot fix: a wrong slot name,
// password or game.
var ErrRefused = errors.New("archipelago refused the connection")

type Options struct {
	// Address is host:port, or a ws:// or wss:// URL.
	Address  string
	Slot     string
	Password string
	Logger   *slog.Logger
}

// State is what the room has said, as of now.
type State struct {
	Connected bool
	Seed      string
	Slot      int
	SlotData  json.RawMessage
	// Items is every item received, in the order the room numbers them.
	Items []int64
	// Checked is every location the room knows is checked.
	Checked map[int64]bool
	Error   string
}

type Client struct {
	opts Options
	uuid string
	wake chan struct{}

	mu       sync.Mutex
	state    State
	sent     map[int64]bool
	goal     bool
	names    *names
	messages []string
}

func New(opts Options) *Client {
	id := make([]byte, 16)
	_, _ = rand.Read(id)
	return &Client{
		opts: opts, uuid: hex.EncodeToString(id), wake: make(chan struct{}, 1),
		state: State{Checked: map[int64]bool{}}, sent: map[int64]bool{}, names: newNames(),
	}
}

// State returns a copy of what the room has said.
func (c *Client) State() State {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.state
	s.Items = slices.Clone(c.state.Items)
	s.Checked = maps.Clone(c.state.Checked)
	return s
}

// Check reports locations. They are kept and sent again on every reconnect,
// so a check found while offline is not lost.
func (c *Client) Check(ids ...int64) {
	c.mu.Lock()
	for _, id := range ids {
		c.sent[id] = true
	}
	c.mu.Unlock()
	c.poke()
}

// Done reports whether a location is checked, by the room or by us.
func (c *Client) Done(id int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state.Checked[id] || c.sent[id]
}

// Goal tells the room the goal is reached.
func (c *Client) Goal() {
	c.mu.Lock()
	c.goal = true
	c.mu.Unlock()
	c.poke()
}

// Messages is the last chat lines the room sent.
func (c *Client) Messages() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.messages)
}

func (c *Client) poke() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *Client) note(text string) {
	c.mu.Lock()
	c.messages = append(c.messages, text)
	if len(c.messages) > messagesKept {
		c.messages = c.messages[len(c.messages)-messagesKept:]
	}
	c.mu.Unlock()
}

// urls is what to dial: a bare host:port tries wss first, as the
// Archipelago clients do, then ws.
func (c *Client) urls() []string {
	a := strings.TrimSpace(c.opts.Address)
	if strings.HasPrefix(a, "ws://") || strings.HasPrefix(a, "wss://") {
		return []string{a}
	}
	return []string{"wss://" + a, "ws://" + a}
}

// Run holds the session until ctx ends or the room refuses the slot.
func (c *Client) Run(ctx context.Context) error {
	backoff := backoffFirst
	for {
		start := time.Now()
		err := c.session(ctx)
		if ctx.Err() != nil {
			return nil //nolint:nilerr // a cancelled context is a clean stop
		}
		c.mu.Lock()
		c.state.Connected = false
		if err != nil {
			c.state.Error = err.Error()
		}
		c.mu.Unlock()
		if errors.Is(err, ErrRefused) {
			return err
		}
		if time.Since(start) > backoffMax {
			backoff = backoffFirst
		}
		c.opts.Logger.WarnContext(ctx, "archipelago session ended, will retry", "error", err, "in", backoff)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, backoffMax)
	}
}

func (c *Client) dial(ctx context.Context) (*websocket.Conn, error) {
	var errs []error
	for _, url := range c.urls() {
		dialCtx, cancel := context.WithTimeout(ctx, dialTimeout)
		conn, resp, err := websocket.Dial(dialCtx, url, &websocket.DialOptions{CompressionMode: websocket.CompressionContextTakeover})
		cancel()
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		if err == nil {
			conn.SetReadLimit(readLimitBytes)
			return conn, nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", url, err))
	}
	return nil, errors.Join(errs...)
}

func (c *Client) session(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	conn, err := c.dial(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.CloseNow() }()
	ready := make(chan struct{})
	done := make(chan error, 2)
	go func() { done <- c.read(ctx, conn, ready) }()
	go func() { done <- c.pump(ctx, conn, ready) }()
	first := <-done
	cancel()
	_ = conn.CloseNow()
	<-done
	return first
}

func (c *Client) send(ctx context.Context, conn *websocket.Conn, msg any) error {
	body, err := json.Marshal([]any{msg})
	if err != nil {
		return err
	}
	writeCtx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	return conn.Write(writeCtx, websocket.MessageText, body)
}

func (c *Client) read(ctx context.Context, conn *websocket.Conn, ready chan struct{}) error {
	for {
		kind, body, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		if kind != websocket.MessageText {
			continue
		}
		var messages []json.RawMessage
		if err := json.Unmarshal(body, &messages); err != nil {
			return fmt.Errorf("unreadable packet: %w", err)
		}
		for _, m := range messages {
			if err := c.handle(ctx, conn, m, ready); err != nil {
				return err
			}
		}
	}
}

func (c *Client) handle(ctx context.Context, conn *websocket.Conn, m json.RawMessage, ready chan struct{}) error {
	var h header
	if err := json.Unmarshal(m, &h); err != nil {
		return err
	}
	switch h.Cmd {
	case "RoomInfo":
		var room roomInfo
		if err := json.Unmarshal(m, &room); err != nil {
			return err
		}
		c.mu.Lock()
		c.state.Seed = room.SeedName
		c.mu.Unlock()
		return c.send(ctx, conn, connectMessage{
			Cmd: "Connect", Password: c.opts.Password, Game: Game, Name: c.opts.Slot, UUID: c.uuid,
			Version: clientVersion, ItemsHandling: itemsHandling, Tags: []string{}, SlotData: true,
		})
	case "Connected":
		return c.onConnected(ctx, conn, m, ready)
	case "ConnectionRefused":
		var r connectionRefused
		_ = json.Unmarshal(m, &r)
		return fmt.Errorf("%w: %s", ErrRefused, strings.Join(r.Errors, ", "))
	case "ReceivedItems":
		return c.onReceivedItems(ctx, conn, m)
	case "RoomUpdate":
		var u roomUpdate
		if err := json.Unmarshal(m, &u); err != nil {
			return err
		}
		c.mu.Lock()
		for _, id := range u.CheckedLocations {
			c.state.Checked[id] = true
		}
		c.mu.Unlock()
	case "DataPackage":
		var p dataPackage
		if err := json.Unmarshal(m, &p); err != nil {
			return err
		}
		c.mu.Lock()
		for game, t := range p.Data.Games {
			c.names.items[game] = invert(t.ItemNameToID)
			c.names.places[game] = invert(t.LocationNameToID)
		}
		c.mu.Unlock()
	case "PrintJSON":
		var p printJSON
		if json.Unmarshal(m, &p) == nil {
			c.mu.Lock()
			text := p.text(c.names)
			c.mu.Unlock()
			c.note(text)
			c.opts.Logger.InfoContext(ctx, "archipelago", "message", text)
		}
	}
	return nil
}

func invert(m map[string]int64) map[int64]string {
	out := make(map[int64]string, len(m))
	for name, id := range m {
		out[id] = name
	}
	return out
}

func (c *Client) onConnected(ctx context.Context, conn *websocket.Conn, m json.RawMessage, ready chan struct{}) error {
	var p connected
	if err := json.Unmarshal(m, &p); err != nil {
		return err
	}
	var games []string
	c.mu.Lock()
	c.state.Connected, c.state.Slot, c.state.SlotData, c.state.Error = true, p.Slot, p.SlotData, ""
	for _, id := range p.CheckedLocations {
		c.state.Checked[id] = true
	}
	for _, pl := range p.Players {
		name := pl.Alias
		if name == "" {
			name = pl.Name
		}
		c.names.players[pl.Slot] = name
	}
	for slot, info := range p.SlotInfo {
		var n int
		if _, err := fmt.Sscan(slot, &n); err == nil {
			c.names.games[n] = info.Game
		}
		if _, have := c.names.items[info.Game]; !have && !slices.Contains(games, info.Game) {
			games = append(games, info.Game)
		}
	}
	c.mu.Unlock()
	c.opts.Logger.InfoContext(ctx, "connected to archipelago", "slot", c.opts.Slot, "checked", len(p.CheckedLocations))
	close(ready)
	for _, game := range games {
		if err := c.send(ctx, conn, getDataPackage{Cmd: "GetDataPackage", Games: []string{game}}); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) onReceivedItems(ctx context.Context, conn *websocket.Conn, m json.RawMessage) error {
	var p receivedItems
	if err := json.Unmarshal(m, &p); err != nil {
		return err
	}
	c.mu.Lock()
	switch {
	case p.Index == 0:
		c.state.Items = c.state.Items[:0]
	case p.Index != len(c.state.Items):
		c.mu.Unlock()
		c.opts.Logger.WarnContext(ctx, "the item list skipped, asking for it again", "index", p.Index)
		return c.send(ctx, conn, plainCommand{Cmd: "Sync"})
	}
	for _, it := range p.Items {
		c.state.Items = append(c.state.Items, it.Item)
	}
	c.mu.Unlock()
	return nil
}

// pump sends every check and the goal each time something changes, and
// pings; a session that stopped answering is dropped and redialled.
func (c *Client) pump(ctx context.Context, conn *websocket.Conn, ready chan struct{}) error {
	select {
	case <-ready:
	case <-ctx.Done():
		return nil
	}
	ping := time.NewTicker(pingEvery)
	defer ping.Stop()
	for {
		c.mu.Lock()
		checks := slices.Sorted(maps.Keys(c.sent))
		goal := c.goal
		c.mu.Unlock()
		if len(checks) > 0 {
			if err := c.send(ctx, conn, locationChecks{Cmd: "LocationChecks", Locations: checks}); err != nil {
				return err
			}
		}
		if goal {
			if err := c.send(ctx, conn, statusUpdate{Cmd: "StatusUpdate", Status: statusGoal}); err != nil {
				return err
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-c.wake:
		case <-ping.C:
			pingCtx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := conn.Ping(pingCtx)
			cancel()
			if err != nil {
				return err
			}
		}
	}
}
