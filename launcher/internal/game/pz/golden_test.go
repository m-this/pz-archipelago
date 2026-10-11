package pz

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

type goldenFile struct {
	Variant54 []string    `json:"variant54"`
	Variant32 []string    `json:"variant32"`
	LoadBool  []string    `json:"loadbool"`
	SetHash   [][3]uint64 `json:"set_hash"`
	Int32Set  []struct {
		Keys []uint32 `json:"keys"`
		Cap  uint32   `json:"cap"`
		Blob string   `json:"blob"`
	} `json:"int32_set"`
	AnimalHash [][3]json.RawMessage `json:"animal_hash"`
}

func golden(t *testing.T) goldenFile {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "testdata", "python-golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var g goldenFile
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatal(err)
	}
	return g
}

func TestBytecodeVariantsMatchPython(t *testing.T) {
	g := golden(t)
	for n := range 5 {
		if got := hex.EncodeToString(variant54(n)); got != g.Variant54[n] {
			t.Errorf("variant54(%d) %s, want %s", n, got, g.Variant54[n])
		}
		if got := hex.EncodeToString(variant32(n)); got != g.Variant32[n] {
			t.Errorf("variant32(%d) %s, want %s", n, got, g.Variant32[n])
		}
	}
	i := 0
	for _, a := range []uint32{4, 5, 200} {
		for _, v := range []bool{false, true} {
			if got := hex.EncodeToString(loadBool(a, v)); got != g.LoadBool[i] {
				t.Errorf("loadBool(%d, %v) %s, want %s", a, v, got, g.LoadBool[i])
			}
			i++
		}
	}
}

func TestMarketSetMatchesPython(t *testing.T) {
	g := golden(t)
	for _, c := range g.SetHash {
		if got := setHash(uint32(c[0]), uint32(c[1])); uint64(got) != c[2] {
			t.Errorf("setHash(%d, %d) %d, want %d", c[0], c[1], got, c[2])
		}
	}
	for _, c := range g.Int32Set {
		slots, blob := buildSet(c.Keys)
		if slots != c.Cap || hex.EncodeToString(blob) != c.Blob {
			t.Errorf("buildSet(%v): slots %d, want %d", c.Keys, slots, c.Cap)
		}
	}
}

func TestAnimalHashMatchesPython(t *testing.T) {
	for _, c := range golden(t).AnimalHash {
		var keyText string
		var slots, want uint64
		if json.Unmarshal(c[0], &keyText) != nil || json.Unmarshal(c[1], &slots) != nil || json.Unmarshal(c[2], &want) != nil {
			t.Fatal("bad golden entry")
		}
		key, err := strconv.ParseUint(keyText, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		if got := animalHash(key, slots); got != want {
			t.Errorf("animalHash(%d, %d) %d, want %d", key, slots, got, want)
		}
	}
}
