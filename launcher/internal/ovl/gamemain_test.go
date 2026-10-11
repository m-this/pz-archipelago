package ovl

import (
	"slices"
	"testing"
)

const noneResearchable = "default_off_noneresearchable.mechanicresearchsettings"

// cobra-gm-patched.ovl is cobra-tools' rename of scenario_04 onto
// scenario_22_empty plus three gates added to the topology, the edit the
// shell makes to GameMain.
func TestRenameAndGatesAgreeWithCobra(t *testing.T) {
	f, _ := readFixture(t, "cobra-gm-vanilla.ovl")
	want, _ := readFixture(t, "cobra-gm-patched.ovl")
	if err := f.Rename("scenario_04.mechanicresearchsettings", "scenario_22_empty.mechanicresearchsettings"); err != nil {
		t.Fatal(err)
	}
	gates := []string{"ApBarrierGate2", "ApGateFoodShopsPizzaPen", "ApGateTransportBoatRideStation"}
	if err := f.AppendResearch(noneResearchable, gates); err != nil {
		t.Fatal(err)
	}
	out, _, err := f.Write()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Read(out, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	gotNames, wantNames := fileNames(got), fileNames(want)
	if !slices.Equal(gotNames, wantNames) {
		t.Fatalf("files %v, cobra %v", gotNames, wantNames)
	}
	for i := range got.Files {
		if got.Files[i].FileHash != want.Files[i].FileHash {
			t.Errorf("file %s: hash %d, cobra %d", gotNames[i], got.Files[i].FileHash, want.Files[i].FileHash)
		}
	}
	for _, name := range gotNames {
		if l, _ := got.Locate(name); l.Content == nil {
			t.Errorf("%s: no root entry", name)
			continue
		}
		switch name {
		case "c0research.fdb":
			compareFile(t, got, want, name, fdbRootSize)
		case "scenarioscripts.scenarioscriptutils.lua":
			compareFile(t, got, want, name, luaHeaderWords)
		default:
			g, err := got.ResearchEntries(name)
			if err != nil {
				t.Fatal(err)
			}
			w, err := want.ResearchEntries(name)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.EqualFunc(g, w, func(a, b Research) bool { return a.ItemName == b.ItemName && slices.Equal(a.Next, b.Next) }) {
				t.Errorf("%s: %v, cobra %v", name, g, w)
			}
		}
	}
}

func fileNames(f *File) []string {
	names := make([]string, len(f.Files))
	for i := range f.Files {
		names[i] = f.FileName(i)
	}
	return names
}

// A pointer out of a moved entry must move with it, and a pointer to the old
// end of the pool must stay at the end.
func TestAppendResearchCarriesPointers(t *testing.T) {
	f, _ := readFixture(t, "cobra-gm-vanilla.ovl")
	ra, err := f.researchArray(noneResearchable)
	if err != nil {
		t.Fatal(err)
	}
	l, c, pool := ra.located, ra.located.Content, ra.array.Pool
	guest := c.Fragments[slices.IndexFunc(c.Fragments, func(fr Fragment) bool { return fr.Link == ra.array })].Struct

	// Entry 1 gets a next_research list of one name, GuestSpawner, and the
	// list a second slot pointing at the end of the pool.
	g := f.grow(l.Archive, c, int(pool))
	list := g.appendStruct(make([]byte, 16))
	if err := g.commit(); err != nil {
		t.Fatal(err)
	}
	entry1 := ra.array.Offset + researchEntrySize
	c.Fragments = append(c.Fragments,
		Fragment{Link: Pointer{Pool: pool, Offset: entry1 + researchNextAt}, Struct: Pointer{Pool: pool, Offset: list}},
		Fragment{Link: Pointer{Pool: pool, Offset: list}, Struct: guest},
		Fragment{Link: Pointer{Pool: pool, Offset: list + 8}, Struct: Pointer{Pool: pool, Offset: c.Pools[pool].Size}},
	)
	c.PoolData[pool][entry1+researchNextAt+8] = 1

	if err := f.AppendResearch(noneResearchable, []string{"ApGateX"}); err != nil {
		t.Fatal(err)
	}
	entries, err := f.ResearchEntries(noneResearchable)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 || !slices.Equal(entries[1].Next, []string{"GuestSpawner"}) || entries[3].ItemName != "ApGateX" {
		t.Fatalf("entries %+v", entries)
	}
	end, _ := c.fragmentAt(Pointer{Pool: pool, Offset: list + 8})
	if c.Fragments[end].Struct.Offset != c.Pools[pool].Size {
		t.Errorf("end-of-pool pointer left at %d, the pool ends at %d", c.Fragments[end].Struct.Offset, c.Pools[pool].Size)
	}
}
