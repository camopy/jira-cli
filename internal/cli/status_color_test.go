package cli

import (
	"image/color"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	ansi "github.com/charmbracelet/x/ansi"
	clibtheme "github.com/gechr/clib/theme"
)

// statusPill renders a status as a badge colored by its workflow category,
// mapping each of the three category keys to a distinct theme color and
// falling back to a per-name hash color for an unknown/absent category. The
// badge always sets a background fill, so its rendered output differs from the
// bare text and from the other categories.
func TestStatusPillColorsByCategory(t *testing.T) {
	theme := clibtheme.Dark()
	cfg := plainConfig{tty: true, theme: theme}
	const sample = "Some Status"

	rendered := map[string]string{}
	for _, category := range []string{"done", "indeterminate", "new"} {
		out := statusPill(cfg, sample, category, "").Render(sample)
		if out == sample || !strings.Contains(out, "\x1b[") {
			t.Errorf("category %q produced no styling: %q", category, out)
		}
		rendered[category] = out
	}
	if rendered["done"] == rendered["indeterminate"] || rendered["done"] == rendered["new"] ||
		rendered["indeterminate"] == rendered["new"] {
		t.Errorf("category colors collapsed together: %+v", rendered)
	}

	// Unknown category falls back to a per-name hash color: still a pill,
	// distinct from the category colors and stable for a given status name.
	fallback := statusPill(cfg, "Custom", "", "").Render("Custom")
	if fallback == "Custom" || !strings.Contains(fallback, "\x1b[") {
		t.Errorf("unknown category should still render a pill, got %q", fallback)
	}
	if again := statusPill(cfg, "Custom", "weird-key", "").Render("Custom"); again != fallback {
		t.Errorf("hash fallback not stable for the same name: %q vs %q", again, fallback)
	}

	// Non-TTY output carries no styling.
	if got := statusPill(plainConfig{tty: false, theme: theme}, "Done", "done", "green").Render("Done"); got != "Done" {
		t.Errorf("non-TTY pill should be unstyled, got %q", got)
	}

	// A TTY with no theme must not panic (the category branches deref the
	// theme) and produces no styling.
	if got := statusPill(plainConfig{tty: true, theme: nil}, "Done", "done", "green").Render("Done"); got != "Done" {
		t.Errorf("nil-theme pill should be unstyled, got %q", got)
	}
}

// TestStatusPillPrefersAPIColorName checks that Jira's own colorName
// designation wins over the category key, so the badge matches the Jira UI, and
// that medium-gray resolves to the theme's dim/grey rather than a category
// color.
func TestStatusPillPrefersAPIColorName(t *testing.T) {
	theme := clibtheme.Dark()
	cfg := plainConfig{tty: true, theme: theme}
	const s = "Whatever"

	// colorName overrides a mismatched category: "green" on a "new" status
	// renders the same as the done-category pill, not the new (blue) one.
	greenByColor := statusPill(cfg, s, "new", "green").Render(s)
	greenByCategory := statusPill(cfg, s, "done", "").Render(s)
	if greenByColor != greenByCategory {
		t.Errorf("colorName should override category: %q vs %q", greenByColor, greenByCategory)
	}
	if blue := statusPill(cfg, s, "new", "").Render(s); greenByColor == blue {
		t.Errorf("colorName green should not match the new-category pill")
	}

	// medium-gray maps to the dim/grey fill: a real pill, distinct from the
	// three category colors.
	grey := statusPill(cfg, s, "", "medium-gray").Render(s)
	if grey == s || !strings.Contains(grey, "\x1b[") {
		t.Errorf("medium-gray should render a styled pill, got %q", grey)
	}
	for _, cat := range []string{"done", "indeterminate", "new"} {
		if grey == statusPill(cfg, s, cat, "").Render(s) {
			t.Errorf("medium-gray should differ from the %q category color", cat)
		}
	}
}

