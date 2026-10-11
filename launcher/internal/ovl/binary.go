package ovl

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// reader walks a byte slice; the first failure sticks and every later read
// is a no-op, so a parse checks err once at the end.
type reader struct {
	data []byte
	pos  int
	err  error
}

func (r *reader) read(into any) {
	if r.err != nil {
		return
	}
	size := binary.Size(into)
	if size < 0 || r.pos+size > len(r.data) {
		r.err = fmt.Errorf("truncated at byte %d: need %d more", r.pos, size)
		return
	}
	if _, err := binary.Decode(r.data[r.pos:r.pos+size], binary.LittleEndian, into); err != nil {
		r.err = err
		return
	}
	r.pos += size
}

func (r *reader) bytes(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || r.pos+n > len(r.data) {
		r.err = fmt.Errorf("truncated at byte %d: need %d more", r.pos, n)
		return nil
	}
	out := r.data[r.pos : r.pos+n : r.pos+n]
	r.pos += n
	return out
}

func readSlice[T any](r *reader, n int) []T {
	out := make([]T, n)
	if n > 0 {
		r.read(out)
	}
	return out
}

type writer struct {
	bytes.Buffer
}

func (w *writer) write(v any) {
	// binary.Write to a bytes.Buffer fails only on a type it cannot encode,
	// which is a bug here, not an input error.
	if err := binary.Write(&w.Buffer, binary.LittleEndian, v); err != nil {
		panic(err)
	}
}
