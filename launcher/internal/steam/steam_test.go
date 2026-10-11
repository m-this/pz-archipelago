package steam

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFindGameInASecondLibrary(t *testing.T) {
	root := t.TempDir()
	lib := t.TempDir()
	write(t, filepath.Join(root, "steamapps", "libraryfolders.vdf"), `// written by Steam
"libraryfolders"
{
	"0"
	{
		"path"		"`+root+`"
		"apps" { "228980" "1" }
	}
	"1"
	{
		"path"		"`+lib+`"
		"apps"
		{
			"703080"		"20000000000"
		}
	}
}`)
	write(t, filepath.Join(lib, "steamapps", "appmanifest_703080.acf"), `"AppState"
{
	"appid"		"703080"
	"installdir"		"Planet Zoo"
}`)
	got, err := FindGame(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(lib, "steamapps", "common", "Planet Zoo"); got != want {
		t.Errorf("%s, want %s", got, want)
	}
}

func TestFindGameNowhere(t *testing.T) {
	if _, err := FindGame(t.TempDir()); !errors.Is(err, ErrNotFound) {
		t.Errorf("err %v", err)
	}
}

func TestParseVDFRefuses(t *testing.T) {
	for _, bad := range []string{`"a" {`, `"a"`, `}`, `"a" "unterminated`} {
		if _, err := ParseVDF(bad); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}

func TestParseVDFEscapes(t *testing.T) {
	n, err := ParseVDF(`"k" { "path" "D:\\Games\\Steam" }`)
	if err != nil {
		t.Fatal(err)
	}
	if got := n.Child("K", "PATH").Value; got != `D:\Games\Steam` {
		t.Errorf("%q", got)
	}
}
