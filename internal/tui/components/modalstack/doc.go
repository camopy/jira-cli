// Package modalstack manages a stack of overlay layers (confirm, input, filter,
// help) so more than one can be open at once and they dismiss in LIFO order.
// Only the top layer is rendered, centered over the background.
//
// Layers are render functions, not pre-rendered strings: each is called with
// the current terminal size at render time, so an open modal reflows correctly
// when the terminal is resized without the caller re-pushing it.
package modalstack
