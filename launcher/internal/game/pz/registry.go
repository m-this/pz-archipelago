package pz

import (
	"strings"

	"github.com/m-this/pz-archipelago/launcher/internal/game/mem"
)

// The global string-symbol registry interns every content and species name.
// Ids are dense small ints in intern order and change between runs, so the
// client reads them live each session. Layout, from upstream's
// memory/registry.py and memory/rewards.py:
//
//	registry  = *(module + 0x298AE00)
//	+0x10 slot stride, +0x30 arena top (slots below it), +0x48 capacity,
//	+0x9C bucket count; entry = *(top - id*stride) & ~1
//	entry: +0x00 i32 refcount, +0x08 the name.
const (
	registryRVA     = 0x298AE00
	regStride       = 0x10
	regTop          = 0x30
	regCap          = 0x48
	regBuckets      = 0x9C
	regEntryRef     = 0x00
	regEntryName    = 0x08
	regMaxStride    = 0x100
	regMaxCap       = 1_000_000
	regMaxName      = 128
	regMaxIndexName = 96
)

// Registry reads the symbol registry.
type Registry struct {
	g     *Game
	base  uint64
	index map[string]uint32
}

type regHeader struct {
	top, stride uint64
	cap         uint32
}

func (r *Registry) header() (regHeader, bool) {
	if r.base == 0 {
		global := r.g.base + registryRVA
		for _, cand := range []uint64{mem.Ptr(r.g.proc, global), global} {
			if cand != 0 && r.plausible(cand) {
				r.base = cand
				break
			}
		}
		if r.base == 0 {
			return regHeader{}, false
		}
	}
	h := regHeader{top: mem.Ptr(r.g.proc, r.base+regTop), stride: mem.Ptr(r.g.proc, r.base+regStride)}
	c, err := mem.U32(r.g.proc, r.base+regCap)
	if err != nil || h.top == 0 || h.stride == 0 || h.stride > regMaxStride || c == 0 || c > regMaxCap {
		return regHeader{}, false
	}
	h.cap = c
	return h, true
}

func (r *Registry) plausible(base uint64) bool {
	stride := mem.Ptr(r.g.proc, base+regStride)
	top := mem.Ptr(r.g.proc, base+regTop)
	c, err := mem.I32(r.g.proc, base+regCap)
	return err == nil && stride > 0 && stride <= regMaxStride && top != 0 && c > 0 && c <= regMaxCap
}

func (r *Registry) entry(h regHeader, id uint32) uint64 {
	slot := mem.Ptr(r.g.proc, h.top-uint64(id)*h.stride)
	if slot == 0 || slot&1 == 1 {
		return 0
	}
	return slot
}

func asciiName(raw []byte, limit int) (string, bool) {
	end := 0
	for end < len(raw) && end < limit && raw[end] != 0 {
		end++
	}
	if end == 0 || end == len(raw) {
		return "", false
	}
	for _, c := range raw[:end] {
		if c < 0x20 || c >= 0x7F {
			return "", false
		}
	}
	return string(raw[:end]), true
}

// Name is an id's interned name, skipping freed entries.
func (r *Registry) Name(id uint32) (string, bool) {
	h, ok := r.header()
	if !ok || id == 0 {
		return "", false
	}
	e := r.entry(h, id)
	if e == 0 {
		return "", false
	}
	if ref, err := mem.I32(r.g.proc, e+regEntryRef); err != nil || ref == 0 {
		return "", false
	}
	raw, err := r.g.proc.Read(e+regEntryName, regMaxName)
	if err != nil {
		return "", false
	}
	return asciiName(raw, regMaxName)
}

// Lookup finds the id of a content name, case-insensitively, as the reward
// granter does. The index is built once and rebuilt by Refresh, because
// names intern lazily as research trees load.
func (r *Registry) Lookup(name string) (uint32, bool) {
	id, ok := r.Index()[strings.ToLower(name)]
	return id, ok
}

// Index is {lowercased name: first id} over the whole registry.
func (r *Registry) Index() map[string]uint32 {
	if r.index != nil {
		return r.index
	}
	h, ok := r.header()
	if !ok {
		return map[string]uint32{}
	}
	index := map[string]uint32{}
	for id := uint32(1); id < h.cap; id++ {
		e := r.entry(h, id)
		if !isHeap(e) {
			continue
		}
		raw, err := r.g.proc.Read(e+regEntryName, regMaxIndexName)
		if err != nil {
			continue
		}
		if name, ok := asciiName(raw, regMaxIndexName); ok {
			if _, seen := index[strings.ToLower(name)]; !seen {
				index[strings.ToLower(name)] = id
			}
		}
	}
	r.index = index
	return index
}

// Refresh drops the cached index; the next Lookup rebuilds it.
func (r *Registry) Refresh() { r.index = nil }

// Forget drops everything cached, for a new park.
func (r *Registry) Forget() {
	r.base = 0
	r.index = nil
}
