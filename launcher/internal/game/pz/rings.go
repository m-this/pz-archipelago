package pz

import (
	"encoding/binary"

	"github.com/m-this/pz-archipelago/launcher/internal/game/mem"
)

// drain reads the entries a detour appended to its ring since cursor: the
// scratch holds a u32 count, the n-th entry (1-based) sits in slot
// (n-1) mod ring. Entries overwritten before a poll are lost; for
// first-of-a-kind checks that loses nothing.
func drain(p mem.Process, scratch uint64, cursor uint32, ring uint32, read func(slot uint32) error) (uint32, error) {
	count, err := mem.U32(p, scratch)
	if err != nil || count <= cursor {
		return count, err
	}
	first := cursor + 1
	if count >= ring && count-ring+1 > first {
		first = count - ring + 1
	}
	for n := first; n <= count; n++ {
		if err := read((n - 1) & (ring - 1)); err != nil {
			return count, err
		}
	}
	return count, nil
}

func u64At(b []byte, off int) uint64 { return binary.LittleEndian.Uint64(b[off:]) }
