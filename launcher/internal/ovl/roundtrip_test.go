package ovl

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func readFixture(t *testing.T, name string) (*File, []byte) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	f, err := Read(raw, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return f, raw
}

// An OVL read and written untouched must come back byte for byte, and so
// must every archive decoded and encoded again.
func TestRoundTripIsByteIdentical(t *testing.T) {
	for _, name := range []string{"EmptyPZ.ovl", "OVLIncludesPZ.ovl", "cobra-pack.ovl"} {
		t.Run(name, func(t *testing.T) {
			f, raw := readFixture(t, name)
			out, ovs, err := f.Write()
			if err != nil {
				t.Fatal(err)
			}
			if ovs != nil || !bytes.Equal(out, raw) {
				t.Fatalf("untouched write differs: %d bytes, want %d", len(out), len(raw))
			}
			for _, a := range f.Archives {
				codec, _ := f.codecs.get(f.Compression())
				decompressed, err := codec.Decompress(a.compressed, int(a.Entry.UncompressedSize))
				if err != nil {
					t.Fatal(err)
				}
				content, err := f.Content(a)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(content.encode(), decompressed) {
					t.Errorf("archive %s: re-encoded content differs", f.ArchiveName(a))
				}
			}
		})
	}
}
