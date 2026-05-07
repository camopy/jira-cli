package integration

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/matcra587/jira-cli/internal/tui"
)

func TestTUITabNavigationAndViewReachability(t *testing.T) {
	model := tui.New(t.Context())
	if model.ActiveTab() != "issues" {
		t.Fatalf("initial tab = %q", model.ActiveTab())
	}
	updated, _ := model.Update(tea.KeyPressMsg{Code: '\t'})
	model = updated.(tui.App)
	if model.ActiveTab() != "epics" {
		t.Fatalf("tab after tab key = %q", model.ActiveTab())
	}
}
