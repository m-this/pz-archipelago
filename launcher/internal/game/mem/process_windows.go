//go:build windows

package mem

import (
	"errors"
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32         = windows.NewLazySystemDLL("kernel32.dll")
	ntdll            = windows.NewLazySystemDLL("ntdll.dll")
	procAllocEx      = kernel32.NewProc("VirtualAllocEx")
	procFreeEx       = kernel32.NewProc("VirtualFreeEx")
	procSuspend      = ntdll.NewProc("NtSuspendProcess")
	procResume       = ntdll.NewProc("NtResumeProcess")
	errNotRunning    = errors.New("the game is not running")
	errNoTrampoline  = errors.New("no free memory within 2 GiB of the hook site")
	allocGranularity = uint64(0x10000)
)

const (
	memCommit     = 0x1000
	memReserve    = 0x2000
	memRelease    = 0x8000
	memFree       = 0x10000
	pageGuard     = 0x100
	pageRW        = 0x04
	pageRWX       = 0x40
	rel32Reach    = 0x7FFF0000
	processAccess = windows.PROCESS_VM_READ | windows.PROCESS_VM_WRITE | windows.PROCESS_VM_OPERATION |
		windows.PROCESS_QUERY_INFORMATION | 0x0800 // PROCESS_SUSPEND_RESUME
)

// ErrNotRunning means no process of that name exists.
var ErrNotRunning = errNotRunning

// Windows is an attached process.
type Windows struct {
	handle     windows.Handle
	moduleBase uint64
	moduleSize uint64
}

// Open attaches to the first process whose executable is named exe.
func Open(exe string) (Process, error) {
	pid, err := findProcess(exe)
	if err != nil {
		return nil, err
	}
	h, err := windows.OpenProcess(processAccess, false, pid)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", exe, err)
	}
	base, size, err := mainModule(pid, exe)
	if err != nil {
		_ = windows.CloseHandle(h)
		return nil, err
	}
	return &Windows{handle: h, moduleBase: base, moduleSize: size}, nil
}

func findProcess(exe string) (uint32, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, err
	}
	defer func() { _ = windows.CloseHandle(snap) }()
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if strings.EqualFold(windows.UTF16ToString(e.ExeFile[:]), exe) {
			return e.ProcessID, nil
		}
	}
	return 0, errNotRunning
}

func mainModule(pid uint32, exe string) (uint64, uint64, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPMODULE|windows.TH32CS_SNAPMODULE32, pid)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = windows.CloseHandle(snap) }()
	var m windows.ModuleEntry32
	m.Size = uint32(windows.SizeofModuleEntry32)
	for err = windows.Module32First(snap, &m); err == nil; err = windows.Module32Next(snap, &m) {
		if strings.EqualFold(windows.UTF16ToString(m.Module[:]), exe) {
			return uint64(m.ModBaseAddr), uint64(m.ModBaseSize), nil
		}
	}
	return 0, 0, fmt.Errorf("%s has no module named %s", exe, exe)
}

func (w *Windows) Read(addr uint64, n int) ([]byte, error) {
	if n <= 0 {
		return nil, nil
	}
	buf := make([]byte, n)
	var got uintptr
	if err := windows.ReadProcessMemory(w.handle, uintptr(addr), &buf[0], uintptr(n), &got); err != nil {
		return nil, fmt.Errorf("read 0x%X+%d: %w", addr, n, err)
	}
	if int(got) != n {
		return nil, fmt.Errorf("read 0x%X+%d: %w", addr, n, ErrUnmapped)
	}
	return buf, nil
}

