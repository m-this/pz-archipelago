package apclient

import (
	"encoding/json"
	"strconv"
	"strings"
)

// The wire format: a JSON array of commands. Only the ones this client acts
// on are modelled. See docs/network protocol.md in the Archipelago tree.

// itemsHandling asks for items from other worlds, our own, and the starting
// inventory.
const itemsHandling = 0b111

const statusGoal = 30

type header struct {
	Cmd string `json:"cmd"`
}

// version is the Archipelago release this client speaks to; the "class" key
// is how the server reads it.
type version struct {
	Class string `json:"class"`
	Major int    `json:"major"`
	Minor int    `json:"minor"`
	Build int    `json:"build"`
}

var clientVersion = version{Class: "Version", Major: 0, Minor: 6, Build: 8}

type connectMessage struct {
	Cmd           string   `json:"cmd"`
	Password      string   `json:"password"`
	Game          string   `json:"game"`
	Name          string   `json:"name"`
	UUID          string   `json:"uuid"`
	Version       version  `json:"version"`
	ItemsHandling int      `json:"items_handling"`
	Tags          []string `json:"tags"`
	SlotData      bool     `json:"slot_data"`
}

type locationChecks struct {
	Cmd       string  `json:"cmd"`
	Locations []int64 `json:"locations"`
}

type statusUpdate struct {
	Cmd    string `json:"cmd"`
	Status int    `json:"status"`
}

type plainCommand struct {
	Cmd string `json:"cmd"`
}

type getDataPackage struct {
	Cmd   string   `json:"cmd"`
	Games []string `json:"games"`
}

type roomInfo struct {
	SeedName string `json:"seed_name"`
}

type connected struct {
	Slot             int             `json:"slot"`
	CheckedLocations []int64         `json:"checked_locations"`
	SlotData         json.RawMessage `json:"slot_data"`
	Players          []struct {
		Slot  int    `json:"slot"`
		Alias string `json:"alias"`
		Name  string `json:"name"`
	} `json:"players"`
	SlotInfo map[string]struct {
		Game string `json:"game"`
	} `json:"slot_info"`
}

type roomUpdate struct {
	CheckedLocations []int64 `json:"checked_locations"`
}

type connectionRefused struct {
	Errors []string `json:"errors"`
}

type receivedItems struct {
	Index int `json:"index"`
	Items []struct {
		Item int64 `json:"item"`
	} `json:"items"`
}

type dataPackage struct {
	Data struct {
		Games map[string]struct {
			ItemNameToID     map[string]int64 `json:"item_name_to_id"`
			LocationNameToID map[string]int64 `json:"location_name_to_id"`
		} `json:"games"`
	} `json:"data"`
}

type printJSON struct {
	Data []struct {
		Type   string `json:"type"`
		Text   string `json:"text"`
		Player int    `json:"player"`
	} `json:"data"`
}

// names turns the ids in a chat line back into names.
type names struct {
	players map[int]string
	games   map[int]string
	items   map[string]map[int64]string
	places  map[string]map[int64]string
}

func newNames() *names {
	return &names{
		players: map[int]string{}, games: map[int]string{},
		items: map[string]map[int64]string{}, places: map[string]map[int64]string{},
	}
}

func (n *names) lookup(from map[string]map[int64]string, id int64, slot int) string {
	if name := from[n.games[slot]][id]; name != "" {
		return name
	}
	return strconv.FormatInt(id, 10)
}

func (p printJSON) text(n *names) string {
	var b strings.Builder
	for _, part := range p.Data {
		id, err := strconv.ParseInt(part.Text, 10, 64)
		switch {
		case part.Type == "player_id" && err == nil:
			if name := n.players[int(id)]; name != "" {
				b.WriteString(name)
			} else {
				b.WriteString(part.Text)
			}
		case part.Type == "item_id" && err == nil:
			b.WriteString(n.lookup(n.items, id, part.Player))
		case part.Type == "location_id" && err == nil:
			b.WriteString(n.lookup(n.places, id, part.Player))
		default:
			b.WriteString(part.Text)
		}
	}
	return b.String()
}
