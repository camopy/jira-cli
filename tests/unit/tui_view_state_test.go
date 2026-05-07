package unit

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/matcra587/jira-cli/internal/tui"
)

func TestTUIViewStatePreservedAcrossTabs(t *testing.T) {
	model := tui.New(t.Context())
	model.SetFilter("mine")
	updated, _ := model.Update(tea.KeyPressMsg{Code: '\t'})
	model = updated.(tui.App)
	updated, _ = model.Update(tea.KeyPressMsg{Code: '\t'})
	model = updated.(tui.App)
	updated, _ = model.Update(tea.KeyPressMsg{Code: '\t'})
	model = updated.(tui.App)
	updated, _ = model.Update(tea.KeyPressMsg{Code: '\t'})
	model = updated.(tui.App)
	if model.Filter() != "mine" {
		t.Fatalf("filter state = %q", model.Filter())
	}
}
