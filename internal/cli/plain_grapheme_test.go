package cli

import (
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"
	clibtheme "github.com/gechr/clib/theme"
)

// graphemeSampleIssues mixes a plain-ASCII summary with one carrying a ZWJ
// emoji sequence (wcwidth 4, grapheme 2) and VS16 emoji (wcwidth 1,
// grapheme 2) — the shapes that drift table columns when measured with the
// wrong method.
func graphemeSampleIssues() []map[string]any {
	return []map[string]any{
		{
			"key": "JCT-1", "summary": "👨‍💻 emoji probe — alignment ⚠️ check",
			"status": "To Do", "status_category": "new", "status_color": "blue-gray",
			"assignee": "", "priority": "Medium",
		},
		{
			"key": "JCT-2", "summary": "plain ascii row",
			"status": "In Progress", "status_category": "indeterminate", "status_color": "yellow",
			"assignee": "", "priority": "Medium",
		},
	}
}

func renderedIssueRows(t *testing.T, cfg plainConfig) []string {
	t.Helper()
	rows, err := issueRows(graphemeSampleIssues(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, row := range rows {
		if row != "" {
			out = append(out, row)
		}
	}
	// Header plus two data rows.
	if len(out) != 3 {
		t.Fatalf("expected 3 rendered rows, got %d: %q", len(out), out)
	}
	return out
}

// rowWidths measures every rendered row with the given method, excluding
// the last (unpadded) column by cutting at the final field — simplest is to
// measure the full row; data rows share identical trailing columns here so
// equal-width rows mean aligned columns.
func rowWidths(rows []string, method xansi.Method) []int {
	widths := make([]int, len(rows))
	for i, row := range rows {
		widths[i] = method.StringWidth(row)
	}
	return widths
}

// On a grapheme-clustering terminal the rows must align under grapheme
// measurement; under the default they must align under wcwidth. The two
// methods disagree by 3 cells on the emoji row, so each assertion would
// fail under the other mode — this pins that the option actually switches
// the measurement.
func TestIssueRowsAlignUnderSelectedWidthMethod(t *testing.T) {
	base := plainConfig{
		tty:       true,
		termWidth: 100,
		theme:     clibtheme.Dark(),
	}

	t.Run("grapheme", func(t *testing.T) {
		cfg := base
		cfg.graphemeWidth = true
		rows := renderedIssueRows(t, cfg)
		widths := rowWidths(rows[1:], xansi.GraphemeWidth)
		if widths[0] != widths[1] {
			t.Errorf("grapheme mode: data rows measure %v under grapheme width — a mode-2027 terminal draws these misaligned", widths)
		}
	})

	t.Run("wcwidth default", func(t *testing.T) {
		rows := renderedIssueRows(t, base)
		widths := rowWidths(rows[1:], xansi.WcWidth)
		if widths[0] != widths[1] {
			t.Errorf("default mode: data rows measure %v under wcwidth — a legacy terminal draws these misaligned", widths)
		}
	})
}

// The emoji fixture must actually exercise the disagreement, or the test
// above passes vacuously.
func TestGraphemeFixtureDisagrees(t *testing.T) {
	summary := graphemeSampleIssues()[0]["summary"].(string)
	wc := xansi.WcWidth.StringWidth(summary)
	gr := xansi.GraphemeWidth.StringWidth(summary)
	if wc == gr {
		t.Fatalf("fixture summary %q measures identically (%d) under both methods; pick a spicier emoji", summary, wc)
	}
	if !strings.Contains(summary, "\u200d") {
		t.Fatal("fixture summary lost its ZWJ sequence")
	}
}
