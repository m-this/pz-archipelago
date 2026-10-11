package pz

// Layout exposes the addresses a fake game process needs, for fakepz. It is
// the same table the readers use, so a fake cannot drift from them.
type Layout struct {
	Sites                          []Site
	RootCash, RootGuest            uint64
	ParkFirst, ParkOffset          uint64
	ParkInfoVtable, ParkInfoName   uint64
	ParkInfoYears                  uint64
	RegistryRVA                    uint64
	ResearchChain                  []uint64
	ResearchItemsMap, RecordStride uint64
	HabitatMarket, ExhibitMarket   uint64
	HabitatMode, ExhibitMode       uint64
	Cash, Credits, Rating, Guests  []uint64
}

func FakeLayout() Layout {
	return Layout{
		Sites: Sites, RootCash: rootCash, RootGuest: rootGuest, ParkFirst: parkFirst, ParkOffset: parkOffset,
		ParkInfoVtable: parkInfoVtableRVA, ParkInfoName: parkInfoName, ParkInfoYears: parkInfoYears,
		RegistryRVA: registryRVA, ResearchChain: researchChains[0],
		ResearchItemsMap: researchItemsMap, RecordStride: researchRecStride,
		HabitatMarket: habitatMarket.parkOff, ExhibitMarket: exhibitMarket.parkOff,
		HabitatMode: habitatMarket.mode, ExhibitMode: exhibitMarket.mode,
		Cash: Cash.offsets, Credits: ConservationCredits.offsets, Rating: ZooRating.offsets, Guests: GuestCount.offsets,
	}
}
