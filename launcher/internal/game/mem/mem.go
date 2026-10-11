// Package mem reads and writes another process's memory. It knows nothing of
// Planet Zoo; Process is the seam the game code is written against, so it
// runs the same on the real game and on the fake the tests build.
package mem

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
)

// Region is a committed span of the target's address space.
type Region struct {
	Base, Size uint64
}

// Process is an attached game process.
type Process interface {
	Read(addr uint64, n int) ([]byte, error)
	Write(addr uint64, b []byte) error
	// Module is the main executable's base and size.
	Module() (base, size uint64)
	// Writable lists committed, writable, non-guard regions: the heap.
	Writable() ([]Region, error)
	// AllocNear reserves read-write-execute memory within ±2 GiB of target,
	// for a trampoline a rel32 jump can reach. Alloc reserves read-write
	// memory anywhere. Free releases either.
	AllocNear(target, size uint64) (uint64, error)
	Alloc(size uint64) (uint64, error)
	Free(addr uint64) error
	// Suspend stops every thread of the target until Resume, so a code patch
	// is never half-written while the game runs it.
	Suspend() error
	Resume() error
	Close() error
}

// ErrUnmapped is a read or write outside any mapped memory.
var ErrUnmapped = errors.New("address not mapped")

func U8(p Process, addr uint64) (uint8, error) {
	b, err := p.Read(addr, 1)
	if err != nil {
		return 0, err
	}
	return b[0], nil
}

func U32(p Process, addr uint64) (uint32, error) {
	b, err := p.Read(addr, 4)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(b), nil
}

func I32(p Process, addr uint64) (int32, error) {
	v, err := U32(p, addr)
	return int32(v), err
}

func U64(p Process, addr uint64) (uint64, error) {
	b, err := p.Read(addr, 8)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(b), nil
}

func I64(p Process, addr uint64) (int64, error) {
	v, err := U64(p, addr)
	return int64(v), err
}

func F32(p Process, addr uint64) (float32, error) {
	v, err := U32(p, addr)
	return math.Float32frombits(v), err
}

func F64(p Process, addr uint64) (float64, error) {
	v, err := U64(p, addr)
	return math.Float64frombits(v), err
}

// Ptr reads a pointer, treating an unreadable one like a null one: a chain
// that breaks means the object is not there yet.
func Ptr(p Process, addr uint64) uint64 {
	v, err := U64(p, addr)
	if err != nil {
		return 0
	}
	return v
}

func PutU8(p Process, addr uint64, v uint8) error { return p.Write(addr, []byte{v}) }

func PutU32(p Process, addr uint64, v uint32) error {
	return p.Write(addr, binary.LittleEndian.AppendUint32(nil, v))
}

func PutI32(p Process, addr uint64, v int32) error { return PutU32(p, addr, uint32(v)) }

func PutU64(p Process, addr uint64, v uint64) error {
	return p.Write(addr, binary.LittleEndian.AppendUint64(nil, v))
}

func PutI64(p Process, addr uint64, v int64) error { return PutU64(p, addr, uint64(v)) }

func PutF32(p Process, addr uint64, v float32) error { return PutU32(p, addr, math.Float32bits(v)) }

// Chain walks a pointer chain the way Cheat Engine writes one: dereference at
// every offset but the last, which is added. A null or unreadable step means
// the chain is not live, and Chain returns 0.
func Chain(p Process, base uint64, offsets ...uint64) uint64 {
	if len(offsets) == 0 {
		return base
	}
	addr := base
	for _, off := range offsets[:len(offsets)-1] {
		addr = Ptr(p, addr+off)
		if addr == 0 {
			return 0
		}
	}
	return addr + offsets[len(offsets)-1]
}

// Deref walks a chain dereferencing at every offset, the last included.
func Deref(p Process, base uint64, offsets ...uint64) uint64 {
	addr := base
	for _, off := range offsets {
		addr = Ptr(p, addr+off)
		if addr == 0 {
			return 0
		}
	}
	return addr
}

// Bounds on a heap scan: one read never exceeds a chunk, and a scan for a
// value that is never found stops after the budget.
const (
	scanChunk  = 16 << 20
	scanBudget = 4 << 30
)

// FindQword returns up to limit 8-aligned addresses in the heap whose qword
// equals value. It is how an object with no stable pointer chain is found by
// its vtable.
func FindQword(p Process, value uint64, limit int) ([]uint64, error) {
	regions, err := p.Writable()
	if err != nil {
		return nil, err
	}
	needle := binary.LittleEndian.AppendUint64(nil, value)
	var out []uint64
	var scanned uint64
	for _, r := range regions {
		if len(out) >= limit || scanned >= scanBudget {
			break
		}
		scanned += r.Size
		for off := uint64(0); off < r.Size && len(out) < limit; off += scanChunk {
			n := min(scanChunk, r.Size-off)
			blob, err := p.Read(r.Base+off, int(n))
			if err != nil {
				break
			}
			for i := bytes.Index(blob, needle); i >= 0 && len(out) < limit; {
				if (r.Base+off+uint64(i))%8 == 0 {
					out = append(out, r.Base+off+uint64(i))
				}
				next := bytes.Index(blob[i+1:], needle)
				if next < 0 {
					break
				}
				i += 1 + next
			}
		}
	}
	return out, nil
}

// FindBytes returns every address in the heap where pattern starts. Loaded
// Lua code arrays are found this way. Regions of 256 MiB or more are skipped,
// as upstream did.
func FindBytes(p Process, pattern []byte) ([]uint64, error) {
	regions, err := p.Writable()
	if err != nil {
		return nil, err
	}
	var out []uint64
	for _, r := range regions {
		if r.Size >= 0x10000000 {
			continue
		}
		blob, err := p.Read(r.Base, int(r.Size))
		if err != nil {
			continue
		}
		for i := 0; ; {
			j := bytes.Index(blob[i:], pattern)
			if j < 0 {
				break
			}
			out = append(out, r.Base+uint64(i+j))
			i += j + 1
		}
	}
	return out, nil
}
