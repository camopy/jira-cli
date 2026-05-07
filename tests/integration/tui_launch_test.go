package integration

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/matcra587/jira-cli/internal/tui"
)

// TestTUILaunchRendersAndHandlesQuit smoke-tests the App: a default-options
// New() renders something, and pressing 'q' returns tea.Quit.
func TestTUILaunchRendersAndHandlesQuit(t *testing.T) {
	app := tui.New(t.Context())
	if got := app.View().Content; got == "" {
		t.Fatal("initial view is empty")
	}
	_, cmd := app.Update(tea.KeyPressMsg{Code: 'q'})
	if cmd == nil {
		t.Fatal("q did not return quit command")
	}
}

// TestTUIHelpKeyTogglesHelpOverlay verifies the global '?' keybinding
// opens the help overlay. The overlay is part of the App's render now,
// not a per-view ad-hoc string.
func TestTUIHelpKeyTogglesHelpOverlay(t *testing.T) {
	app := tui.New(t.Context())
	if app.HelpVisible() {
		t.Fatal("help should not be visible on launch")
	}
	updated, _ := app.Update(tea.KeyPressMsg{Code: '?'})
	app = updated.(tui.App)
	if !app.HelpVisible() {
		t.Fatal("? should open help overlay")
	}
}
