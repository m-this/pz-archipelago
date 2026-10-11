package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type upstreamSpecies struct {
	StringID      string `json:"stringid"`
	Label         string `json:"label"`
	Interactivity string `json:"interactivity"`
	WaterNeeded   bool   `json:"water_needed"`
	FenceGrade    int8   `json:"fence_grade"`
	Domesticated  string `json:"domesticated"`
}

type upstreamItem struct {
	Name             string `json:"name"`
	APClassification string `json:"ap_classification"`
	Version          string `json:"version"`
	Quantity         int8   `json:"quantity"`
	ID               int64  `json:"id"`
}

type upstreamLocation struct {
	StringID    string `json:"stringid"`
	Label       string `json:"label"`
	Type        string `json:"type"`
	SpeciesType string `json:"species_type"`
	// Copies of the species' own fields. The tables keep them once, on the
	// species, and the importer refuses a location that disagrees.
	WaterNeeded  bool  `json:"water_needed"`
	FenceGrade   int8  `json:"fence_grade"`
	Domesticated bool  `json:"domesticated"`
	ID           int64 `json:"id"`
}

type upstream struct {
	species   []upstreamSpecies
	items     []upstreamItem
	locations []upstreamLocation
	tokens    map[string]string
}

func readUpstream(world string) (upstream, error) {
	var up upstream
	data := filepath.Join(world, "data")
	for name, into := range map[string]any{
		"specieses.json": &up.species,
		"items.json":     &up.items,
		"locations.json": &up.locations,
	} {
		if err := readJSON(filepath.Join(data, name), into); err != nil {
			return up, err
		}
	}
	tokens, err := readTokenIndex(filepath.Join(data, "research_catalog.json"))
	up.tokens = tokens
	return up, err
}

func readJSON(path string, into any) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// readTokenIndex maps every research content's readable name to its engine
// token, by replaying the apworld's own readable() over the catalog. The
// catalog is read in file order: the first token to claim a name keeps it,
// as the Python setdefault did.
func readTokenIndex(path string) (map[string]string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var catalog struct {
		Species  json.RawMessage `json:"species"`
		Mechanic json.RawMessage `json:"mechanic"`
	}
	if err := json.Unmarshal(body, &catalog); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	index := map[string]string{}
	claim := func(token string) {
		if _, taken := index[readable(token)]; !taken {
			index[readable(token)] = token
		}
	}
	var species []struct {
		Rewards []string `json:"rewards"`
	}
	if err := eachValue(catalog.Species, &species, func() {
		for _, entry := range species {
			for _, reward := range entry.Rewards {
				claim(reward)
			}
		}
	}); err != nil {
		return nil, fmt.Errorf("%s: species: %w", path, err)
	}
	var mechanic []struct {
		Item string `json:"item"`
	}
	if err := eachValue(catalog.Mechanic, &mechanic, func() {
		for _, option := range mechanic {
			claim(option.Item)
		}
	}); err != nil {
		return nil, fmt.Errorf("%s: mechanic: %w", path, err)
	}
	return index, nil
}

// eachValue decodes the values of a JSON object into into, in file order,
// calling visit after each one.
func eachValue(object json.RawMessage, into any, visit func()) error {
	decoder := json.NewDecoder(bytes.NewReader(object))
	if _, err := decoder.Token(); err != nil {
		return err
	}
	for decoder.More() {
		if _, err := decoder.Token(); err != nil {
			return err
		}
		if err := decoder.Decode(into); err != nil {
			return err
		}
		visit()
	}
	return nil
}
