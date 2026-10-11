// Package hook places jump detours in the game's code and builds the x64
// trampolines they jump to. Each trampoline runs the instructions the detour
// replaced, does its own work against a scratch block the client polls, and
// jumps back. The byte sequences are tinkieZo's Planet-Zoo-AP
// pz_ap_client/memory/hook.py; the tests hold them to that file's output.
package hook

import "encoding/binary"

// A trampoline's slot holds its scratch block at +0 and its code at +0x40.
const CodeOff = 0x40

// Builder makes a trampoline for a slot whose code starts at region+CodeOff
// and whose scratch is at scratch, ending in a jump to resume.
type Builder func(region, scratch, resume uint64) []byte

type asm []byte

func (a *asm) b(bytes ...byte) { *a = append(*a, bytes...) }

func (a *asm) u32(v uint32) { *a = binary.LittleEndian.AppendUint32(*a, v) }

func (a *asm) i32(v int32) { a.u32(uint32(v)) }

func (a *asm) u64(v uint64) { *a = binary.LittleEndian.AppendUint64(*a, v) }

// movabs emits `movabs rax, imm64`.
func (a *asm) movabsRAX(v uint64) { a.b(0x48, 0xB8); a.u64(v) }

// short emits a two-byte branch with a placeholder rel8 and returns where.
func (a *asm) short(op byte) int { at := len(*a); a.b(op, 0); return at }

// land patches the rel8 of the branch at `at` to jump here.
func (a *asm) land(at int) { (*a)[at+1] = byte(len(*a) - (at + 2)) }

// landTo patches the rel8 of the branch at `at` to jump back to `to`.
func (a *asm) landTo(at, to int) { (*a)[at+1] = byte(to - (at + 2)) }

// jmp emits a rel32 jmp placeholder and returns where.
func (a *asm) jmp() int { at := len(*a); a.b(0xE9, 0, 0, 0, 0); return at }

// patchJmp points the rel32 jmp at `at` (in a trampoline at region+CodeOff)
// at the absolute target.
func (a *asm) patchJmp(at int, region, target uint64) {
	code := region + CodeOff
	rel := int64(target) - int64(code+uint64(at)+5)
	binary.LittleEndian.PutUint32((*a)[at+1:], uint32(int32(rel)))
}

// end appends the closing jmp back to resume.
func (a *asm) end(region, resume uint64) []byte {
	at := a.jmp()
	a.patchJmp(at, region, resume)
	return *a
}

// Register framing the permit-style detours: pushfq; push rax; push r10;
// push r11, and the matching pops.
var (
	saveFlagsRaxR10R11    = []byte{0x9C, 0x50, 0x41, 0x52, 0x41, 0x53}
	restoreFlagsRaxR10R11 = []byte{0x41, 0x5B, 0x41, 0x5A, 0x58, 0x9D}
)
