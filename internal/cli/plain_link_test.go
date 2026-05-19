package cli

import (
	"strings"
	"testing"

	termansi "github.com/gechr/x/ansi"
)

func TestPlainPadRightUsesDisplayWidth(t *testing.T) {
	got := padRight("界", 4)
	if width := termansi.StringWidth(got); width != 4 {
		t.Fatalf("display width = %d, want 4 for %q", width, got)
	}
	if spaces := strings.Count(got, " "); spaces != 2 {
		t.Fatalf("spaces = %d, want 2 for %q", spaces, got)
	}
}

func TestPlainTruncateUsesDisplayWidth(t *testing.T) {
	got := truncate("世界abc", 4)
	if width := termansi.StringWidth(got); width > 4 {
		t.Fatalf("display width = %d, want <= 4 for %q", width, got)
	}
	if !strings.Contains(got, "…") {
		t.Fatalf("truncate did not append ellipsis: %q", got)
	}
}
