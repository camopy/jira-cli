// Package picker is a vertical, type-to-filter select list for modal choices
// (workflow transitions today; assignees, labels and facet values next).
// Typing narrows the list fzf-style through the shared input substrate,
// up/down move the cursor, and the caller reads Selected() on its own submit
// key — the picker never consumes enter or esc.
//
// primer was considered first per the project rule: its picker package is a
// settings grid (label rows with horizontal choice cycling) and its pick
// package is a blocking huh form that owns the terminal — neither embeds in a
// running Bubble Tea program as a select list, so this stays in-tree on the
// existing input.Line substrate.
package picker
