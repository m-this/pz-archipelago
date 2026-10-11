package play

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/m-this/pz-archipelago/gamedata"
	"github.com/m-this/pz-archipelago/launcher/internal/apclient"
	"github.com/m-this/pz-archipelago/launcher/internal/game/mem"
	"github.com/m-this/pz-archipelago/launcher/internal/game/pz"
)

const (
	tickEvery      = time.Second
	freshWaitTicks = 8
	marketMinFill  = 8
)

type Options struct {
	Room     *apclient.Client
	StateDir string
	Logger   *slog.Logger
	// Open attaches to the game; mem.Open on the real one.
	Open func() (mem.Process, error)
	// Tick is the loop period, a second unless a test shortens it.
	Tick time.Duration
}

// Status is what the loop is doing, for the page.
type Status struct {
	Attached  bool
	InSession bool
	Ready     bool
	Applied   int
	Received  int
	Goal      bool
	Preflight []pz.Check
}

// Loop runs once a second for as long as the client does.
type Loop struct {
	opts      Options
	store     *store
	storeSeed string
	s         *session
	goalSent  bool
	warned    map[string]bool

	mu     sync.Mutex
	status Status
}

func New(opts Options) *Loop { return &Loop{opts: opts, warned: map[string]bool{}} }

// Status returns a copy of the loop's status.
func (l *Loop) Status() Status {
	l.mu.Lock()
	defer l.mu.Unlock()
	s := l.status
	s.Preflight = slices.Clone(l.status.Preflight)
	return s
}

func (l *Loop) setStatus(edit func(*Status)) {
	l.mu.Lock()
	edit(&l.status)
	l.mu.Unlock()
}

// Run ticks until ctx ends, then removes every patch it placed in the game.
func (l *Loop) Run(ctx context.Context) error {
	period := l.opts.Tick
	if period == 0 {
		period = tickEvery
	}
	t := time.NewTicker(period)
	defer t.Stop()
	defer l.detach(true)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			l.tick(ctx)
		}
	}
}

// warnOnce logs a problem the loop will keep running into, once.
func (l *Loop) warnOnce(key string, err error) {
	if !l.warned[key] {
		l.warned[key] = true
		l.opts.Logger.Warn("the client cannot go on with part of its work", "what", key, "error", err)
	}
}

// step runs one part of a tick. Game memory is untrusted input: a step that
// trips over it is logged and the rest of the tick goes on.
func (l *Loop) step(name string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			l.opts.Logger.Error("a client step failed", "step", name, "panic", fmt.Sprint(r))
		}
	}()
	fn()
}

func (l *Loop) tick(ctx context.Context) {
	room := l.opts.Room.State()
	if room.Seed == "" || room.Slot == 0 {
		return
	}
	if l.storeSeed != room.Seed {
		st, err := openStore(l.opts.StateDir, room.Seed)
		if err != nil {
			l.warnOnce("open the client state", err)
			return
		}
		l.store, l.storeSeed, l.goalSent = st, room.Seed, false
	}
	slot, err := parseSlotData(room.SlotData)
	if err != nil {
		l.warnOnce("read the slot data", err)
		return
	}
	if !l.attach() {
		return
	}
	s := l.s
	active := s.name.Read() == pz.APParkName && s.habitat.ScenarioMode()
	l.setStatus(func(st *Status) { st.Attached, st.InSession, st.Received = true, active, len(room.Items) })
	if !active {
		if s.active {
			s.active = false
			l.opts.Logger.InfoContext(ctx, "the AP park unloaded, restoring the markets before the game frees them")
			s.unload()
			l.setStatus(func(st *Status) { st.Ready = false })
		}
		return
	}
	if !s.active {
		s.active = true
		l.opts.Logger.InfoContext(ctx, "AP park detected, the client is active")
	}
	l.runSteps(ctx, room, slot)
}

