package cli

import (
	"testing"

	clibtheme "github.com/gechr/clib/theme"
)

// statusStyle colors a status by its workflow category, mapping each of the
// three category keys to a distinct theme color and falling back to a
// per-name hash color for an unknown/absent category. Styles are compared by
// the ANSI they produce for a sample string.
func TestStatusStyleColorsByCategory(t *testing.T) {
	theme := clibtheme.Default()
	cfg := plainConfig{tty: true, theme: theme}
	const sample = "Some Status"

	cases := map[string]string{
		"done":          foregroundStyle(theme.Green).Render(sample),
		"indeterminate": foregroundStyle(theme.Yellow).Render(sample),
		"new":           foregroundStyle(theme.Blue).Render(sample),
	}
	for category, want := range cases {
		if got := statusStyle(cfg, sample, category).Render(sample); got != want {
			t.Errorf("statusStyle category %q = %q, want %q", category, got, want)
		}
	}

	// Unknown category falls back to a per-name hash color: distinct from the
	// category colors and stable for a given status name.
	fallback := statusStyle(cfg, "Custom", "").Render("Custom")
	for category, want := range cases {
		if fallback == want {
			t.Errorf("unknown category should not reuse the %q color", category)
		}
	}
	if again := statusStyle(cfg, "Custom", "weird-key").Render("Custom"); again != fallback {
		t.Errorf("hash fallback not stable for the same name: %q vs %q", again, fallback)
	}

	// Non-TTY output carries no styling.
	if got := statusStyle(plainConfig{tty: false, theme: theme}, "Done", "done").Render("Done"); got != "Done" {
		t.Errorf("non-TTY status style should be unstyled, got %q", got)
	}

	// A TTY with no theme must not panic (the category branches deref the
	// theme) and produces no styling.
	if got := statusStyle(plainConfig{tty: true, theme: nil}, "Done", "done").Render("Done"); got != "Done" {
		t.Errorf("nil-theme status style should be unstyled, got %q", got)
	}
}
