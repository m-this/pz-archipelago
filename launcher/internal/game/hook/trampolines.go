package hook

// Scratch layout of the permit detour: the exchange manager it saw, then the
// species handles the client blocks, then a log of the last purchase.
const (
	PermitBlockedMax     = 12
	PermitScratchCount   = 0x08
	PermitScratchIDs     = 0x10
	permitListingSpecies = 0x10
	permitListingID      = 0x228
	permitLogHandle      = 0x100
	permitLogID          = 0x104
	permitLogCount       = 0x108
)

// Permit aborts an Animal-Exchange purchase whose species handle is in the
// blocked set, by jumping to fail; otherwise it runs orig and resumes. At the
// site rbx is the listing and r15 the exchange manager.
func Permit(region, scratch, resume, fail uint64, orig []byte) []byte {
	var a asm
	a.b(saveFlagsRaxR10R11...)
	a.movabsRAX(scratch)
	a.b(0x4C, 0x89, 0x38) // mov [rax], r15
	a.b(0x44, 0x8B, 0x53, permitListingSpecies)
	a.b(0x44, 0x89, 0x90)
	a.i32(permitLogHandle)
	a.b(0x44, 0x8B, 0x9B)
	a.i32(permitListingID)
	a.b(0x44, 0x89, 0x98)
	a.i32(permitLogID)
	a.b(0xFF, 0x80)
	a.i32(permitLogCount)
	a.b(0x44, 0x8B, 0x50, PermitScratchCount)
	a.b(0x45, 0x85, 0xD2) // test r10d, r10d
	allow := a.short(0x74)
	a.b(0x4C, 0x8D, 0x58, PermitScratchIDs)
	a.b(0x8B, 0x43, permitListingSpecies)
	loop := len(a)
	a.b(0x41, 0x3B, 0x03) // cmp eax, [r11]
	block := a.short(0x74)
	a.b(0x49, 0x83, 0xC3, 0x04) // add r11, 4
	a.b(0x41, 0xFF, 0xCA)       // dec r10d
	again := a.short(0x75)
	a.landTo(again, loop)
	a.land(allow)
	a.b(restoreFlagsRaxR10R11...)
	a.b(orig...)
	toResume := a.jmp()
	a.land(block)
	a.b(restoreFlagsRaxR10R11...)
	toFail := a.jmp()
	a.patchJmp(toResume, region, resume)
	a.patchJmp(toFail, region, fail)
	return a
}

// Scratch of the release gate: the release count, then the lock flag the
// client writes (1 = the Conservation Program is not received yet), then the
// manager pointer seen at entry.
const (
	ReleaseScratchCount = 0x00
	ReleaseScratchLock  = 0x04
)

// ReleaseGate counts releases at a function entry, or aborts them with
// `xor eax, eax; ret` while locked.
func ReleaseGate(region, scratch, resume uint64, orig []byte) []byte {
	var a asm
	a.movabsRAX(scratch)
	a.b(0x83, 0x78, 0x04, 0x00) // cmp dword [rax+4], 0
	locked := a.short(0x75)
	a.b(0xFF, 0x00)             // inc dword [rax]
	a.b(0x48, 0x89, 0x48, 0x10) // mov [rax+0x10], rcx
	a.b(orig...)
	toResume := a.jmp()
	a.land(locked)
	a.b(0x31, 0xC0, 0xC3)
	a.patchJmp(toResume, region, resume)
	return a
}

// Scratch of the release species capture.
const (
	ReleaseSpeciesHandle  = 0x08
	ReleaseSpeciesManager = 0x10
)

// ReleaseSpeciesCapture records the released animal's handle (rsi) and the
// manager the release resolves it through ([rbp+0x48]). orig is the two
// instructions `mov rcx,[rbp+0x48]` and `mov rdx,rsi`, run on either side.
func ReleaseSpeciesCapture(region, scratch, resume uint64, orig []byte) []byte {
	var a asm
	a.b(orig[:4]...)
	a.b(0x50)
	a.movabsRAX(scratch)
	a.b(0xFF, 0x00)
	a.b(0x48, 0x89, 0x70, 0x08)
	a.b(0x48, 0x89, 0x48, 0x10)
	a.b(0x58)
	a.b(orig[4:]...)
	return a.end(region, resume)
}

// Scratch of the research gate: the count of gated categories, then the
// category bytes. A research record's category sits at +0x3C.
const (
	ResearchGateCount = 0x00
	ResearchGateCats  = 0x08
	researchCategory  = 0x3C
)

// ResearchGate skips a research status write for a record whose category is
// gated. catModRM picks the record register: 0x4E for r14, 0x4F for r15.
func ResearchGate(region, scratch, resume uint64, orig []byte, catModRM byte) []byte {
	var a asm
	a.b(0x9C, 0x50, 0x51, 0x52)
	a.movabsRAX(scratch)
	a.b(0x41, 0x0F, 0xB6, catModRM, researchCategory)
	a.b(0x8B, 0x10) // mov edx, [rax]
	a.b(0x85, 0xD2) // test edx, edx
	do := a.short(0x74)
	a.b(0x48, 0x8D, 0x40, ResearchGateCats)
	loop := len(a)
	a.b(0x3A, 0x08) // cmp cl, [rax]
	skip := a.short(0x74)
	a.b(0x48, 0xFF, 0xC0) // inc rax
	a.b(0xFF, 0xCA)       // dec edx
	again := a.short(0x75)
	a.landTo(again, loop)
	a.land(do)
	a.b(0x5A, 0x59, 0x58, 0x9D)
	a.b(orig...)
	doJmp := a.jmp()
	a.land(skip)
	a.b(0x5A, 0x59, 0x58, 0x9D)
	skipJmp := a.jmp()
	a.patchJmp(doJmp, region, resume)
	a.patchJmp(skipJmp, region, resume)
	return a
}

