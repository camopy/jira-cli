package components

import (
	tea "charm.land/bubbletea/v2"

	"github.com/matcra587/jira-cli/internal/tui/theme"
)

// ConfirmResult is emitted when the user accepts or cancels a confirmation.
// When Confirmed is true, OnYes carries the action command to execute.
type ConfirmResult struct {
	Confirmed bool
	OnYes     tea.Cmd
}

// Confirm is a modal y/n dialog used by destructive workflows
// (clone/move/delete) and any other action that needs an explicit OK.
type Confirm struct {
	Visible bool
	title   string
	message string
	onYes   tea.Cmd
}

// Show returns a new Confirm with Visible=true and the supplied prompt.
// The original is unmodified — callers reassign: c = c.Show(...).
func (c Confirm) Show(title, message string, onYes tea.Cmd) Confirm {
	c.Visible = true
	c.title = title
	c.message = message
	c.onYes = onYes
	return c
}

func (c Confirm) Init() tea.Cmd { return nil }

func (c Confirm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if !c.Visible {
		return c, nil
	}
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return c, nil
	}
	switch k.String() {
	case "y", "enter":
		cmd := c.onYes
		c.Visible = false
		c.onYes = nil
		return c, func() tea.Msg { return ConfirmResult{Confirmed: true, OnYes: cmd} }
	case "n", "esc":
		c.Visible = false
		c.onYes = nil
		return c, func() tea.Msg { return ConfirmResult{Confirmed: false} }
	}
	return c, nil
}

func (c Confirm) View() tea.View {
	if !c.Visible {
		return tea.NewView("")
	}
	title := theme.Title.Render(c.title)
	message := theme.HelpDesc.Render(c.message)
	keys := theme.HelpKey.Render("y") + theme.HelpDesc.Render(" confirm  ") +
		theme.HelpKey.Render("n") + theme.HelpDesc.Render(" cancel")
	content := title + "\n\n" + message + "\n\n" + keys
	return tea.NewView(RenderOverlay(content, 40))
}
