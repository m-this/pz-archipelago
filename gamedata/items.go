package gamedata

// EffectKind is what the client does to the game when the item arrives.
type EffectKind uint8

const (
	// Permit unlocks one species for purchase.
	Permit EffectKind = iota + 1
	FacilityUnlock
	WaterTools
	ConservationProgram
	Cash
	ConservationCredits
	// Progressive unlocks the next level of a research family.
	Progressive
	// ResearchReward unlocks one piece of research content by its engine token.
	ResearchReward
)

type Facility uint8

const (
	ResearchCentre Facility = iota + 1
	Workshop
)

type Family uint8

const (
	Supplement Family = iota + 1
	Education
	Breeding
	ExhibitEnrichment
	Barrier
)

// Size scales a cash or credit item. The amount is a room option, read from
// slot data: Medium is the option, Small half of it, Large double.
type Size uint8

const (
	Small Size = iota + 1
	Medium
	Large
)

// Effect holds the field its Kind names and leaves the others zero.
type Effect struct {
	Kind     EffectKind
	Species  SpeciesKey
	Facility Facility
	Family   Family
	Size     Size
	Content  string
}

type Item struct {
	ID             ItemID
	Name           string
	Classification Classification
	// Quantity is how many copies the pool holds: one per level for a
	// progressive item.
	Quantity int8
	Effect   Effect
}
