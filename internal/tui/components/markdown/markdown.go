// Package markdown renders GFM (produced by internal/adf) for the TUI through
// glamour, with a style derived from the active clib theme so issue bodies and
// comments match the rest of the dashboard. Rendering is cached per
// issue+width because glamour is too slow to run on every View frame.
package markdown

import (
	"fmt"
	"hash/fnv"
	"image/color"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	xansi "github.com/charmbracelet/x/ansi"
	clibtheme "github.com/gechr/clib/theme"
)

// maxCacheEntries bounds the render cache; past it the whole cache resets
// (entries are cheap to rebuild and a dashboard shows ~tens of issues).
const maxCacheEntries = 256

type cacheKey struct {
	id    string
	width int
}

type cacheEntry struct {
	hash uint64
	out  string
}

// Renderer is a width-aware, cached glamour renderer. Not safe for concurrent
// use; the TUI renders from a single goroutine.
type Renderer struct {
	style     ansi.StyleConfig
	renderers map[int]*glamour.TermRenderer
	cache     map[cacheKey]cacheEntry
}

// NewRenderer builds a Renderer with the given glamour style.
func NewRenderer(style ansi.StyleConfig) *Renderer {
	return &Renderer{
		style:     style,
		renderers: make(map[int]*glamour.TermRenderer),
		cache:     make(map[cacheKey]cacheEntry),
	}
}

// Render returns md rendered at the given wrap width, cached under id+width
// and invalidated when the content changes (a refresh can rewrite an issue).
// On any glamour failure the raw markdown comes back — text always beats a
// blank pane.
func (r *Renderer) Render(id string, width int, md string) string {
	if width < 1 {
		width = 1
	}
	key := cacheKey{id: id, width: width}
	h := fnv.New64a()
	_, _ = h.Write([]byte(md))
	sum := h.Sum64()
	if e, ok := r.cache[key]; ok && e.hash == sum {
		return e.out
	}

	out := r.render(width, md)
	if len(r.cache) >= maxCacheEntries {
		r.cache = make(map[cacheKey]cacheEntry)
	}
	r.cache[key] = cacheEntry{hash: sum, out: out}
	return out
}

func (r *Renderer) render(width int, md string) string {
	tr, ok := r.renderers[width]
	if !ok {
		var err error
		tr, err = glamour.NewTermRenderer(
			glamour.WithStyles(r.style),
			glamour.WithWordWrap(width),
		)
		if err != nil {
			return md
		}
		// Same cap discipline as the render cache: a drag-resize can sweep
		// through many widths; don't keep a renderer for every one forever.
		if len(r.renderers) >= maxCacheEntries {
			r.renderers = make(map[int]*glamour.TermRenderer)
		}
		r.renderers[width] = tr
	}
	out, err := tr.Render(md)
	if err != nil {
		return md
	}
	// Glamour pads with blank lines around the document; the panes manage
	// their own spacing.
	return strings.Trim(out, "\n")
}

// StyleFromTheme derives a glamour style from the clib palette: headings in
// the chrome blue (H1 magenta, matching detail headers), links blue, inline
// code yellow, quotes and rules dim. The base config — which supplies
// everything not overridden here, notably the code-block chroma palette —
// follows the theme's declared background so a light theme never renders on
// glamour's dark defaults. Margins are zeroed — the panes own their padding.
func StyleFromTheme(t *clibtheme.Theme) ansi.StyleConfig {
	cfg := styles.DarkStyleConfig
	if t.Background == clibtheme.BackgroundLight {
		cfg = styles.LightStyleConfig
	}

	blue := ColorToken(t.Blue.GetForeground())
	magenta := ColorToken(t.Magenta.GetForeground())
	yellow := ColorToken(t.Yellow.GetForeground())
	text := ColorToken(t.MarkdownText.GetForeground())
	dim := ColorToken(t.Dim.GetForeground())

	cfg.Document.Margin = uintPtr(0)
	cfg.Document.Color = strPtr(text)

	cfg.H1.Color = strPtr(magenta)
	cfg.H1.BackgroundColor = nil
	for _, h := range []*ansi.StyleBlock{&cfg.H2, &cfg.H3, &cfg.H4, &cfg.H5, &cfg.H6} {
		h.Color = strPtr(blue)
	}
	cfg.Heading.Color = strPtr(blue)

	cfg.Strong.Color = strPtr(text)
	cfg.Emph.Color = strPtr(text)
	cfg.Item.Color = strPtr(text)
	cfg.Enumeration.Color = strPtr(text)

	cfg.Link.Color = strPtr(blue)
	cfg.LinkText.Color = strPtr(blue)
	cfg.Code.Color = strPtr(yellow)
	cfg.Code.BackgroundColor = nil
	cfg.BlockQuote.Color = strPtr(dim)
	cfg.HorizontalRule.Color = strPtr(dim)

	return cfg
}

// ColorToken converts a theme color to the string form glamour accepts.
// ANSI palette indexes pass through as their index ("4", "212"), so the
// terminal's own palette renders them exactly like the rest of the chrome —
// round-tripping an indexed color through RGBA would bake in the standard
// VGA value and visibly drift from the themed UI around it.
func ColorToken(c color.Color) string {
	switch v := c.(type) {
	case xansi.BasicColor:
		return fmt.Sprintf("%d", int(v))
	case xansi.IndexedColor:
		return fmt.Sprintf("%d", int(v))
	}
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", uint8(r>>8), uint8(g>>8), uint8(b>>8))
}

func strPtr(s string) *string { return &s }
func uintPtr(u uint) *uint    { return &u }
