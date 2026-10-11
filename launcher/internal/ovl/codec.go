package ovl

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"fmt"
	"io"
)

// Codec compresses and decompresses archives. None and Zlib are built in;
// Oodle needs the game's own DLL and comes from the caller.
type Codec interface {
	Decompress(compressed []byte, size int) ([]byte, error)
	Compress(raw []byte) ([]byte, error)
}

type noneCodec struct{}

func (noneCodec) Decompress(compressed []byte, _ int) ([]byte, error) { return compressed, nil }
func (noneCodec) Compress(raw []byte) ([]byte, error)                 { return raw, nil }

// zlibCodec reads the deflate stream behind the two-byte zlib header and
// ignores the checksum, as cobra-tools does, and writes a full zlib stream.
type zlibCodec struct{}

func (zlibCodec) Decompress(compressed []byte, size int) ([]byte, error) {
	if len(compressed) < 2 {
		return nil, fmt.Errorf("zlib stream of %d bytes", len(compressed))
	}
	reader := flate.NewReader(bytes.NewReader(compressed[2:]))
	defer func() { _ = reader.Close() }()
	out := make([]byte, size)
	if _, err := io.ReadFull(reader, out); err != nil {
		return nil, fmt.Errorf("inflate: %w", err)
	}
	return out, nil
}

func (zlibCodec) Compress(raw []byte) ([]byte, error) {
	var out bytes.Buffer
	writer := zlib.NewWriter(&out)
	if _, err := writer.Write(raw); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// Codecs maps each compression to its codec. Oodle is absent unless the
// caller supplies one.
type Codecs map[Compression]Codec

func (c Codecs) get(compression Compression) (Codec, error) {
	switch compression {
	case None:
		return noneCodec{}, nil
	case Zlib:
		return zlibCodec{}, nil
	case Oodle:
		if codec, ok := c[Oodle]; ok {
			return codec, nil
		}
		return nil, fmt.Errorf("the archive is Oodle-compressed and no Oodle codec was given")
	}
	return nil, fmt.Errorf("unknown compression %d", compression)
}