func (l *Loop) runSteps(ctx context.Context, room apclient.State, slot SlotData) {
	s := l.s
	r := Summarise(room.Items, slot)
	first := !s.ready
	steps := []struct {
		name string
		fn   func()
	}{
		{"preflight", s.preflight},
		{"detect", func() { s.detect(l.opts.Room) }},
		{"apply_items", func() { l.applyItems(room, slot, r) }},
		{"permits", func() { s.permit.Reconcile(r.Blocked()) }},
		{"conservation", func() { s.releases.SetLocked(!r.Conservation) }},
		{"barriers", func() {
			if r.BarrierLevels > 0 {
				s.rewards.ReconcileBarriers(r.BarrierLevels)
			}
		}},
		{"research", func() { s.research.Reconcile(r.ResearchCentre, r.Workshop) }},
		{"facility_reveal", func() { s.rewards.ReconcileFacilities(r.facilities()) }},
		{"mechanic_content", func() { s.rewards.ReconcileMechanic(r.MechanicRewards) }},
		{"rewards", func() { s.reconcileRewards(r) }},
		{"terrain", func() { s.terrain.Reconcile(r.WaterTools) }},
		{"exhibit_enrichment", func() { s.enrich.Reconcile(r.Levels[pz.FamilyExhibitEnrichment]) }},
		{"market", func() { s.reconcileMarket(r) }},
		{"market_fill", s.fillMarkets},
	}
	for _, st := range steps {
		start := time.Now()
		l.step(st.name, st.fn)
		if first && time.Since(start) > 500*time.Millisecond {
			l.opts.Logger.InfoContext(ctx, "first tick", "step", st.name, "took", time.Since(start).Round(100*time.Millisecond))
		}
	}
	if first {
		s.ready = true
		l.opts.Logger.InfoContext(ctx, "READY: every gate is active", "items", len(room.Items))
	}
	l.checkGoal(slot)
	l.setStatus(func(st *Status) { st.Ready, st.Applied, st.Preflight = true, l.store.get(room.Slot).Applied, s.report })
}

func (r Received) facilities() []string {
	var out []string
	if r.ResearchCentre {
		out = append(out, "research_centre")
	}
	if r.Workshop {
		out = append(out, "workshop")
	}
	return out
}

func (l *Loop) attach() bool {
	if l.s != nil {
		base, _ := l.s.game.Proc().Module()
		if _, err := l.s.game.Proc().Read(base, 2); err == nil {
			return true
		}
		l.opts.Logger.Info("the game closed")
		l.detach(false)
	}
	proc, err := l.opts.Open()
	if err != nil {
		if !errors.Is(err, mem.ErrNotRunning) {
			l.warnOnce("attach to the game", err)
		}
		l.setStatus(func(st *Status) { *st = Status{} })
		return false
	}
	l.s = newSession(pz.New(proc, l.opts.Logger), l.opts.Logger)
	l.opts.Logger.Info("attached to the game")
	return true
}

// detach leaves the game; restore is false when the process is already gone.
func (l *Loop) detach(restore bool) {
	if l.s == nil {
		return
	}
	if restore {
		l.s.shutdown()
	}
	_ = l.s.game.Proc().Close()
	l.s = nil
	l.setStatus(func(st *Status) { *st = Status{} })
}

// applyItems applies the received items the client has not applied yet. On
// a fresh save it first sets the room's starting money and starts the money
// ledger over, so everything received is granted on top of it.
func (l *Loop) applyItems(room apclient.State, slot SlotData, r Received) {
	s := l.s
	st := l.store.get(room.Slot)
	years, known := s.age.Read()
	if st.Applied < len(room.Items) && !known && s.freshWait < freshWaitTicks {
		s.freshWait++
		l.opts.Logger.Info("waiting for the park age, so starting money lands before the first cash item",
			"tick", s.freshWait, "of", freshWaitTicks)
		return
	}
	if s.initialApplied < 0 {
		s.initialApplied = st.Applied
	}
	if known && years < freshYears {
		switch {
		case !fresh(years, known, s.game):
			s.freshDone = false
		case !s.freshDone:
			s.freshDone = true
			l.opts.Logger.Info("a new AP zoo: setting the starting money and granting every item received")
			if slot.StartingMoney != nil && !s.game.Write(pz.Cash, *slot.StartingMoney) {
				l.opts.Logger.Info("cannot set the starting money yet")
			}
			st.Cash, st.Credits = 0, 0
			if s.initialApplied > 0 {
				st.Applied = 0
			}
		}
	}
	for st.Applied < len(room.Items) {
		id := room.Items[st.Applied]
		it, known := lookupItem(id)
		if known && !s.apply(it) {
			if s.pausedAt != st.Applied {
				s.pausedAt = st.Applied
				l.opts.Logger.Info("pausing item application, will retry", "index", st.Applied, "item", it.Name)
			}
			break
		}
		if !known {
			l.opts.Logger.Warn("an item this client does not know", "id", id)
		}
		st.Applied++
		s.pausedAt = -1
	}
	st = settle(st, r, s.game)
	if err := l.store.set(room.Slot, st); err != nil {
		l.warnOnce("save the client state", err)
	}
}

