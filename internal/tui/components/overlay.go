// Package components contains reusable Bubble Tea overlays and chrome
// for the Jira TUI. Patterns mirror pagerduty-client/internal/tui/components.
package components

import (
	"charm.land/lipgloss/v2"

	"github.com/matcra587/jira-cli/internal/tui/theme"
)

// RenderOverlay wraps content in the standard overlay style (rounded border,
// dim foreground, no background — inherits the terminal's default bg so the
// overlay works on any color scheme).
func RenderOverlay(content string, minWidth int) string {
	if lipgloss.Width(content) < minWidth {
		return theme.HelpOverlay.Width(minWidth).Render(content)
	}
	return theme.HelpOverlay.Render(content)
}
