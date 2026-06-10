package markdown

import (
	"strings"
	"testing"

	"charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	clibtheme "github.com/gechr/clib/theme"

	"github.com/matcra587/jira-cli/internal/config"
)

func testRenderer() *Renderer {
	return NewRenderer(StyleFromTheme(config.DefaultTheme()))
}

func TestRenderWrapsToWidth(t *testing.T) {
	r := testRenderer()
	out := r.Render("JCT-1", 30, strings.Repeat("wrap me please ", 20))
	for _, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > 30 {
			t.Fatalf("line wider than wrap width: %d > 30 (%q)", w, line)
		}
	}
}

func TestRenderStylesHeading(t *testing.T) {
	r := testRenderer()
	out := r.Render("JCT-1", 60, "## Section\n\nbody text\n")
	if !strings.Contains(out, "Section") {
		t.Fatalf("heading text missing:\n%q", out)
	}
	if !strings.Contains(out, "\x1b[") {
		t.Error("output carries no ANSI styling")
	}
}

func TestRenderCachesPerIssueAndWidth(t *testing.T) {
	r := testRenderer()
	first := r.Render("JCT-1", 40, "hello **world**")
	if len(r.cache) != 1 {
		t.Fatalf("cache entries = %d, want 1", len(r.cache))
	}
	again := r.Render("JCT-1", 40, "hello **world**")
	if again != first {
		t.Error("cache hit returned different output")
	}
	r.Render("JCT-1", 60, "hello **world**") // same issue, new width = new entry
	r.Render("JCT-2", 40, "hello **world**") // other issue = new entry
	if len(r.cache) != 3 {
		t.Errorf("cache entries = %d, want 3 (per issue+width)", len(r.cache))
	}
}

func TestRenderInvalidatesOnContentChange(t *testing.T) {
	r := testRenderer()
	r.Render("JCT-1", 40, "first version")
	out := r.Render("JCT-1", 40, "second version")
	if !strings.Contains(ansi.Strip(out), "second version") {
		t.Errorf("stale cache served after content changed:\n%q", out)
	}
	if len(r.cache) != 1 {
		t.Errorf("cache entries = %d, want 1 (replaced, not appended)", len(r.cache))
	}
}

func TestRenderClampsWidth(t *testing.T) {
	r := testRenderer()
	if out := r.Render("JCT-1", -3, "tiny"); out == "" {
		t.Error("non-positive width returned empty output")
	}
}

func TestRenderEvictsWhenFull(t *testing.T) {
	r := testRenderer()
	for i := 0; i < maxCacheEntries+10; i++ {
		r.Render("JCT-"+strings.Repeat("x", i%50)+string(rune('a'+i%26)), 40+i, "body")
	}
	if len(r.cache) > maxCacheEntries {
		t.Errorf("cache grew past cap: %d > %d", len(r.cache), maxCacheEntries)
	}
}

func TestStyleFromThemeUsesPalette(t *testing.T) {
	th := config.DefaultTheme()
	cfg := StyleFromTheme(th)
	if cfg.H2.Color == nil {
		t.Fatal("H2 color not set from theme")
	}
	want := colorToken(th.Blue.GetForeground())
	if *cfg.H2.Color != want {
		t.Errorf("H2 color = %q, want theme blue %q", *cfg.H2.Color, want)
	}
	if cfg.Document.Margin == nil || *cfg.Document.Margin != 0 {
		t.Error("document margin must be 0 inside TUI panes")
	}
}

func TestColorTokenPreservesANSIIndexes(t *testing.T) {
	// An indexed color must pass through as its index so the user's terminal
	// palette renders it — converting to hex bakes in the standard VGA value
	// and stops matching the lipgloss-rendered chrome around the markdown.
	for in, want := range map[string]string{"4": "4", "212": "212", "#ff00aa": "#ff00aa"} {
		if got := colorToken(lipgloss.Color(in)); got != want {
			t.Errorf("colorToken(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStyleFromThemePicksBaseByBackground(t *testing.T) {
	light := clibtheme.Light()
	dark := clibtheme.Dark()
	lightCfg := StyleFromTheme(light)
	darkCfg := StyleFromTheme(dark)
	wantLight := styles.LightStyleConfig.CodeBlock.Chroma.Text.Color
	wantDark := styles.DarkStyleConfig.CodeBlock.Chroma.Text.Color
	if lightCfg.CodeBlock.Chroma == nil || *lightCfg.CodeBlock.Chroma.Text.Color != *wantLight {
		t.Error("light theme should ride glamour's light base (chroma palette)")
	}
	if darkCfg.CodeBlock.Chroma == nil || *darkCfg.CodeBlock.Chroma.Text.Color != *wantDark {
		t.Error("dark theme should ride glamour's dark base (chroma palette)")
	}
}
