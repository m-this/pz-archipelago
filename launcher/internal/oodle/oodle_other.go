//go:build !windows

package oodle

import "errors"

// Codec is only available on Windows, where the game's DLL runs.
type Codec struct{}

func Load(string) (*Codec, error) {
	return nil, errors.New("oodle needs Windows: the game's DLL is a Windows library")
}

func (*Codec) Close() error                           { return nil }
func (*Codec) Decompress([]byte, int) ([]byte, error) { return nil, errors.ErrUnsupported }
func (*Codec) Compress([]byte) ([]byte, error)        { return nil, errors.ErrUnsupported }
