// Package icons is the TUI's glyph table: one Set per icon mode (Nerd Font
// or plain Unicode) so both stay complete, selected by the tui.icons config
// key. There is no reliable terminal query for font capability, so "auto"
// stays conservative — it opts into Nerd Fonts only on an explicit
// environment convention (a non-empty NERD_FONT) and otherwise renders the
// Unicode set that works everywhere. The active set is package state on the
// theme package's precedent: render helpers read it per call, so a config
// hot-reload swaps glyphs on the next frame without threading state through
// every row renderer.
package icons
