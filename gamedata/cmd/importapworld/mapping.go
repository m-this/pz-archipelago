package main

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	enrichmentPrefix = regexp.MustCompile(`^EN_`)
	lowerUpper       = regexp.MustCompile(`([a-z])([A-Z])`)
	letterDigit      = regexp.MustCompile(`([a-zA-Z])(\d)`)
	notAlphanumeric  = regexp.MustCompile(`[^a-z0-9]`)
)

// readable is the apworld's convert_readable, from generate_items.py: the
// display name it gives a research content token.
func readable(token string) string {
	name := enrichmentPrefix.ReplaceAllString(token, "Enrichment: ")
	name = strings.ReplaceAll(name, "_", " ")
	name = lowerUpper.ReplaceAllString(name, "$1 $2")
	name = letterDigit.ReplaceAllString(name, "$1 $2")
	name = strings.ReplaceAll(name, " L ", " Level ")
	return strings.TrimSpace(name)
}

// The engine token of a species is its label lowercased with everything but
// letters and digits dropped, except for these six, whose labels lost or
// changed a word the engine kept.
var engineTokenAlias = map[string]string{
	"aelephant":  "africanelephant",
	"acentipede": "amazongiantcentipede",
	"liguana":    "antilleaniguana",
	"btarantula": "brazilliansalmonpinktarantula",
	"lfrog":      "lehmannspoisonfrog",
	"blemur":     "blackwhiteruffedlemur",
}

func engineToken(key, label string) string {
	if alias, ok := engineTokenAlias[key]; ok {
		return alias
	}
	return notAlphanumeric.ReplaceAllString(strings.ToLower(label), "")
}

// The apworld renamed these for display; the engine kept the old names.
var contentTokenAlias = map[string]string{
	"East Asian Theme Sets Scenery":            "OrientThemeSetsScenery",
	"East Asian Theme Sets Blueprints Level 1": "OrientThemeSetsBlueprintsL1",
	"East Asian Theme Sets Blueprints Level 2": "OrientThemeSetsBlueprintsL2",
	"East Asian Theme Sets Blueprints Level 3": "OrientThemeSetsBlueprintsL3",
	"Souvenir Shops Just A Momento":            "SouvenirShopsJustAMomento", //nolint:misspell // the shop's name in the game
}

func contentToken(name string, index map[string]string) (string, error) {
	if alias, ok := contentTokenAlias[name]; ok {
		return alias, nil
	}
	if token, ok := index[name]; ok {
		return token, nil
	}
	return "", fmt.Errorf("no engine token for %q in the research catalog", name)
}

var fixedEffects = map[string]string{
	"Research Centre":      "gamedata.Effect{Kind: gamedata.FacilityUnlock, Facility: gamedata.ResearchCentre}",
	"Workshop":             "gamedata.Effect{Kind: gamedata.FacilityUnlock, Facility: gamedata.Workshop}",
	"Water Habitat Tools":  "gamedata.Effect{Kind: gamedata.WaterTools}",
	"Conservation Program": "gamedata.Effect{Kind: gamedata.ConservationProgram}",
	"Cash Inject Small":    "gamedata.Effect{Kind: gamedata.Cash, Size: gamedata.Small}",
	"Cash Inject Medium":   "gamedata.Effect{Kind: gamedata.Cash, Size: gamedata.Medium}",
	"Cash Inject Large":    "gamedata.Effect{Kind: gamedata.Cash, Size: gamedata.Large}",

	"Conservation Credits Small":  "gamedata.Effect{Kind: gamedata.ConservationCredits, Size: gamedata.Small}",
	"Conservation Credits Medium": "gamedata.Effect{Kind: gamedata.ConservationCredits, Size: gamedata.Medium}",
	"Conservation Credits Large":  "gamedata.Effect{Kind: gamedata.ConservationCredits, Size: gamedata.Large}",

	"Progressive Supplement Level":         "gamedata.Effect{Kind: gamedata.Progressive, Family: gamedata.Supplement}",
	"Progressive Education Level":          "gamedata.Effect{Kind: gamedata.Progressive, Family: gamedata.Education}",
	"Progressive Breeding Level":           "gamedata.Effect{Kind: gamedata.Progressive, Family: gamedata.Breeding}",
	"Progressive Exhibit Enrichment Level": "gamedata.Effect{Kind: gamedata.Progressive, Family: gamedata.ExhibitEnrichment}",
	"Progressive Barrier Level":            "gamedata.Effect{Kind: gamedata.Progressive, Family: gamedata.Barrier}",
}

func effect(name string, speciesByLabel map[string]string, tokens map[string]string) (string, error) {
	if label, ok := strings.CutPrefix(name, "Permit: "); ok {
		key, known := speciesByLabel[label]
		if !known {
			return "", fmt.Errorf("permit %q: no species labelled %q", name, label)
		}
		return fmt.Sprintf("gamedata.Effect{Kind: gamedata.Permit, Species: %q}", key), nil
	}
	if fixed, ok := fixedEffects[name]; ok {
		return fixed, nil
	}
	token, err := contentToken(name, tokens)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("gamedata.Effect{Kind: gamedata.ResearchReward, Content: %q}", token), nil
}

func classification(apClass string) (string, error) {
	switch apClass {
	case "Progression", "Useful", "Filler":
		return "gamedata." + apClass, nil
	}
	return "", fmt.Errorf("unknown classification %q", apClass)
}
