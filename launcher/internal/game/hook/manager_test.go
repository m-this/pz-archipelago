package hook

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/m-this/pz-archipelago/launcher/internal/game/mem"
)

const moduleBase = 0x140000000

func TestInstallAndRestore(t *testing.T) {
	f := mem.NewFake(moduleBase, 0x1000000)
	site := uint64(moduleBase + 0xC82168)
	orig := []byte{0x48, 0x8B, 0xBD, 0xD8, 0x00, 0x00, 0x00}
	if err := f.Write(site, orig); err != nil {
		t.Fatal(err)
	}
	m := NewManager(f)
	if err := m.Install("birth_insert", site, orig, func(r, sc, res uint64) []byte {
		return InsertInstrument(r, sc, res, orig)
	}); err != nil {
		t.Fatal(err)
	}
	patch, _ := f.Read(site, len(orig))
	region, _ := m.Scratch("birth_insert")
	target := int64(site) + 5 + int64(int32(binary.LittleEndian.Uint32(patch[1:])))
	if patch[0] != 0xE9 || uint64(target) != region+CodeOff || !bytes.Equal(patch[5:], []byte{0x90, 0x90}) {
		t.Fatalf("patch %x, region 0x%X", patch, region)
	}
	if f.Suspended != 0 {
		t.Error("left the process suspended")
	}
	code, _ := f.Read(region+CodeOff, 8)
	if code[0] != 0x9C {
		t.Errorf("trampoline not written: %x", code)
	}
	if err := m.Restore("birth_insert"); err != nil {
		t.Fatal(err)
	}
	if back, _ := f.Read(site, len(orig)); !bytes.Equal(back, orig) {
		t.Errorf("restored %x", back)
	}
	if _, err := f.Read(region, 1); err == nil {
		t.Error("the arena block outlived its last hook")
	}
}

func TestInstallRefusesChangedSite(t *testing.T) {
	f := mem.NewFake(moduleBase, 0x1000000)
	m := NewManager(f)
	err := m.Install("x", moduleBase+0x100, []byte{1, 2, 3, 4, 5}, func(r, sc, res uint64) []byte { return nil })
	if !errors.Is(err, ErrSiteChanged) {
		t.Errorf("err %v", err)
	}
}

func TestOneBlockServesManyHooks(t *testing.T) {
	f := mem.NewFake(moduleBase, 0x1000000)
	m := NewManager(f)
	orig := []byte{0x90, 0x90, 0x90, 0x90, 0x90}
	for i, name := range []string{"a", "b", "c"} {
		site := moduleBase + 0x1000*uint64(i+1)
		_ = f.Write(site, orig)
		if err := m.Install(name, site, orig, func(r, sc, res uint64) []byte { return []byte{0xC3} }); err != nil {
			t.Fatal(err)
		}
	}
	if len(m.blocks) != 1 || m.live != 3 {
		t.Errorf("%d blocks, %d live", len(m.blocks), m.live)
	}
	if err := m.RestoreAll(); err != nil || m.live != 0 || m.blocks != nil {
		t.Errorf("restore all: %v, %d live", err, m.live)
	}
}
