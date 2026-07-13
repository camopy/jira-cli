package pill

import (
	"image/color"
	"testing"
)

// rgb flattens a color for comparison.
func rgb(c color.Color) [3]uint32 {
	r, g, b, _ := c.RGBA()
	return [3]uint32{r, g, b}
}

func TestFillPrefersColorNameOverCategory(t *testing.T) {
	// Jira's own designation wins even when the category disagrees, so the
	// badge matches what the Jira web UI shows for the same status.
	got := Fill("Weird", "done", "yellow")
	if rgb(got) != rgb(fillYellow) {
		t.Errorf("colorName yellow lost to category done")
	}
}

func TestFillCategoryFallback(t *testing.T) {
	cases := map[string]color.Color{
		"done":          fillGreen,
		"indeterminate": fillYellow,
		"new":           fillBlue,
	}
	for category, want := range cases {
		if got := Fill("Any", category, ""); rgb(got) != rgb(want) {
			t.Errorf("category %q fill = %v, want %v", category, got, want)
		}
	}
}

func TestFillHashFallbackIsStableAndCaseInsensitive(t *testing.T) {
	a := Fill("Waiting For Vendor", "", "")
	b := Fill("waiting for vendor", "", "")
	if rgb(a) != rgb(b) {
		t.Error("case variants of one status hashed to different fills")
	}
	if rgb(a) != rgb(Fill("Waiting For Vendor", "", "")) {
		t.Error("fallback fill not stable across calls")
	}
}

func TestForegroundContrastsWithEveryFill(t *testing.T) {
	// Every palette fill must pick the text color its luma demands: light
	// text on the dark fills, dark text on the light ones — the whole reason
	// the palette is fixed truecolor.
	fills := []color.Color{fillGreen, fillYellow, fillBlue, fillGray}
	fills = append(fills, fillFallbacks...)
	for _, f := range fills {
		fg := Foreground(f)
		r, g, b, _ := f.RGBA()
		luma := (299*r + 587*g + 114*b) / 1000
		wantDark := luma > 0x7fff
		if wantDark && rgb(fg) != rgb(textDark) {
			t.Errorf("light fill %v got light text", f)
		}
		if !wantDark && rgb(fg) != rgb(textLight) {
			t.Errorf("dark fill %v got dark text", f)
		}
	}
}

func TestStyleIsBoldWithContrastingPair(t *testing.T) {
	st := Style("Done", "done", "green")
	if !st.GetBold() {
		t.Error("pill style not bold")
	}
	if rgb(st.GetBackground()) != rgb(fillGreen) {
		t.Error("style background is not the category fill")
	}
	if rgb(st.GetForeground()) != rgb(Foreground(fillGreen)) {
		t.Error("style foreground is not the luma-picked contrast color")
	}
}
