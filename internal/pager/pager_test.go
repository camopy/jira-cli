package pager

import (
	"os"
	"strings"
	"testing"
)

func TestOverflowsIsFalseOffTerminal(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "not-a-tty")
	if err != nil {
		t.Fatalf("CreateTemp() error = %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	tall := strings.Repeat("line\n", 10_000)
	if Overflows(f, tall) {
		t.Fatal("Overflows reported true for a non-terminal writer; paging must never trigger off-TTY")
	}
}
