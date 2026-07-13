package icons

import (
	"os"
	"strings"
)

// Mode selects a glyph set.
type Mode int

const (
	// Unicode is the portable set — renders in any monospace font.
	Unicode Mode = iota
	// Nerd uses Nerd Font glyphs (private-use-area codepoints); the user
	// opts in because only they know what font the terminal runs.
	Nerd
)

// Set is the complete glyph table for one mode. Every field is exactly one
// terminal cell wide so column layouts never shift between modes.
type Set struct {
	// Issue types.
	Epic        string
	Story       string
	Task        string
	Subtask     string
	Bug         string
	UnknownType string

	// Priorities.
	PriorityHighest string
	PriorityHigh    string
	PriorityMedium  string
	PriorityLow     string
	PriorityLowest  string

	// Chrome.
	Paused string
}

// unicodeSet is the portable table — the glyphs the dashboard has always used.
var unicodeSet = Set{
	Epic: "◆", Story: "●", Task: "■", Subtask: "▸", Bug: "▲", UnknownType: "◇",
	PriorityHighest: "↟", PriorityHigh: "↑", PriorityMedium: "=", PriorityLow: "↓", PriorityLowest: "↡",
	Paused: "⏸",
}

// nerdSet uses classic Font Awesome codepoints from the Nerd Fonts range —
// escaped rather than pasted so review can verify each one — all single-cell
// in the patched mono fonts. Shapes follow Jira's own iconography: bolt for
// epic, bookmark for story, check for task, fork for subtask, bug for bug.
var nerdSet = Set{
	Epic:        "\uf0e7", // nf-fa-bolt
	Story:       "\uf02e", // nf-fa-bookmark
	Task:        "\uf14a", // nf-fa-check_square
	Subtask:     "\uf126", // nf-fa-code_fork
	Bug:         "\uf188", // nf-fa-bug
	UnknownType: "\uf128", // nf-fa-question

	PriorityHighest: "\uf102", // nf-fa-angle_double_up
	PriorityHigh:    "\uf106", // nf-fa-angle_up
	PriorityMedium:  "\uf068", // nf-fa-minus
	PriorityLow:     "\uf107", // nf-fa-angle_down
	PriorityLowest:  "\uf103", // nf-fa-angle_double_down

	Paused: "\uf04c", // nf-fa-pause
}

// For returns the glyph table for a mode.
func For(mode Mode) Set {
	if mode == Nerd {
		return nerdSet
	}
	return unicodeSet
}

// Resolve maps a tui.icons config value to a mode: "nerd" and "unicode" are
// explicit, everything else (including empty) is auto — Nerd only when the
// NERD_FONT environment convention says the font is there, else Unicode.
// Wrong-but-explicit beats guessing: broken glyphs from a bad "nerd" setting
// are the user's to see and revert, silent downgrades are not.
func Resolve(value string) Mode {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "nerd":
		return Nerd
	case "unicode":
		return Unicode
	default:
		if os.Getenv("NERD_FONT") != "" {
			return Nerd
		}
		return Unicode
	}
}

// active is the process-wide table, defaulting to the portable set.
var active = unicodeSet

// Use installs a glyph table. Called at startup and on config hot-reload,
// both on the Update goroutine; render helpers read the table per call, so
// the swap shows on the next frame.
func Use(s Set) { active = s }

// Active returns the current glyph table.
func Active() Set { return active }
