// Package input is the TUI's shared text-entry substrate: thin themed wrappers
// over bubbles/textinput and textarea, plus the external-editor hop. Every
// prompt in the dashboard (filter, JQL, action verbs) goes through here, so
// cursor movement, word-wise editing and bracketed paste behave identically
// everywhere — replacing the hand-rolled append-only buffers.
package input