// Write falls back to lifting the page protection, which a code patch in
// the executable needs.
func (w *Windows) Write(addr uint64, b []byte) error {
	if len(b) == 0 {
		return nil
	}
	var put uintptr
	err := windows.WriteProcessMemory(w.handle, uintptr(addr), &b[0], uintptr(len(b)), &put)
	if err == nil && int(put) == len(b) {
		return nil
	}
	var old uint32
	if perr := windows.VirtualProtectEx(w.handle, uintptr(addr), uintptr(len(b)), pageRWX, &old); perr != nil {
		return fmt.Errorf("write 0x%X+%d: %w", addr, len(b), err)
	}
	err = windows.WriteProcessMemory(w.handle, uintptr(addr), &b[0], uintptr(len(b)), &put)
	_ = windows.VirtualProtectEx(w.handle, uintptr(addr), uintptr(len(b)), old, &old)
	if err != nil {
		return fmt.Errorf("write 0x%X+%d: %w", addr, len(b), err)
	}
	return nil
}

func (w *Windows) Module() (uint64, uint64) { return w.moduleBase, w.moduleSize }

func (w *Windows) query(addr uint64) (windows.MemoryBasicInformation, bool) {
	var mbi windows.MemoryBasicInformation
	err := windows.VirtualQueryEx(w.handle, uintptr(addr), &mbi, unsafe.Sizeof(mbi))
	return mbi, err == nil
}

// Writable walks the address space once; every step moves past one region.
func (w *Windows) Writable() ([]Region, error) {
	var out []Region
	for addr := uint64(0); addr < 0x7FFFFFFFFFFF; {
		mbi, ok := w.query(addr)
		if !ok {
			break
		}
		base, size := uint64(mbi.BaseAddress), uint64(mbi.RegionSize)
		switch mbi.Protect & 0xFF {
		case 0x04, 0x08, 0x40, 0x80:
			if mbi.State == memCommit && mbi.Protect&pageGuard == 0 && size > 0 {
				out = append(out, Region{Base: base, Size: size})
			}
		}
		if base+size <= addr {
			break
		}
		addr = base + size
	}
	return out, nil
}

func (w *Windows) allocAt(addr, size uint64, protect uint32) uint64 {
	r, _, _ := procAllocEx.Call(uintptr(w.handle), uintptr(addr), uintptr(size), memCommit|memReserve, uintptr(protect))
	return uint64(r)
}

// AllocNear walks the free regions within reach of target, lowest first.
func (w *Windows) AllocNear(target, size uint64) (uint64, error) {
	lo := uint64(0x10000)
	if target > rel32Reach+lo {
		lo = target - rel32Reach
	}
	hi := target + rel32Reach
	for addr := lo; addr < hi; {
		mbi, ok := w.query(addr)
		if !ok {
			break
		}
		base, region := uint64(mbi.BaseAddress), uint64(mbi.RegionSize)
		if mbi.State == memFree {
			start := (base + allocGranularity - 1) &^ (allocGranularity - 1)
			if start+size <= base+region && start+size < hi {
				if got := w.allocAt(start, size, pageRWX); got != 0 {
					return got, nil
				}
			}
		}
		if base+region <= addr {
			break
		}
		addr = base + region
	}
	return 0, errNoTrampoline
}

func (w *Windows) Alloc(size uint64) (uint64, error) {
	if got := w.allocAt(0, size, pageRW); got != 0 {
		return got, nil
	}
	return 0, errors.New("VirtualAllocEx failed")
}

func (w *Windows) Free(addr uint64) error {
	r, _, err := procFreeEx.Call(uintptr(w.handle), uintptr(addr), 0, memRelease)
	if r == 0 {
		return fmt.Errorf("VirtualFreeEx 0x%X: %w", addr, err)
	}
	return nil
}

func (w *Windows) Suspend() error {
	if r, _, _ := procSuspend.Call(uintptr(w.handle)); r != 0 {
		return fmt.Errorf("NtSuspendProcess: status 0x%X", r)
	}
	return nil
}

func (w *Windows) Resume() error {
	if r, _, _ := procResume.Call(uintptr(w.handle)); r != 0 {
		return fmt.Errorf("NtResumeProcess: status 0x%X", r)
	}
	return nil
}

func (w *Windows) Close() error { return windows.CloseHandle(w.handle) }