func (l *Loop) checkGoal(slot SlotData) {
	if l.goalSent {
		return
	}
	goal := GoalLocations(slot)
	if len(goal) == 0 {
		if slot.Goal.Type != "" {
			l.warnOnce("match the goal to a location", fmt.Errorf("goal %q has no location; it can never complete", slot.Goal.Type))
		}
		return
	}
	for _, id := range goal {
		if !l.opts.Room.Done(int64(id)) {
			return
		}
	}
	l.goalSent = true
	l.opts.Room.Goal()
	l.setStatus(func(st *Status) { st.Goal = true })
	l.opts.Logger.Info("goal complete, the multiworld has been told")
}

// session is everything built on one attached game process.
type session struct {
	game         *pz.Game
	log          *slog.Logger
	name         *pz.ParkName
	age          *pz.ParkAge
	permit       *pz.PermitGate
	research     *pz.ResearchGate
	terrain      *pz.TerrainGate
	enrich       *pz.EnrichGate
	habitat      *pz.MarketGate
	exhibit      *pz.MarketGate
	rewards      *pz.Rewards
	births       *pz.Births
	exhibits     *pz.Exhibits
	releases     *pz.Releases
	conservation *pz.Conservation

	active         bool
	ready          bool
	report         []pz.Check
	initialApplied int
	freshDone      bool
	freshWait      int
	pausedAt       int
	bred           map[gamedata.SpeciesKey]bool
	acquired       map[gamedata.SpeciesKey]bool
	marketAllowed  []gamedata.SpeciesKey
	marketIDs      []uint32
}

func newSession(g *pz.Game, log *slog.Logger) *session {
	s := &session{
		game: g, log: log, name: g.NewParkName(), age: g.NewParkAge(),
		permit: g.NewPermitGate(), research: g.NewResearchGate(), terrain: g.NewTerrainGate(),
		enrich: g.NewEnrichGate(), rewards: g.NewRewards(), births: g.NewBirths(), exhibits: g.NewExhibits(),
		releases: g.NewReleases(), initialApplied: -1, pausedAt: -1,
		bred: map[gamedata.SpeciesKey]bool{}, acquired: map[gamedata.SpeciesKey]bool{},
	}
	s.habitat, s.exhibit = g.NewMarketGates()
	s.conservation = g.NewConservation(s.births, s.releases)
	return s
}

// preflight runs once, before any detour goes in: it inspects the game's
// own code at every site.
func (s *session) preflight() {
	if s.report != nil {
		return
	}
	s.report = s.game.Preflight()
	ok := 0
	for _, c := range s.report {
		if c.OK {
			ok++
		} else {
			s.log.Error("preflight: a site the client needs is not as expected", "site", c.Name, "detail", c.Detail)
		}
	}
	s.log.Info("preflight", "ok", ok, "of", len(s.report))
}

// apply applies one item. Most are enforced by the per-tick reconciles, so
// applying them only acknowledges; false stalls the queue at this item.
func (s *session) apply(it gamedata.Item) bool {
	e := it.Effect
	switch e.Kind {
	case gamedata.Permit, gamedata.WaterTools:
		return s.permit.Installed()
	case gamedata.ConservationProgram:
		s.releases.SetLocked(false)
		return true
	case gamedata.ResearchReward:
		if isMechanic(e.Content) {
			return true
		}
		return s.rewards.Grant(e.Content)
	case gamedata.FacilityUnlock, gamedata.Cash, gamedata.ConservationCredits, gamedata.Progressive:
		return true
	}
	return false
}

