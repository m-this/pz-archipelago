package logbook

import (
	"fmt"
	"slices"
	"testing"
)

func TestBookKeepsWholeLines(t *testing.T) {
	var b Book
	_, _ = b.Write([]byte("one\ntw"))
	_, _ = b.Write([]byte("o\nthree"))
	if got := b.Lines(); !slices.Equal(got, []string{"one", "two"}) {
		t.Errorf("lines %v", got)
	}
	for i := range kept + 10 {
		_, _ = fmt.Fprintf(&b, "%d\n", i)
	}
	if got := b.Lines(); len(got) != kept || got[len(got)-1] != fmt.Sprint(kept+9) {
		t.Errorf("%d lines, last %q", len(got), got[len(got)-1])
	}
}
