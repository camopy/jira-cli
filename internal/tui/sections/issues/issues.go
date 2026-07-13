package issues

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/matcra587/jira-cli/internal/tui/components/form"
	"github.com/matcra587/jira-cli/internal/tui/components/input"
	"github.com/matcra587/jira-cli/internal/tui/core"
)

var _ core.Section = (*Model)(nil)

// Model is the issues triage section: the shared results view plus quick-filter
// lenses as its query source.
type Model struct {
	results
	lens int
}

// New builds the issues triage section.
func New(ctx *core.ProgramContext) core.Section {
	m := &Model{results: newResults(ctx, ID)}
	m.refetch = m.fetch
	return m
}

// ID returns the issues section's identifier.
func (m *Model) ID() core.SectionID { return ID }

// Title returns the tab-bar label.
func (m *Model) Title() string { return "Issues" }

// Init sizes the list (reserving one row for the chips header) and fetches the
// configured default lens (tui.default_lens, first lens otherwise).
func (m *Model) Init(ctx *core.ProgramContext) tea.Cmd {
	m.ctx = ctx
	m.refetch = m.fetch
	m.lens = defaultLensIndex(m.lenses(), ctx.DefaultLens)
	m.applySize(1)
	return m.fetch()
}

// lenses returns the configured quick-filters ([[tui.lenses]]) or the
// built-ins. Read per call so a config hot-reload takes effect immediately —
// and the active index is clamped on the same call, so a reload that shrank
// the set can never index out of range anywhere downstream (fetch or render).
func (m *Model) lenses() []Lens {
	lenses := lensesFor(m.ctx)
	if m.lens >= len(lenses) {
		m.lens = 0
	}
	return lenses
}

// fetch runs the active lens's JQL.
func (m *Model) fetch() tea.Cmd {
	return m.runFetch(m.lenses()[m.lens].JQL)
}

// cycleLens switches the active quick-filter and refetches.
func (m *Model) cycleLens(delta int) tea.Cmd {
	return m.setLens(m.lens + delta)
}

// setLens activates a lens by index (modulo the lens count) and reruns it.
func (m *Model) setLens(i int) tea.Cmd {
	n := len(m.lenses())
	m.lens = ((i % n) + n) % n
	m.filter = ""
	return m.fetch()
}

// Update delegates shared behavior to results and handles the lens/refresh keys.
func (m *Model) Update(msg tea.Msg) (core.Section, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.applySize(1)
		return m, nil
	case core.TaskFinishedMsg:
		cmd, _ := m.handleTask(msg)
		return m, cmd
	case core.RestyleMsg:
		m.restyle()
		return m, nil
	case core.RefreshTickMsg:
		return m, m.autoRefresh()
	case tea.MouseWheelMsg:
		return m, m.handleWheel(msg)
	case tea.MouseClickMsg:
		// A click on the chips row switches the lens — but never under a
		// modal/filter/detail capture, and only inside the main pane (a
		// left-docked preview shifts the chips right).
		if !m.capturing() && msg.Button == tea.MouseLeft && msg.Y == core.TopChromeRows {
			if x, ok := m.mainX(msg.X); ok {
				if i, ok := lensAt(m.lenses(), x); ok {
					return m, m.setLens(i)
				}
			}
		}
		return m, m.handleClick(msg)
	case input.EditorFinishedMsg:
		return m, m.handleEditor(msg)
	case form.SuggestionsMsg:
		// Autocomplete fetches resolve as commands; their results must find
		// their way back into the open form or the seam silently drops them.
		cmd, _ := m.ctrl.Update(msg)
		return m, cmd
	case spinner.TickMsg:
		return m, m.handleSpinner(msg)
	case flashClearMsg:
		m.handleFlashClear(msg)
		return m, nil
	case tea.PasteMsg:
		cmd, _ := m.handlePaste(msg)
		return m, cmd
	case tea.KeyPressMsg:
		if cmd, handled := m.handleKey(msg); handled {
			return m, cmd
		}
		switch {
		case key.Matches(msg, m.ctx.Keys.Refresh):
			return m, m.fetch()
		case key.Matches(msg, m.ctx.Keys.NextLens):
			return m, m.cycleLens(1)
		case key.Matches(msg, m.ctx.Keys.PrevLens):
			return m, m.cycleLens(-1)
		}
	}
	return m, nil
}

// View renders the lens chips (with the active lens's JQL hint) above the
// shared results body.
func (m *Model) View() string {
	return m.view(chipsWithQuery(m.lenses(), m.lens, m.ctx.MainWidth))
}

// CapturesInput reports filter/action focus.
func (m *Model) CapturesInput() bool { return m.capturing() }

// HelpBindings lists the section's contextual bindings, verbs first so the
// footer hint leads with the triage actions.
func (m *Model) HelpBindings() []key.Binding {
	k := m.ctx.Keys
	return []key.Binding{
		k.Open, k.Create, k.Transition, k.AssignMe, k.Comment, k.Edit, k.Labels, k.OpenBrowse,
		k.Select, k.SelectAll, k.SelectInvert, k.SelectRange,
		k.Filter, k.Facet, k.Jumplist, k.NextLens, k.TogglePreview, k.Refresh,
	}
}
