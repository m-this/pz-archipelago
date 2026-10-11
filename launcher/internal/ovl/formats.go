package ovl

import (
	"encoding/binary"
	"fmt"
)

// An FDB is a SQLite database. Its root struct is the database's size
// followed by 28 zero bytes; buffer 0 holds its basename and buffer 1 the
// database itself.
const fdbRootSize = 32

// FDB returns a database file's bytes.
func (f *File) FDB(name string) ([]byte, error) {
	l, err := f.Locate(name)
	if err != nil {
		return nil, err
	}
	buffers, err := l.Buffers()
	if err != nil {
		return nil, err
	}
	if len(buffers) != 2 {
		return nil, fmt.Errorf("%s: %d buffers, an fdb has 2", name, len(buffers))
	}
	return buffers[1], nil
}

// ReplaceFDB swaps a database for another, as cobra-tools' inject does.
func (f *File) ReplaceFDB(name string, db []byte) error {
	l, err := f.Locate(name)
	if err != nil {
		return err
	}
	root, err := l.RootBytes()
	if err != nil {
		return err
	}
	buffers, err := l.Buffers()
	if err != nil {
		return err
	}
	if len(root) < fdbRootSize || len(buffers) != 2 {
		return fmt.Errorf("%s: not shaped like an fdb", name)
	}
	if err := l.ReplaceBuffers([][]byte{buffers[0], db}); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	clear(root[:fdbRootSize])
	binary.LittleEndian.PutUint32(root, uint32(len(db)))
	return nil
}

// A Lua module's root struct starts with the source size; the two pointers
// after it name the module and are left alone. cobra-tools writes the
// other header words as zero when it injects a module, and the game takes it.
const (
	luaRootSize    = 48
	luaHeaderWords = 16
)

// ReplaceLua swaps a Lua module's source, plain text or compiled.
func (f *File) ReplaceLua(name string, source []byte) error {
	l, err := f.Locate(name)
	if err != nil {
		return err
	}
	root, err := l.RootBytes()
	if err != nil {
		return err
	}
	if len(root) < luaRootSize {
		return fmt.Errorf("%s: not shaped like a lua module", name)
	}
	if err := l.ReplaceBuffers([][]byte{source}); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	clear(root[:luaHeaderWords])
	binary.LittleEndian.PutUint32(root, uint32(len(source)))
	return nil
}
