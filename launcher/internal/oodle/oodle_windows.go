//go:build windows

package oodle

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	kraken = 8
	level  = 5
	// Input is compressed in blocks of this size, each on its own, and the
	// streams are concatenated.
	inputBlock = 32 * seekChunk
	seekChunk  = 256 * 1024
	// Scratch memory when Oodle's own bound is unusable.
	scratchFallback = 8 * 1024 * 1024
	scratchMax      = 2 * 1024 * 1024 * 1024
)

// Codec calls into a loaded Oodle DLL.
type Codec struct {
	dll                *windows.DLL
	compress           *windows.Proc
	decompress         *windows.Proc
	bufferSizeNeeded   *windows.Proc
	optionsDefault     *windows.Proc
	scratchMemoryBound *windows.Proc
}

// Load opens the DLL at path.
func Load(path string) (*Codec, error) {
	dll, err := windows.LoadDLL(path)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", path, err)
	}
	c := &Codec{dll: dll}
	for name, into := range map[string]**windows.Proc{
		"OodleLZ_Compress":                      &c.compress,
		"OodleLZ_Decompress":                    &c.decompress,
		"OodleLZ_GetCompressedBufferSizeNeeded": &c.bufferSizeNeeded,
		"OodleLZ_CompressOptions_GetDefault":    &c.optionsDefault,
		"OodleLZ_GetCompressScratchMemBound":    &c.scratchMemoryBound,
	} {
		proc, err := dll.FindProc(name)
		if err != nil {
			_ = dll.Release()
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		*into = proc
	}
	return c, nil
}

// Close unloads the DLL.
func (c *Codec) Close() error { return c.dll.Release() }

func ptr(b []byte) uintptr {
	if len(b) == 0 {
		return 0
	}
	return uintptr(unsafe.Pointer(&b[0]))
}

// Decompress inflates one archive whose size is known.
func (c *Codec) Decompress(compressed []byte, size int) ([]byte, error) {
	out := make([]byte, size)
	r, _, _ := c.decompress.Call(ptr(compressed), uintptr(len(compressed)), ptr(out), uintptr(size),
		0, 0, 0, 0, 0, 0, 0, 0, 0, 3)
	if int(r) != size {
		return nil, fmt.Errorf("oodle decompressed %d bytes, want %d", int(r), size)
	}
	return out, nil
}

// Compress deflates an archive block by block with Kraken.
func (c *Codec) Compress(raw []byte) ([]byte, error) {
	var out []byte
	for start := 0; start < len(raw); start += inputBlock {
		block, err := c.compressBlock(raw[start:min(start+inputBlock, len(raw))])
		if err != nil {
			return nil, err
		}
		out = append(out, block...)
	}
	return out, nil
}

// The options are copied out of Oodle's defaults before they are changed:
// the defaults are Oodle's own memory. Offsets follow its
// OodleLZ_CompressOptions; the buffer leaves room for fields cobra-tools
// does not declare.
const (
	optionsDeclared        = 76
	optionsBuffer          = 128
	optionSeekChunkReset   = 8
	optionSeekChunkLen     = 12
	optionLongRangeMatcher = 40
)

func (c *Codec) compressBlock(raw []byte) ([]byte, error) {
	size, _, _ := c.bufferSizeNeeded.Call(kraken, uintptr(len(raw)))
	out := make([]byte, int(size))
	options := c.options(len(raw))
	bound, _, _ := c.scratchMemoryBound.Call(kraken, level, seekChunk, ptr(options))
	scratchSize := int(int32(bound))
	if scratchSize < 0 || scratchSize > scratchMax {
		scratchSize = scratchFallback
	}
	scratch := make([]byte, scratchSize)
	n, _, _ := c.compress.Call(kraken, ptr(raw), uintptr(len(raw)), ptr(out), level, ptr(options), 0, 0,
		ptr(scratch), uintptr(scratchSize))
	if len(raw) > 0 && int(n) <= 0 {
		return nil, fmt.Errorf("oodle compressed %d bytes into nothing", len(raw))
	}
	return out[:int(n)], nil
}

func (c *Codec) options(rawLen int) []byte {
	defaults, _, _ := c.optionsDefault.Call(kraken, level)
	options := make([]byte, optionsBuffer)
	// The address comes back from the DLL as an integer; Oodle owns that memory.
	copy(options, unsafe.Slice((*byte)(unsafe.Add(nil, defaults)), optionsDeclared))
	reset, chunk := byte(1), uint32(seekChunk)
	if rawLen < seekChunk {
		reset, chunk = 0, 0
	}
	options[optionSeekChunkReset] = reset
	*(*uint32)(unsafe.Pointer(&options[optionSeekChunkLen])) = chunk
	options[optionLongRangeMatcher] = 0
	return options
}
