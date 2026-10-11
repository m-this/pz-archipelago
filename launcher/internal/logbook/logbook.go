// Package logbook is the launcher's log: the console, a file a player can
// send, and the last lines for the page.
package logbook

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"
)

const kept = 300

// Book keeps the last lines written.
type Book struct {
	mu    sync.Mutex
	lines []string
	part  []byte
}

func (b *Book) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.part = append(b.part, p...)
	for {
		i := bytes.IndexByte(b.part, '\n')
		if i < 0 {
			break
		}
		b.lines = append(b.lines, string(b.part[:i]))
		b.part = b.part[i+1:]
	}
	if len(b.lines) > kept {
		b.lines = b.lines[len(b.lines)-kept:]
	}
	return len(p), nil
}

// Lines is the last lines, oldest first.
func (b *Book) Lines() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.lines)
}

// Open starts a log writing to stderr, to dir/pzap.log (replaced each run)
// and to the returned book.
func Open(dir string) (*slog.Logger, *Book, io.Closer, error) {
	book := &Book{}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, nil, nil, err
	}
	file, err := os.Create(filepath.Join(dir, "pzap.log"))
	if err != nil {
		return nil, nil, nil, err
	}
	out := io.MultiWriter(os.Stderr, file, book)
	return slog.New(slog.NewTextHandler(out, nil)), book, file, nil
}
