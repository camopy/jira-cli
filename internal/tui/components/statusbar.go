package components

import (
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/matcra587/jira-cli/internal/tui/theme"
)

// HintContext picks the right keybinding hints for the bottom hint line.
type HintContext struct {
	View         string // "dashboard", "detail", "epics", "search", "activity"
	FilterActive bool
	Paused       bool
	Editing      bool
}

// StatusBar renders the bottom chrome: a labeled separator with profile +
// refresh state, and a hint line of contextual keybindings.
type StatusBar struct {
	Profile     string
	Total       int
	Open        int
	Done        int
	LastRefresh time.Time
	Paused      bool
	Width       int
	StatusMsg   string
	Hint        HintContext
}

func (s StatusBar) Init() tea.Cmd                         { return nil }
func (s StatusBar) Update(_ tea.Msg) (tea.Model, tea.Cmd) { return s, nil }

func (s StatusBar) View() tea.View {
	left := theme.HelpKey.Render("?") + " " + theme.HelpDesc.Render("help")
	if s.Profile != "" {
		left = theme.HelpKey.Render("⚑") + " " + theme.HelpDesc.Render(s.Profile) + "  " + left
	}
	right := s.refreshLabel()
	sep := LabelledBorder(s.Width, theme.HelpDesc, left, right)

	hint := s.StatusMsg
	if hint == "" {
		hint = s.hintView()
	}
	hint = lipgloss.PlaceHorizontal(s.Width, lipgloss.Center, hint)
	return tea.NewView(sep + "\n" + hint)
}

// LabelledBorder renders ── left ────────── right ── as a horizontal rule.
func LabelledBorder(width int, borderStyle lipgloss.Style, left, right string) string {
	rule := borderStyle.Render("─")
	gap := borderStyle.Render(" ")
	leftPart := rule + rule + gap + left + gap
	rightPart := gap + right + gap + rule + rule
	leftW := lipgloss.Width(leftPart)
	rightW := lipgloss.Width(rightPart)
	fillW := width - leftW - rightW
	if fillW < 0 {
		fillW = 0
	}
	fill := borderStyle.Render(strings.Repeat("─", fillW))
	return leftPart + fill + rightPart
}

func (s StatusBar) refreshLabel() string {
	if s.Paused {
		return theme.Paused.Render("⏸  paused")
	}
	if s.LastRefresh.IsZero() {
		return theme.Active.Render("↻")
	}
	secs := int(time.Since(s.LastRefresh).Seconds())
	return theme.Active.Render("↻") + " " + strconv.Itoa(secs) + "s"
}

func (s StatusBar) hintView() string {
	if s.Width <= 0 {
		return ""
	}
	bindings := s.hintBindings()
	const sep = "  "
	sepW := lipgloss.Width(sep)
	render := func(b key.Binding) string {
		return theme.HelpKey.Render(b.Help().Key) + " " + theme.HelpDesc.Render(b.Help().Desc)
	}
	var parts []string
	used := 0
	for _, b := range bindings {
		r := render(b)
		w := lipgloss.Width(r)
		need := w
		if len(parts) > 0 {
			need += sepW
		}
		if used+need > s.Width {
			ell := theme.HelpDesc.Render("…")
			if used+lipgloss.Width(ell) <= s.Width {
				parts = append(parts, ell)
			}
			break
		}
		parts = append(parts, r)
		used += need
	}
	return strings.Join(parts, sep)
}

func (s StatusBar) hintBindings() []key.Binding {
	bind := func(k, desc string) key.Binding {
		return key.NewBinding(key.WithKeys(k), key.WithHelp(k, desc))
	}
	switch {
	case s.Hint.FilterActive:
		return []key.Binding{
			bind("type", "filter"),
			bind("enter", "commit"),
			bind("esc", "clear"),
		}
	case s.Hint.View == "detail":
		return []key.Binding{
			bind("e", "edit"),
			bind("m", "transition"),
			bind("c", "comment"),
			bind("w", "worklog"),
			bind("o", "open"),
			bind("esc", "back"),
			bind("↑↓", "scroll"),
		}
	case s.Hint.View == "epics":
		return []key.Binding{
			bind("↑↓", "navigate"),
			bind("enter", "expand"),
			bind("R", "refresh"),
			bind("?", "help"),
			bind("q", "quit"),
		}
	case s.Hint.View == "search":
		return []key.Binding{
			bind("/", "filter"),
			bind("enter", "open"),
			bind("?", "help"),
			bind("q", "quit"),
		}
	case s.Hint.View == "activity":
		return []key.Binding{
			bind("↑↓", "navigate"),
			bind("?", "help"),
			bind("q", "quit"),
		}
	default:
		return []key.Binding{
			bind("j/k", "nav"),
			bind("/", "filter"),
			bind("O", "options"),
			bind("enter", "open"),
			bind("e", "edit"),
			bind("A", "assign me"),
			bind("m", "transition"),
			bind("c", "comment"),
			bind("w", "worklog"),
			bind("n", "new"),
			bind("o", "browser"),
			bind("r", "refresh"),
			bind("P", "profile"),
			bind("?", "help"),
			bind("q", "quit"),
		}
	}
}
