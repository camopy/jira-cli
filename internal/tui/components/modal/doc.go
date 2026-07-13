// Package modal is the shared overlay frame: a styled, width-capped box
// composited centered over the current view — the assembly every overlay
// (action forms, pickers, confirmations) otherwise rebuilds by hand. The
// frame owns only geometry and placement: the box style is injected so it
// stays theme-agnostic, the width cap keeps long content from bleeding past
// the screen edges, and compositing goes through primer's overlay so the
// backdrop shows around the box. Content (title, fields, hints) is rendered
// by the caller; the frame is deliberately the last, dumb step.
package modal
