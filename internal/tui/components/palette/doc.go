// Package palette is a fuzzy, type-to-filter command list for a
// command-palette dialog. Typing narrows the visible commands with
// subsequence (fuzzy) matching — the sibling picker package uses substring —
// up/down (and ctrl+p/ctrl+n) move the cursor, and the caller reads
// Selected() on its own accept key. Like picker, the palette never consumes
// enter or esc: accept and cancel belong to the wrapping dialog.
//
// Selecting a command yields its Entry, whose Key is the literal keybinding
// the caller replays (e.g. "t", "c", "tab"). The palette executes nothing
// itself, so an invocation routes through the exact keyboard path the user
// could have typed — a command surfaced here can never drift from the key it
// is bound to.
package palette
