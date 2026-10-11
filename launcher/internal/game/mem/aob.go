package mem

import (
	"fmt"
	"strconv"
	"strings"
)

// Pattern is an IDA-style byte signature: hex bytes and ?? wildcards.
type Pattern struct {
	bytes []byte
	wild  []bool
}

func ParsePattern(sig string) (Pattern, error) {
	var p Pattern
	for tok := range strings.FieldsSeq(sig) {
		if tok == "??" || tok == "?" {
			p.bytes = append(p.bytes, 0)
			p.wild = append(p.wild, true)
			continue
		}
		b, err := strconv.ParseUint(tok, 16, 8)
		if err != nil || len(tok) != 2 {
			return Pattern{}, fmt.Errorf("bad signature token %q in %q", tok, sig)
		}
		p.bytes = append(p.bytes, byte(b))
		p.wild = append(p.wild, false)
	}
	if len(p.bytes) == 0 {
		return Pattern{}, fmt.Errorf("empty signature")
	}
	return p, nil
}

func MustPattern(sig string) Pattern {
	p, err := ParsePattern(sig)
	if err != nil {
		panic(err)
	}
	return p
}

func (p Pattern) matchAt(data []byte, i int) bool {
	for j, b := range p.bytes {
		if !p.wild[j] && data[i+j] != b {
			return false
		}
	}
	return true
}

// ScanModule finds up to limit matches of the pattern in the main module,
// reading it in overlapping chunks so no match straddles two reads.
func ScanModule(p Process, pat Pattern, limit int) []uint64 {
	base, size := p.Module()
	if base == 0 {
		return nil
	}
	if size == 0 {
		size = 0x20000000
	}
	const chunk = 0x800000
	var hits []uint64
	for addr := base; addr < base+size && len(hits) < limit; addr += chunk {
		n := min(chunk+uint64(len(pat.bytes)), base+size-addr)
		data, err := p.Read(addr, int(n))
		if err != nil {
			continue
		}
		for i := 0; i+len(pat.bytes) <= len(data) && len(hits) < limit; i++ {
			if uint64(i) >= chunk {
				break
			}
			if pat.matchAt(data, i) {
				hits = append(hits, addr+uint64(i))
			}
		}
	}
	return hits
}