func (s *session) reconcileRewards(r Received) {
	s.rewards.ReconcileRewards(r.AnimalRewards, AnimalRewardUniverse())
	for _, f := range []pz.Family{pz.FamilySupplement, pz.FamilyBreeding, pz.FamilyEducation, pz.FamilyExhibitEnrichment} {
		s.rewards.ReconcileLevels(f, r.Levels[f])
	}
}

func (s *session) reconcileMarket(r Received) {
	if !s.habitat.ScenarioMode() {
		return
	}
	allowed := r.Allowed()
	snap, ok := s.game.Research.Snapshot()
	if !ok {
		return
	}
	var ids []uint32
	for _, key := range allowed {
		if h, ok := s.game.Research.Handle(key, snap); ok {
			ids = append(ids, h)
		}
	}
	if len(ids) == 0 && len(allowed) > 0 {
		return
	}
	if slices.Equal(allowed, s.marketAllowed) && slices.Equal(ids, s.marketIDs) {
		return
	}
	if !s.habitat.Apply(ids) {
		return
	}
	s.marketAllowed, s.marketIDs = allowed, ids
	s.habitat.ExpireBlocked(ids)
	if s.exhibit.ScenarioMode() {
		s.exhibit.Apply(ids)
		s.exhibit.ExpireBlocked(ids)
	}
}

func (s *session) fillMarkets() {
	for _, m := range []*pz.MarketGate{s.habitat, s.exhibit} {
		if m.ScenarioMode() {
			m.EnsureMinFill(marketMinFill)
		}
	}
}

// detect turns what happened in the game since the last tick into checks.
func (s *session) detect(room *apclient.Client) {
	snap, snapOK := s.game.Research.Snapshot()
	born, acquired := s.births.Poll()
	eb, ea := s.exhibits.Poll()
	for _, k := range slices.Concat(born, eb) {
		s.bred[k] = true
	}
	for _, k := range slices.Concat(acquired, ea) {
		s.acquired[k] = true
	}
	s.conservation.Poll()
	guests, guestsOK := s.game.Read(pz.GuestCount)
	rating, ratingOK := s.game.Read(pz.ZooRating)
	rating = math.Round(rating*2) / 2
	var fired []int64
	for _, l := range gamedata.Locations {
		id := int64(l.ID)
		if room.Done(id) {
			continue
		}
		hit := false
		switch l.Kind {
		case gamedata.WelfareResearch:
			hit = snapOK && s.game.Research.WelfareLevelComplete(l.Species, int(l.Level), snap)
		case gamedata.MechanicResearch:
			hit = snapOK && s.game.Research.MechanicComplete(l.ResearchKey, snap)
		case gamedata.FirstBreeding:
			hit = s.bred[l.Species]
		case gamedata.FirstAcquisition:
			hit = s.acquired[l.Species]
		case gamedata.ConservationRelease:
			hit = s.conservation.Released[l.Species]
		case gamedata.Milestone:
			if l.Metric == gamedata.GuestCount {
				hit = guestsOK && guests >= float64(l.Threshold)
			} else {
				hit = ratingOK && rating >= float64(l.Threshold)
			}
		}
		if hit {
			s.log.Info("check", "location", l.LocationName())
			fired = append(fired, id)
		}
	}
	if len(fired) > 0 {
		room.Check(fired...)
	}
}

func (s *session) unload() {
	s.habitat.Restore()
	s.exhibit.Restore()
	s.marketAllowed, s.marketIDs = nil, nil
	s.name.Forget()
	s.ready = false
}

// shutdown removes every patch: bytecode, markets, detours.
func (s *session) shutdown() {
	s.terrain.Shutdown()
	s.enrich.Shutdown()
	s.habitat.Shutdown()
	s.exhibit.Shutdown()
	if err := s.game.Close(); err != nil {
		s.log.Warn("a detour could not be removed", "error", err)
	}
}