// Scratch of the insert instrument: a count, then a ring of records.
const (
	InsertRing    = 8
	InsertRec     = 0x40
	InsertRingOff = 0x200
)

// InsertInstrument records, for each animal added to a habitat, its handle
// (rsi), its container (rbx), three frame slots and rbp, r13 and r14.
func InsertInstrument(region, scratch, resume uint64, orig []byte) []byte {
	var a asm
	a.b(0x9C, 0x50, 0x52, 0x41, 0x50)
	a.movabsRAX(scratch)
	a.b(0x8B, 0x10, 0xFF, 0xC2, 0x89, 0x10, 0xFF, 0xCA)
	a.b(0x83, 0xE2, InsertRing-1)
	a.b(0x69, 0xD2)
	a.u32(InsertRec)
	a.b(0x48, 0x05)
	a.u32(InsertRingOff)
	a.b(0x48, 0x8D, 0x04, 0x10)
	a.b(0x48, 0x89, 0x30)
	a.b(0x48, 0x89, 0x58, 0x08)
	a.b(0x4C, 0x8B, 0x85, 0xD8, 0x00, 0x00, 0x00)
	a.b(0x4C, 0x89, 0x40, 0x10)
	a.b(0x4C, 0x8B, 0x85, 0xE0, 0x00, 0x00, 0x00)
	a.b(0x4C, 0x89, 0x40, 0x18)
	a.b(0x4C, 0x8B, 0x85, 0xF8, 0x00, 0x00, 0x00)
	a.b(0x4C, 0x89, 0x40, 0x20)
	a.b(0x48, 0x89, 0x68, 0x28)
	a.b(0x4C, 0x89, 0x68, 0x30)
	a.b(0x4C, 0x89, 0x70, 0x38)
	a.b(0x41, 0x58, 0x5A, 0x58, 0x9D)
	a.b(orig...)
	return a.end(region, resume)
}

// Scratch of the exhibit instrument: a count, then a ring of the classifier
// (-1 for a birth) and the species name token.
const (
	ExhibitRing      = 8
	ExhibitRingOff   = 0x100
	ExhibitNameBytes = 0x28
	ExhibitRec       = 0x8 + ExhibitNameBytes
	exhibitClassOff  = 0xC
	// ExhibitBirth is the classifier of a freshly constructed animal.
	ExhibitBirth = 0xFFFFFFFF
)

// ExhibitInstrument records, at the exhibit-animal construct entry, the
// acquire-or-breed classifier [r8+0xc] and the species name at [[r8]].
func ExhibitInstrument(region, scratch, resume uint64, orig []byte) []byte {
	var a asm
	a.b(0x9C, 0x50, 0x51, 0x56, 0x57)
	a.movabsRAX(scratch)
	a.b(0x8B, 0x08, 0xFF, 0xC1, 0x89, 0x08, 0xFF, 0xC9)
	a.b(0x83, 0xE1, ExhibitRing-1)
	a.b(0x69, 0xC9)
	a.i32(ExhibitRec)
	a.b(0x48, 0x8D, 0xB8)
	a.i32(ExhibitRingOff)
	a.b(0x48, 0x01, 0xCF)
	a.b(0x41, 0x8B, 0x40, exhibitClassOff)
	a.b(0x89, 0x07)
	a.b(0x48, 0x83, 0xC7, 0x08)
	a.b(0x49, 0x8B, 0x30)
	a.b(0xB9)
	a.u32(ExhibitNameBytes / 8)
	a.b(0xFC, 0xF3, 0x48, 0xA5)
	a.b(0x5F, 0x5E, 0x59, 0x58, 0x9D)
	a.b(orig...)
	return a.end(region, resume)
}

// Scratch of the exhibit storage-release capture.
const (
	ExhibitReleaseRing    = 8
	ExhibitReleaseMgr     = 0x08
	ExhibitReleaseHolder  = 0x10
	ExhibitReleaseRingOff = 0x18
)

// ExhibitReleaseCapture runs orig (`mov r15,[r14+0xf8]`) first, then records
// the released animal id (ebx), the manager (r14) and the holder (r15).
func ExhibitReleaseCapture(region, scratch, resume uint64, orig []byte) []byte {
	var a asm
	a.b(orig...)
	a.b(0x9C, 0x50, 0x41, 0x53)
	a.movabsRAX(scratch)
	a.b(0x44, 0x8B, 0x18, 0x41, 0xFF, 0xC3, 0x44, 0x89, 0x18, 0x41, 0xFF, 0xCB)
	a.b(0x41, 0x83, 0xE3, ExhibitReleaseRing-1)
	a.b(0x42, 0x89, 0x5C, 0x98, ExhibitReleaseRingOff)
	a.b(0x4C, 0x89, 0x70, ExhibitReleaseMgr)
	a.b(0x4C, 0x89, 0x78, ExhibitReleaseHolder)
	a.b(0x41, 0x5B, 0x58, 0x9D)
	return a.end(region, resume)
}
