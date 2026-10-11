// Package gamedata is the single source of truth for every Planet Zoo fact
// this project knows and for every Archipelago id. The apworld reads what
// Export writes, and the client reads the tables directly.
package gamedata

// Game is the Archipelago game name. The apworld, the room and the client
// must agree on it byte for byte.
const Game = "Planet Zoo"

type (
	ItemID     int64
	LocationID int64
	SpeciesKey string
)

type Classification uint8

const (
	Progression Classification = iota + 1
	Useful
	Filler
)
