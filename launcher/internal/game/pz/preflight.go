package pz

import (
	"bytes"
	"fmt"

	"github.com/m-this/pz-archipelago/launcher/internal/game/mem"
)

// Check is one line of the preflight report.
type Check struct {
	Name   string
	OK     bool
	Detail string
}

// Preflight reports whether every address the client depends on still holds
// what it expects. A game patch shows up here first, naming the sites it
// moved. It only reads.
func (g *Game) Preflight() []Check {
	var out []Check
	for _, s := range Sites {
		out = append(out, g.checkSite(s))
	}
	out = append(out, g.checkRoots())
	for _, m := range []Metric{Cash, ConservationCredits, ZooRating, GuestCount} {
		v, ok := g.Read(m)
		out = append(out, Check{Name: m.Name, OK: ok, Detail: fmt.Sprintf("%.2f", v)})
	}
	if snap, ok := g.Research.Snapshot(); ok {
		out = append(out, Check{Name: "research", OK: len(snap.Records()) > 0, Detail: fmt.Sprintf("%d records", len(snap.Records()))})
	} else {
		out = append(out, Check{Name: "research", Detail: "the research map is not readable"})
	}
	return out
}

func (g *Game) checkSite(s Site) Check {
	site := g.base + s.RVA
	if g.hooks.Active(s.Name) {
		return Check{Name: s.Name, OK: true, Detail: "installed"}
	}
	cur, err := g.proc.Read(site, len(s.Orig))
	switch {
	case err != nil:
		return Check{Name: s.Name, Detail: "unreadable"}
	case bytes.Equal(cur, s.Orig):
		return Check{Name: s.Name, OK: true, Detail: "intact"}
	case g.leaked(s, site) || g.dead(site):
		return Check{Name: s.Name, OK: true, Detail: "a detour from an earlier run, recovered when the gate installs"}
	}
	if s.AOB != "" {
		switch hits := mem.ScanModule(g.proc, mem.MustPattern(s.AOB), 2); len(hits) {
		case 1:
			return Check{Name: s.Name, OK: true, Detail: fmt.Sprintf("moved by %d bytes, found by its signature", int64(hits[0])-int64(site))}
		case 0:
		default:
			return Check{Name: s.Name, Detail: "its signature matches more than once"}
		}
	}
	return Check{Name: s.Name, Detail: fmt.Sprintf("holds %x, the game was patched", cur)}
}

func (g *Game) checkRoots() Check {
	hits := mem.ScanModule(g.proc, rootClusterAOB, 2)
	if len(hits) != 1 {
		return Check{Name: "anchor_roots", Detail: fmt.Sprintf("the root signature matched %d times, the chains fall back to their RVAs", len(hits))}
	}
	if g.root(rootCash) != g.base+rootCash {
		return Check{Name: "anchor_roots", OK: true, Detail: "moved, found by their signature"}
	}
	return Check{Name: "anchor_roots", OK: true, Detail: "intact"}
}
