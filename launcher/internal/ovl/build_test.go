package ovl

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// buildFromDir adds every file of a mod source directory, as cobra-tools'
// "new" was given them for the fixtures.
func buildFromDir(t *testing.T, dir string) *File {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "*"))
	if err != nil {
		t.Fatal(err)
	}
	var b Builder
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(p)
		switch ext := filepath.Ext(name); ext {
		case ".lua":
			b.AddLua(strings.TrimSuffix(name, ext), raw)
		case ".txt":
			b.AddText(strings.TrimSuffix(name, ext), string(raw))
		case ".animalresearchstartunlockedsettings":
			b.AddEmptyResearchStart(strings.TrimSuffix(name, ext))
		}
	}
	f, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := f.Write()
	if err != nil {
		t.Fatal(err)
	}
	reread, err := Read(out, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return reread
}

// cobra-tools lays roots out in an order it does not fix, so the comparison
// is per file: the tables must match exactly, and each file's struct, the
// strings it points to and its buffers.
func TestBuildAgreesWithCobraNew(t *testing.T) {
	cases := map[string]string{"pack": "cobra-pack.ovl", "pack_loc": "cobra-loc.ovl"}
	for dir, fixture := range cases {
		t.Run(dir, func(t *testing.T) {
			got := buildFromDir(t, filepath.Join("..", "..", "..", "mod", dir))
			want, _ := readFixture(t, fixture)
			if got.header != want.header || got.counts != want.counts {
				t.Errorf("header %+v %+v, cobra %+v %+v", got.header, got.counts, want.header, want.counts)
			}
			if !bytes.Equal(got.Names, want.Names) || !slices.Equal(got.Mimes, want.Mimes) ||
				!slices.Equal(got.Triplets, want.Triplets) || !slices.Equal(got.Files, want.Files) ||
				!bytes.Equal(got.ArchiveNames, want.ArchiveNames) {
				t.Fatalf("file tables differ:\n%q\n%q\n%+v\n%+v", got.Names, want.Names, got.Files, want.Files)
			}
			gc, _ := got.Content(got.Archives[0])
			wc, _ := want.Content(want.Archives[0])
			if !slices.Equal(gc.PoolGroups, wc.PoolGroups) || !slices.Equal(gc.DataEntries, wc.DataEntries) ||
				!slices.Equal(gc.BufferEntries, wc.BufferEntries) || !slices.Equal(gc.BufferGroups, wc.BufferGroups) ||
				len(gc.Fragments) != len(wc.Fragments) || len(gc.RootEntries) != len(wc.RootEntries) {
				t.Fatalf("archive tables differ:\n%+v\n%+v", gc, wc)
			}
			ge, we := got.Archives[0].Entry, want.Archives[0].Entry
			ge.CompressedSize, we.CompressedSize = 0, 0
			if ge != we || got.ArchivesMeta[0] != want.ArchivesMeta[0] {
				t.Errorf("archive entry %+v %+v, cobra %+v %+v", ge, got.ArchivesMeta, we, want.ArchivesMeta)
			}
			for i := range got.Files {
				compareStruct(t, got, want, got.FileName(i))
			}
		})
	}
}

// compareStruct compares a file's root struct and what its pointers reach.
func compareStruct(t *testing.T, got, want *File, name string) {
	t.Helper()
	g, err := got.Locate(name)
	if err != nil {
		t.Fatal(err)
	}
	w, err := want.Locate(name)
	if err != nil {
		t.Fatal(err)
	}
	gs, ws := structView(g), structView(w)
	if gs != ws {
		t.Errorf("%s:\n%s\ncobra:\n%s", name, gs, ws)
	}
	if g.Content.Pools[g.Content.RootEntries[g.Root].Struct.Pool].NumFiles != w.Content.Pools[w.Content.RootEntries[w.Root].Struct.Pool].NumFiles {
		t.Errorf("%s: pool file counts differ", name)
	}
}

// structView prints a root struct's bytes up to the next struct, with each
// pointer replaced by what it points at: a string, or "end" for the end of
// its pool.
func structView(l Located) string {
	c := l.Content
	root := c.RootEntries[l.Root].Struct
	pool := c.PoolData[root.Pool]
	end := uint32(len(pool))
	for _, r := range c.RootEntries {
		if r.Struct.Pool == root.Pool && r.Struct.Offset > root.Offset {
			end = min(end, r.Struct.Offset)
		}
	}
	var b strings.Builder
	b.Write(bytes.TrimRight(pool[root.Offset:end], "\x00"))
	for _, fr := range c.Fragments {
		if fr.Link.Pool != root.Pool || fr.Link.Offset < root.Offset || fr.Link.Offset >= end {
			continue
		}
		target := fr.Struct
		if target.Offset == c.Pools[target.Pool].Size {
			b.WriteString(" ->end")
		} else {
			b.WriteString(" ->" + stringAt(c.PoolData[target.Pool], target.Offset))
		}
	}
	return b.String()
}
