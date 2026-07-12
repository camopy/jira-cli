// Package markdown renders GFM (produced by internal/adf) for the TUI and the
// release-notes plain view through glamour, with a style derived from the
// active clib theme so issue bodies and comments match the rest of the
// dashboard. The caching renderer is primer's (identity+width keyed,
// content-hash invalidated — glamour is too slow to run on every View frame);
// this package only maps the clib theme onto primer's palette.
package markdown

import (
	"image/color"

	"charm.land/glamour/v2/ansi"
	clibtheme "github.com/gechr/clib/theme"
	"github.com/gechr/primer/render"
)

// Renderer is a width-aware, cached glamour renderer. Concurrency-safe,
// though the TUI renders from a single goroutine.
type Renderer = render.MarkdownRenderer

// NewRenderer builds a Renderer with the given glamour style.
func NewRenderer(style ansi.StyleConfig) *Renderer {
	return render.NewMarkdownRenderer(style)
}

// StyleFromTheme derives a glamour style from the clib palette: headings in
// the chrome blue (H1 magenta, matching detail headers), links blue, inline
// code yellow, quotes and rules dim. The base config — which supplies
// everything not overridden here, notably the code-block chroma palette —
// follows the theme's declared background so a light theme never renders on
// glamour's dark defaults. Margins are zeroed — the panes own their padding.
func StyleFromTheme(t *clibtheme.Theme) ansi.StyleConfig {
	base := render.BackgroundDark
	if t.Background == clibtheme.BackgroundLight {
		base = render.BackgroundLight
	}
	return render.StyleFromPalette(render.MarkdownPalette{
		Base:    base,
		Text:    t.MarkdownText.GetForeground(),
		Heading: t.Blue.GetForeground(),
		H1:      t.Magenta.GetForeground(),
		Link:    t.Blue.GetForeground(),
		Code:    t.Yellow.GetForeground(),
		Dim:     t.Dim.GetForeground(),
	})
}

// ColorToken converts a theme color to the string form glamour accepts.
// ANSI palette indexes pass through as their index ("4", "212"), so the
// terminal's own palette renders them exactly like the rest of the chrome —
// round-tripping an indexed color through RGBA would bake in the standard
// VGA value and visibly drift from the themed UI around it.
func ColorToken(c color.Color) string { return render.ColorToken(c) }
