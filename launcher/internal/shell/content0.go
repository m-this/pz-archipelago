package shell

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"strconv"
	"strings"

	"github.com/m-this/pz-archipelago/launcher/internal/ovl"
	"github.com/m-this/pz-archipelago/mod"
)

// The Content0 databases the shell edits, and the Lua module it replaces.
const (
	fdbResearch        = "c0research.fdb"
	fdbHabitatBoundary = "c0habitatboundary.fdb"
	fdbModularScenery  = "c0modularscenery.fdb"
	fdbBlueprints      = "c0blueprints.fdb"
	fdbNotifications   = "c0notifications.fdb"
	scriptUtils        = "scenarioscripts.scenarioscriptutils.lua"
)

// fdbEdit is one database's change. repoint maps a content's research item
// id to its gate, read from c0research before the gates are minted there.
type fdbEdit struct {
	name string
	edit func(ctx context.Context, tx *sql.Tx, repoint map[int64]int64) error
}

var content0Edits = []fdbEdit{
	{fdbHabitatBoundary, repointBarriers},
	{fdbModularScenery, repointScenery},
	{fdbBlueprints, repointBlueprints},
	{fdbNotifications, neuterPlacementAlerts},
}

// PatchContent0 applies the shell to the player's own Content0 ovl: the
// gates in its databases and the script-table hijack in its Lua.
func PatchContent0(ctx context.Context, f *ovl.File, scratch string) error {
	var repoint map[int64]int64
	err := patchFDB(ctx, f, scratch, fdbResearch, func(tx *sql.Tx) error {
		var err error
		if repoint, err = readRepoint(ctx, tx); err != nil {
			return err
		}
		return mintGates(ctx, tx)
	})
	if err != nil {
		return err
	}
	for _, e := range content0Edits {
		if err := patchFDB(ctx, f, scratch, e.name, func(tx *sql.Tx) error { return e.edit(ctx, tx, repoint) }); err != nil {
			return err
		}
	}
	source, err := fs.ReadFile(mod.Sources, "content0/"+scriptUtils)
	if err != nil {
		return err
	}
	return f.ReplaceLua(scriptUtils, source)
}

func patchFDB(ctx context.Context, f *ovl.File, scratch, name string, edit func(*sql.Tx) error) error {
	db, err := f.FDB(name)
	if err != nil {
		return err
	}
	edited, err := editFDB(ctx, scratch, db, edit)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return f.ReplaceFDB(name, edited)
}

