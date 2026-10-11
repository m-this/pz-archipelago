package mem

import (
	"fmt"
	"slices"
	"sync"
)

// Fake is an in-memory Process for tests: a set of mapped regions, a module,
// and a bump allocator. Tests lay out the structures a reader expects and
// check what it reads and writes.
// Its methods are safe to call from several goroutines: a test changes the
// game while the client loop reads it, as the real game does.
type Fake struct {
	mu         sync.Mutex
	regions    []fakeRegion
	moduleBase uint64
	moduleSize uint64
	next       uint64
	Suspended  int
}

type fakeRegion struct {
	base     uint64
	data     []byte
	writable bool
}

func NewFake(moduleBase, moduleSize uint64) *Fake {
	f := NewSparseFake(moduleBase, moduleSize)
	f.Map(moduleBase, int(moduleSize), false)
	return f
}

// NewSparseFake declares a module but maps none of it; MapPage maps the
// pages a test uses, so a large module costs no memory.
func NewSparseFake(moduleBase, moduleSize uint64) *Fake {
	return &Fake{moduleBase: moduleBase, moduleSize: moduleSize, next: 0x7FF000000000}
}

// MapPage maps the 4 KiB page holding addr, unless it already is.
func (f *Fake) MapPage(addr uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	page := addr &^ 0xFFF
	if _, err := f.find(page, 0x1000); err == nil {
		return
	}
	f.mapLocked(page, 0x1000, false)
}

// Map adds a zeroed region; writable regions are what heap scans see.
func (f *Fake) Map(base uint64, size int, writable bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mapLocked(base, size, writable)
}

func (f *Fake) mapLocked(base uint64, size int, writable bool) {
	f.regions = append(f.regions, fakeRegion{base: base, data: make([]byte, size), writable: writable})
	slices.SortFunc(f.regions, func(a, b fakeRegion) int {
		switch {
		case a.base < b.base:
			return -1
		case a.base > b.base:
			return 1
		}
		return 0
	})
}

func (f *Fake) find(addr uint64, n int) ([]byte, error) {
	for _, r := range f.regions {
		if addr >= r.base && addr+uint64(n) <= r.base+uint64(len(r.data)) {
			return r.data[addr-r.base : addr-r.base+uint64(n)], nil
		}
	}
	return nil, fmt.Errorf("0x%X+%d: %w", addr, n, ErrUnmapped)
}

func (f *Fake) Read(addr uint64, n int) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, err := f.find(addr, n)
	if err != nil {
		return nil, err
	}
	return slices.Clone(b), nil
}

func (f *Fake) Write(addr uint64, b []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	dst, err := f.find(addr, len(b))
	if err != nil {
		return err
	}
	copy(dst, b)
	return nil
}

func (f *Fake) Module() (uint64, uint64) { return f.moduleBase, f.moduleSize }

func (f *Fake) Writable() ([]Region, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Region
	for _, r := range f.regions {
		if r.writable {
			out = append(out, Region{Base: r.base, Size: uint64(len(r.data))})
		}
	}
	return out, nil
}

func (f *Fake) AllocNear(target, size uint64) (uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	base := (target + f.moduleSize + 0x100000) &^ 0xFFFF
	for _, r := range f.regions {
		if base < r.base+uint64(len(r.data)) && r.base < base+size {
			base = (r.base + uint64(len(r.data)) + 0xFFFF) &^ 0xFFFF
		}
	}
	f.mapLocked(base, int(size), true)
	return base, nil
}

func (f *Fake) Alloc(size uint64) (uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	base := f.next
	f.next += (size + 0xFFFF) &^ 0xFFFF
	f.mapLocked(base, int(size), true)
	return base, nil
}

func (f *Fake) Free(addr uint64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, r := range f.regions {
		if r.base == addr {
			f.regions = slices.Delete(f.regions, i, i+1)
			return nil
		}
	}
	return fmt.Errorf("free 0x%X: %w", addr, ErrUnmapped)
}

func (f *Fake) Suspend() error { f.mu.Lock(); f.Suspended++; f.mu.Unlock(); return nil }
func (f *Fake) Resume() error  { f.mu.Lock(); f.Suspended--; f.mu.Unlock(); return nil }
func (f *Fake) Close() error   { return nil }