// TestStatusPillCellGating checks the cell wrapper: a styled cell whose plain
// (measured) text is space-padded on a color TTY; bare text off-TTY; and an
// empty cell for an empty status.
func TestStatusPillCellGating(t *testing.T) {
	theme := clibtheme.Dark()

	styled := statusPillCell(plainConfig{tty: true, theme: theme}, "Done", "done", "green")
	if !strings.Contains(styled.Text, "\x1b[") {
		t.Errorf("TTY pill cell should be styled, got %q", styled.Text)
	}
	// The measured (plain) text carries a space either side so the column is
	// sized to the rendered fill, not the bare label.
	if styled.Plain != " Done " {
		t.Errorf("pill cell plain text = %q, want %q", styled.Plain, " Done ")
	}

	plain := statusPillCell(plainConfig{tty: false, theme: theme}, "Done", "done", "green")
	if plain.Text != "Done" || plain.Plain != "Done" {
		t.Errorf("non-TTY pill cell should be bare text, got Text=%q Plain=%q", plain.Text, plain.Plain)
	}

	empty := statusPillCell(plainConfig{tty: true, theme: theme}, "", "done", "green")
	if empty.Text != "" {
		t.Errorf("empty status should render an empty cell, got %q", empty.Text)
	}
}

// TestPriorityStyleFollowsJiraScale pins the priority colors to Jira's scale:
// red for high and highest (highest bold), orange for medium, blue for low and
// lowest.
func TestPriorityStyleFollowsJiraScale(t *testing.T) {
	theme := clibtheme.Dark()
	cfg := plainConfig{tty: true, theme: theme}
	const s = "x"
	cases := map[string]lipgloss.Style{
		"Highest": foregroundStyle(theme.Red).Bold(true),
		"High":    foregroundStyle(theme.Red),
		"Medium":  foregroundStyle(theme.Orange),
		"Low":     foregroundStyle(theme.Blue),
		"Lowest":  foregroundStyle(theme.Blue),
	}
	for name, want := range cases {
		if got := priorityStyle(cfg, name).Render(s); got != want.Render(s) {
			t.Errorf("priorityStyle(%q) = %q, want %q", name, got, want.Render(s))
		}
	}
	// Non-TTY output carries no styling.
	if got := priorityStyle(plainConfig{tty: false, theme: theme}, "Highest").Render(s); got != s {
		t.Errorf("non-TTY priority should be unstyled, got %q", got)
	}
}

// TestPillForegroundContrast pins the luma rule: a light fill takes dark text,
// a dark fill takes light text, so the label stays legible on any background.
func TestPillForegroundContrast(t *testing.T) {
	const (
		dark  = "#1c1c1c"
		light = "#f5f5f5"
	)
	cases := map[string]string{
		"#ffffff": dark,  // white fill
		"#000000": light, // black fill
		"#ffee00": dark,  // bright yellow
		"#00008b": light, // dark navy
	}
	for fill, want := range cases {
		gotR, gotG, gotB, _ := pillForeground(lipgloss.Color(fill)).RGBA()
		wantR, wantG, wantB, _ := lipgloss.Color(want).RGBA()
		if gotR != wantR || gotG != wantG || gotB != wantB {
			t.Errorf("pillForeground(%s) = %s", fill, want)
		}
	}

	// Basic 16-color fills are chosen by index, not RGBA: nominal yellow reads
	// as dark olive but a terminal renders it bright, so it must take dark text.
	basic := map[ansi.BasicColor]color.Color{
		ansi.Green:  lipgloss.Color(dark), // terminals render basic green bright
		ansi.Yellow: lipgloss.Color(dark),
		ansi.Cyan:   lipgloss.Color(dark),
		ansi.White:  lipgloss.Color(dark),
		ansi.Blue:   lipgloss.Color(light),
		ansi.Red:    lipgloss.Color(light),
	}
	for fill, want := range basic {
		wantR, wantG, wantB, _ := want.RGBA()
		gotR, gotG, gotB, _ := pillForeground(fill).RGBA()
		if gotR != wantR || gotG != wantG || gotB != wantB {
			t.Errorf("pillForeground(ansi %d) wrong contrast", fill)
		}
	}
}