// readRepoint maps each mechanic content's research item, found by name,
// to its gate. A content the database does not have is skipped, as upstream
// did: a game update may drop one.
func readRepoint(ctx context.Context, tx *sql.Tx) (map[int64]int64, error) {
	rows, err := tx.QueryContext(ctx, "SELECT ResearchItem, ResearchItemID FROM ResearchItemData")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	ids := map[string]int64{}
	for rows.Next() {
		var name string
		var id int64
		if err := rows.Scan(&name, &id); err != nil {
			return nil, err
		}
		ids[name] = id
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	repoint := map[int64]int64{}
	for _, g := range contentGates() {
		if id, ok := ids[strings.TrimPrefix(g.name, "ApGate")]; ok {
			repoint[id] = g.id
		}
	}
	return repoint, nil
}

func mintGates(ctx context.Context, tx *sql.Tx) error {
	var rows [][]any
	for _, g := range append(append([]gate{}, barrierGates...), contentGates()...) {
		rows = append(rows, []any{g.name, g.id})
	}
	return execEach(ctx, tx, "INSERT INTO ResearchItemData (ResearchItem, ResearchItemID, ResearchCategory, "+
		"HoursToComplete, Icon, MechanicItemIcon) VALUES (?, ?, 'NoneResearchable', 1.0, NULL, NULL)", rows)
}

func repointBarriers(ctx context.Context, tx *sql.Tx, _ map[int64]int64) error {
	var rows [][]any
	for boundary, id := range barrierGrade {
		rows = append(rows, []any{id, boundary})
	}
	return execEach(ctx, tx, "UPDATE Simulation SET ResearchPack = ? WHERE BoundaryType = ?", rows)
}

// repointScenery moves every scenery part gated on a content's research
// item onto its gate, then gates the basic research centre and workshop
// rooms. It leaves the research pack columns alone: they hold pack ids, a
// different id space, and re-pointing them unlocked enrichment upstream.
func repointScenery(ctx context.Context, tx *sql.Tx, repoint map[int64]int64) error {
	var rows [][]any
	for content, g := range repoint {
		rows = append(rows, []any{g, content})
	}
	if err := execEach(ctx, tx, "UPDATE Simulation SET ResearchItemID = ? WHERE ResearchItemID = ?", rows); err != nil {
		return err
	}
	rows = nil
	for _, room := range facilityRooms {
		rows = append(rows, []any{room.gate, room.part})
	}
	return execEach(ctx, tx, "UPDATE Simulation SET ResearchItemID = ? WHERE SceneryPartName = ?", rows)
}

func neuterPlacementAlerts(ctx context.Context, tx *sql.Tx, _ map[int64]int64) error {
	_, err := tx.ExecContext(ctx, "UPDATE Alerts SET SelectionContextType = 'None' WHERE Name IN (?, ?)",
		placementAlerts[0], placementAlerts[1])
	return err
}

// repointBlueprints rewrites the comma-separated research item list of each
// prebuilt blueprint, then adds the facility gate to the research centre and
// workshop blueprints.
func repointBlueprints(ctx context.Context, tx *sql.Tx, repoint map[int64]int64) error {
	lists, err := blueprintLists(ctx, tx, "")
	if err != nil {
		return err
	}
	var rows [][]any
	for id, list := range lists {
		if changed, ok := repointList(list, repoint); ok {
			rows = append(rows, []any{changed, id})
		}
	}
	if err := execEach(ctx, tx, "UPDATE PrebuiltBlueprints SET ResearchItemIDs = ? WHERE BlueprintID = ?", rows); err != nil {
		return err
	}
	for _, fb := range facilityBlueprints {
		lists, err := blueprintLists(ctx, tx, fb.where)
		if err != nil {
			return err
		}
		rows = nil
		for id, list := range lists {
			gated := strconv.FormatInt(fb.gate, 10)
			if list != "" {
				gated = list + ", " + gated
			}
			rows = append(rows, []any{gated, id})
		}
		if err := execEach(ctx, tx, "UPDATE PrebuiltBlueprints SET ResearchItemIDs = ? WHERE BlueprintID = ?", rows); err != nil {
			return err
		}
	}
	return nil
}

// blueprintLists reads ResearchItemIDs as text; NULL, empty and 0 all read
// as no list, as Python's falsy test did upstream.
func blueprintLists(ctx context.Context, tx *sql.Tx, where string) (map[int64]string, error) {
	query := "SELECT BlueprintID, ResearchItemIDs FROM PrebuiltBlueprints"
	if where != "" {
		query += " WHERE " + where
	}
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	lists := map[int64]string{}
	for rows.Next() {
		var id int64
		var list any
		if err := rows.Scan(&id, &list); err != nil {
			return nil, err
		}
		lists[id] = listText(list)
	}
	return lists, rows.Err()
}

func listText(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case int64:
		if t == 0 {
			return ""
		}
		return strconv.FormatInt(t, 10)
	case []byte:
		return string(t)
	case string:
		return t
	}
	return fmt.Sprint(v)
}

// repointList swaps each id in a list for its gate. It reports false when
// nothing changed, so the row is left exactly as it was.
func repointList(list string, repoint map[int64]int64) (string, bool) {
	if list == "" {
		return "", false
	}
	parts := strings.Split(list, ",")
	changed := false
	for i, p := range parts {
		p = strings.TrimSpace(p)
		parts[i] = p
		if id, err := strconv.ParseInt(p, 10, 64); err == nil && isDigits(p) {
			if g, ok := repoint[id]; ok {
				parts[i] = strconv.FormatInt(g, 10)
				changed = true
			}
		}
	}
	return strings.Join(parts, ", "), changed
}

func isDigits(s string) bool {
	return s != "" && !strings.ContainsFunc(s, func(r rune) bool { return r < '0' || r > '9' })
}
