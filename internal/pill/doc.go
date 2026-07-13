// Package pill is the single source of the status-badge palette: fixed
// truecolor fills keyed by Jira's status category (matching the Jira UI's
// own colors) and a luma-computed contrasting text color. Fixed RGB rather
// than theme colors is deliberate — theme colors resolve to basic ANSI
// palette slots whose actual rendering is whatever the user's terminal
// remaps them to, so fill/text contrast on a themed pill was a guess that
// broke on remapped palettes. Both the CLI plain renderer and the TUI draw
// their pills from here, so status reads identically everywhere.
package pill
