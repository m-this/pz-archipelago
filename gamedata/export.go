package gamedata

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// The exported JSON is what the Python apworld reads at import time, in the
// shape its decoders expect. It is committed so the apworld stands alone, and
// TestCommittedExportIsCurrent catches a stale copy.

const (
	FileItems     = "items.json"
	FileLocations = "locations.json"
	FileSpecies   = "specieses.json"
)

type itemJSON struct {
	Name             string `json:"name"`
	APClassification string `json:"ap_classification"`
	Version          string `json:"version"`
	Quantity         int8   `json:"quantity"`
	ID               ItemID `json:"id"`
}

// The apworld reads a species' domesticated flag as the string "true" or
// "false", and a location's as a boolean.
type speciesJSON struct {
	StringID      string `json:"stringid"`
	Label         string `json:"label"`
	Interactivity string `json:"interactivity"`
	WaterNeeded   bool   `json:"water_needed"`
	FenceGrade    int8   `json:"fence_grade"`
	Domesticated  string `json:"domesticated"`
}

type locationJSON struct {
	StringID     string     `json:"stringid"`
	Label        string     `json:"label"`
	Type         string     `json:"type"`
	SpeciesType  string     `json:"species_type"`
	WaterNeeded  bool       `json:"water_needed"`
	FenceGrade   int8       `json:"fence_grade"`
	Domesticated bool       `json:"domesticated"`
	ID           LocationID `json:"id"`
}

// Export writes the apworld's data files into dir.
func Export(dir string) error {
	files := map[string]any{
		FileItems:     exportItems(),
		FileLocations: exportLocations(),
		FileSpecies:   exportSpecies(),
	}
	for name, body := range files {
		encoded, err := encode(body)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), encoded, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func encode(body any) ([]byte, error) {
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(body); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func exportItems() []itemJSON {
	out := make([]itemJSON, 0, len(Items))
	for _, it := range Items {
		out = append(out, itemJSON{
			Name:             it.Name,
			APClassification: classificationName(it.Classification),
			Version:          "Base",
			Quantity:         it.Quantity,
			ID:               it.ID,
		})
	}
	return out
}

func exportSpecies() []speciesJSON {
	out := make([]speciesJSON, 0, len(AllSpecies))
	for _, s := range AllSpecies {
		interactivity := "full"
		if s.Interactivity == Exhibit {
			interactivity = "exhibit"
		}
		out = append(out, speciesJSON{
			StringID:      string(s.Key),
			Label:         s.Label,
			Interactivity: interactivity,
			WaterNeeded:   s.WaterNeeded,
			FenceGrade:    s.FenceGrade,
			Domesticated:  fmt.Sprint(s.Domesticated),
		})
	}
	return out
}

func exportLocations() []locationJSON {
	out := make([]locationJSON, 0, len(Locations))
	for _, l := range Locations {
		row := locationJSON{
			StringID:    l.StringID(),
			Label:       l.LocationName(),
			Type:        locationTypeName(l.Kind),
			SpeciesType: "none",
			ID:          l.ID,
		}
		if s, ok := SpeciesByKey(l.Species); ok {
			row.SpeciesType = string(s.Key)
			row.WaterNeeded = s.WaterNeeded
			row.FenceGrade = s.FenceGrade
			row.Domesticated = s.Domesticated
		}
		out = append(out, row)
	}
	return out
}

func classificationName(c Classification) string {
	switch c {
	case Progression:
		return "Progression"
	case Useful:
		return "Useful"
	case Filler:
		return "Filler"
	}
	panic(fmt.Sprintf("unknown classification %d", c))
}

func locationTypeName(k LocationKind) string {
	switch k {
	case Milestone:
		return "milestones"
	case MechanicResearch:
		return "mechanic"
	case FirstAcquisition, FirstBreeding:
		return "firsts"
	case ConservationRelease:
		return "conservation"
	case WelfareResearch:
		return "research welfare"
	}
	panic(fmt.Sprintf("unknown location kind %d", k))
}
