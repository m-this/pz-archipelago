package hook

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"

	"github.com/m-this/pz-archipelago/launcher/internal/game/mem"
)

// Trampolines are carved from 64 KiB blocks reserved near the module, one
// 4 KiB slot each: every hook site lies within the module, so one block
// within rel32 reach serves them all, and the game's address space near the
// module has few free holes.
const (
	arenaBlock = 0x10000
	arenaSlot  = 0x1000
	rel32Reach = 0x7FFF0000
	jmpLen     = 5
)

// ErrSiteChanged means the bytes at a hook site are not the ones expected:
// a game patch, a foreign hook, or one of ours left by a crash.
var ErrSiteChanged = errors.New("hook site bytes are not the expected original")

type block struct {
	base, used uint64
}

type installed struct {
	site     uint64
	original []byte
	region   uint64
}

// Manager installs and removes every detour in one process.
type Manager struct {
	proc   mem.Process
	blocks []block
	live   int
	hooks  map[string]installed
}

func NewManager(proc mem.Process) *Manager {
	return &Manager{proc: proc, hooks: map[string]installed{}}
}

func (m *Manager) slot(site uint64) (uint64, error) {
	for i, b := range m.blocks {
		at := b.base + b.used
		if b.used+arenaSlot <= arenaBlock && absDiff(at, site) < rel32Reach {
			m.blocks[i].used += arenaSlot
			m.live++
			return at, nil
		}
	}
	base, err := m.proc.AllocNear(site, arenaBlock)
	if err != nil {
		return 0, err
	}
	m.blocks = append(m.blocks, block{base: base, used: arenaSlot})
	m.live++
	return base, nil
}

func (m *Manager) release() {
	m.live--
	if m.live > 0 {
		return
	}
	for _, b := range m.blocks {
		_ = m.proc.Free(b.base)
	}
	m.blocks = nil
}

func absDiff(a, b uint64) uint64 {
	if a > b {
		return a - b
	}
	return b - a
}

// Install detours site to a trampoline. original must be what the site holds
// now; it is overwritten by a jmp and NOPs, with the process suspended.
func (m *Manager) Install(name string, site uint64, original []byte, build Builder) error {
	if len(original) < jmpLen {
		return fmt.Errorf("hook %s: %d original bytes, a jmp needs 5", name, len(original))
	}
	if _, ok := m.hooks[name]; ok {
		return nil
	}
	cur, err := m.proc.Read(site, len(original))
	if err != nil {
		return fmt.Errorf("hook %s: %w", name, err)
	}
	if !bytes.Equal(cur, original) {
		return fmt.Errorf("hook %s at 0x%X holds %x: %w", name, site, cur, ErrSiteChanged)
	}
	region, err := m.slot(site)
	if err != nil {
		return fmt.Errorf("hook %s: %w", name, err)
	}
	code := region + CodeOff
	if err := m.proc.Write(region, make([]byte, CodeOff)); err != nil {
		m.release()
		return err
	}
	if err := m.proc.Write(code, build(region, region, site+uint64(len(original)))); err != nil {
		m.release()
		return err
	}
	patch := slices.Concat([]byte{0xE9}, binary.LittleEndian.AppendUint32(nil, uint32(int32(int64(code)-int64(site+jmpLen)))),
		bytes.Repeat([]byte{0x90}, len(original)-jmpLen))
	if err := m.suspended(func() error { return m.proc.Write(site, patch) }); err != nil {
		m.release()
		return fmt.Errorf("hook %s: %w", name, err)
	}
	m.hooks[name] = installed{site: site, original: original, region: region}
	return nil
}

func (m *Manager) suspended(write func() error) error {
	if err := m.proc.Suspend(); err != nil {
		return err
	}
	err := write()
	if rerr := m.proc.Resume(); err == nil {
		err = rerr
	}
	return err
}

// Scratch is the scratch block of an installed hook.
func (m *Manager) Scratch(name string) (uint64, bool) {
	h, ok := m.hooks[name]
	return h.region, ok
}

// Active reports whether this manager placed the hook.
func (m *Manager) Active(name string) bool {
	_, ok := m.hooks[name]
	return ok
}

// Restore puts a site's original bytes back.
func (m *Manager) Restore(name string) error {
	h, ok := m.hooks[name]
	if !ok {
		return nil
	}
	delete(m.hooks, name)
	err := m.suspended(func() error { return m.proc.Write(h.site, h.original) })
	m.release()
	return err
}

// RestoreAll removes every hook, attempting each even when one fails.
func (m *Manager) RestoreAll() error {
	var errs []error
	for _, name := range slices.Sorted(func(yield func(string) bool) {
		for n := range m.hooks {
			if !yield(n) {
				return
			}
		}
	}) {
		errs = append(errs, m.Restore(name))
	}
	return errors.Join(errs...)
}
