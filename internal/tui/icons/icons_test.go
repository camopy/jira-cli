package icons

import (
	"reflect"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestResolveModes(t *testing.T) {
	t.Setenv("NERD_FONT", "")
	cases := map[string]Mode{
		"nerd":    Nerd,
		"NERD":    Nerd,
		"unicode": Unicode,
		"":        Unicode, // auto with no env convention stays portable
		"auto":    Unicode,
		"bogus":   Unicode, // unknown values degrade, never break the TUI
	}
	for value, want := range cases {
		if got := Resolve(value); got != want {
			t.Errorf("Resolve(%q) = %v, want %v", value, got, want)
		}
	}
}

func TestResolveAutoHonorsNerdFontEnv(t *testing.T) {
	t.Setenv("NERD_FONT", "1")
	if Resolve("auto") != Nerd {
		t.Error("auto ignored the NERD_FONT convention")
	}
	// Explicit unicode still wins over the env convention.
	if Resolve("unicode") != Unicode {
		t.Error("explicit unicode overridden by NERD_FONT")
	}
}

// TestSetsAreCompleteAndSingleCell walks both tables: every glyph present,
// every glyph exactly one terminal cell — the invariant that keeps column
// layouts identical across modes.
func TestSetsAreCompleteAndSingleCell(t *testing.T) {
	for name, set := range map[string]Set{"unicode": unicodeSet, "nerd": nerdSet} {
		v := reflect.ValueOf(set)
		for i := range v.NumField() {
			glyph := v.Field(i).String()
			field := v.Type().Field(i).Name
			if glyph == "" {
				t.Errorf("%s set: %s is empty", name, field)
				continue
			}
			if w := lipgloss.Width(glyph); w != 1 {
				t.Errorf("%s set: %s glyph %q is %d cells wide, want 1", name, field, glyph, w)
			}
		}
	}
}

func TestUseSwapsActiveSet(t *testing.T) {
	t.Cleanup(func() { Use(unicodeSet) })
	Use(For(Nerd))
	if Active().Bug != nerdSet.Bug {
		t.Error("Use did not install the nerd set")
	}
	Use(For(Unicode))
	if Active().Bug != unicodeSet.Bug {
		t.Error("Use did not restore the unicode set")
	}
}
