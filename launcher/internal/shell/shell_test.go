package shell

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/m-this/pz-archipelago/launcher/internal/ovl"
)

func gateID(t *testing.T, content string) int64 {
	t.Helper()
	for _, g := range contentGates() {
		if g.name == "ApGate"+content {
			return g.id
		}
	}
	t.Fatalf("no gate for %s", content)
	return 0
}

// patchedContent0 runs the shell over testdata/content0.ovl, a Content0 in
// miniature that cobra-tools built from five small databases.
func patchedContent0(t *testing.T) *ovl.File {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "content0.ovl"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := ovl.Read(raw, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := PatchContent0(context.Background(), f, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	out, _, err := f.Write()
	if err != nil {
		t.Fatal(err)
	}
	reread, err := ovl.Read(out, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return reread
}

// query runs one query against a database of the patched ovl and returns
// its rows as text.
func query(t *testing.T, f *ovl.File, fdb, q string) []string {
	t.Helper()
	db, err := f.FDB(fdb)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), fdb)
	if err := os.WriteFile(path, db, 0o600); err != nil {
		t.Fatal(err)
	}
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	rows, err := conn.QueryContext(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var a, b any
		if err := rows.Scan(&a, &b); err != nil {
			t.Fatal(err)
		}
		out = append(out, listText(a)+"|"+listText(b))
	}
	return out
}

func TestContent0Gates(t *testing.T) {
	f := patchedContent0(t)
	pizza, africa := gateID(t, "FoodShopsPizzaPen"), gateID(t, "AfricaThemeSetsScenery")
	itoa := func(i int64) string { return strings.TrimSpace(listText(i)) }

	minted := query(t, f, fdbResearch, "SELECT ResearchItem, ResearchItemID FROM ResearchItemData WHERE ResearchCategory = 'NoneResearchable'")
	if len(minted) != len(GateNames()) || !slices.Contains(minted, "ApGateFoodShopsPizzaPen|"+itoa(pizza)) ||
		!slices.Contains(minted, "ApBarrierGate2|50004") {
		t.Errorf("minted gates %v", minted)
	}
	if got := query(t, f, fdbResearch, "SELECT ResearchItem, ResearchItemID FROM ResearchItemData WHERE ResearchItem = 'FoodShopsPizzaPen'"); !slices.Equal(got, []string{"FoodShopsPizzaPen|101"}) {
		t.Errorf("the content's own research item moved: %v", got)
	}

	cases := []struct {
		fdb, q string
		want   []string
	}{
		{
			fdbHabitatBoundary, "SELECT BoundaryType, ResearchPack FROM Simulation ORDER BY rowid",
			[]string{"Hedge|50002", "Electric|50007", "Unknown|3"},
		},
		{
			fdbModularScenery, "SELECT SceneryPartName, ResearchItemID FROM Simulation ORDER BY rowid",
			[]string{"Pizza_Stall|" + itoa(pizza), "RS_Room_4x4|50000", "WS_Room_4x4|50001", "Tree|104"},
		},
		// The pack id column is a different id space and must not move.
		{
			fdbModularScenery, "SELECT SceneryPartName, ResearchPackID FROM Simulation ORDER BY rowid",
			[]string{"Pizza_Stall|101", "RS_Room_4x4|", "WS_Room_4x4|", "Tree|16"},
		},
		{
			fdbBlueprints, "SELECT BlueprintID, ResearchItemIDs FROM PrebuiltBlueprints ORDER BY rowid",
			[]string{"1|" + itoa(pizza) + ", 12003", "2|50000", "3|50001", "4|5", "5|" + itoa(africa)},
		},
		{
			fdbNotifications, "SELECT Name, SelectionContextType FROM Alerts ORDER BY rowid",
			[]string{"NoResearchCentre|None", "NoWorkshop|None", "NoStaffCentre|SceneryPlacement"},
		},
	}
	for _, c := range cases {
		if got := query(t, f, c.fdb, c.q); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.fdb, got, c.want)
		}
	}
}

func TestContent0Lua(t *testing.T) {
	f := patchedContent0(t)
	l, err := f.Locate(scriptUtils)
	if err != nil {
		t.Fatal(err)
	}
	buffers, err := l.Buffers()
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("..", "..", "..", "mod", "content0", scriptUtils))
	if err != nil {
		t.Fatal(err)
	}
	if string(buffers[0]) != string(want) {
		t.Error("the script-table hijack was not injected")
	}
}

func TestPackAndLocBuild(t *testing.T) {
	for name, build := range map[string]func() ([]byte, error){"pack": BuildPack, "loc": BuildLoc} {
		raw, err := build()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		f, err := ovl.Read(raw, nil, nil)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(f.Files) == 0 {
			t.Errorf("%s: empty", name)
		}
	}
}

func TestGateNames(t *testing.T) {
	names := GateNames()
	if len(names) != len(barrierGates)+51 || len(slices.Compact(slices.Sorted(slices.Values(names)))) != len(names) {
		t.Errorf("%d gate names, want %d distinct", len(names), len(barrierGates)+51)
	}
}
