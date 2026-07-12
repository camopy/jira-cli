// Package markdown renders GFM (produced by internal/adf) for the TUI and the
// release-notes plain view through glamour, with a style derived from the
// active clib theme so issue bodies and comments match the rest of the
// dashboard. The caching renderer is primer's (identity+width keyed,
// content-hash invalidated — glamour is too slow to run on every View frame);
// this package only maps the clib theme onto primer's palette.
package markdown
