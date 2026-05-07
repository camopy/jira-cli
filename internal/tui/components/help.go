package components

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/matcra587/jira-cli/internal/tui/theme"
)

// binding describes a single keybinding entry.
type binding struct {
	key  string
	desc string
}

// section groups bindings under a heading.
type section struct {
	title    string
	bindings []binding
	footer   string
}

var dashboardSections = []section{
	{
		title: "Actions",
		bindings: []binding{
			{"e", "edit"},
			{"A", "assign to me"},
			{"m", "transition"},
			{"c", "comment"},
			{"w", "log work"},
			{"n", "new issue"},
			{"D *", "delete"},
		},
		footer: "* requires confirmation",
	},
	{
		title: "Navigation",
		bindings: []binding{
			{"j/k ↑↓", "navigate"},
			{"enter", "open detail"},
			{"esc", "back / clear"},
			{"g/G", "top / bottom"},
			{"tab", "next tab"},
			{"shift+tab", "previous tab"},
		},
	},
	{
		title: "Filters",
		bindings: []binding{
			{"/", "text filter"},
			{"O", "filter options"},
		},
	},
	{
		title: "Other",
		bindings: []binding{
			{"o", "open in browser"},
			{"r", "refresh now"},
			{"R", "toggle refresh"},
			{"P", "switch profile"},
			{"?", "help"},
			{"q", "quit"},
		},
	},
}

var detailSections = []section{
	{
		title: "Actions",
		bindings: []binding{
			{"e", "edit"},
			{"m", "transition"},
			{"c", "comment"},
			{"w", "log work"},
		},
	},
	{
		title: "Navigation",
		bindings: []binding{
			{"↑↓", "scroll"},
			{"tab", "next tab"},
			{"shift+tab", "previous tab"},
			{"esc", "back to list"},
		},
	},
	{
		title: "Other",
		bindings: []binding{
			{"o", "open in browser"},
			{"?", "help"},
			{"q", "quit"},
		},
	},
}

var epicsSections = []section{
	{
		title: "Navigation",
		bindings: []binding{
			{"↑↓", "navigate"},
			{"enter", "open epic"},
			{"g/G", "top / bottom"},
		},
	},
	{
		title: "Other",
		bindings: []binding{
			{"r", "refresh"},
			{"tab", "switch tab"},
			{"?", "help"},
			{"q", "quit"},
		},
	},
}

var searchSections = []section{
	{
		title: "Navigation",
		bindings: []binding{
			{"/", "filter"},
			{"↑↓", "navigate"},
			{"enter", "open"},
		},
	},
	{
		title: "Other",
		bindings: []binding{
			{"r", "refresh"},
			{"tab", "switch tab"},
			{"?", "help"},
			{"q", "quit"},
		},
	},
}

var activitySections = []section{
	{
		title: "Navigation",
		bindings: []binding{
			{"↑↓", "navigate"},
		},
	},
	{
		title: "Other",
		bindings: []binding{
			{"r", "refresh"},
			{"tab", "switch tab"},
			{"?", "help"},
			{"q", "quit"},
		},
	},
}

// Help is a Bubble Tea model that renders a context-aware keybinding overlay.
type Help struct {
	Visible     bool
	CurrentView string
}

func (h Help) Init() tea.Cmd { return nil }

func (h Help) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "?", "esc":
			h.Visible = false
		}
	}
	return h, nil
}

func (h Help) View() tea.View {
	if !h.Visible {
		return tea.NewView("")
	}

	var sections []section
	switch h.CurrentView {
	case "detail":
		sections = detailSections
	case "epics":
		sections = epicsSections
	case "search":
		sections = searchSections
	case "activity":
		sections = activitySections
	default:
		sections = dashboardSections
	}

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(theme.ColorTitleFg)
	dimDesc := lipgloss.NewStyle().Faint(true)

	columns := make([]string, 0, len(sections))
	for _, sec := range sections {
		var sb strings.Builder
		sb.WriteString(sectionStyle.Render(sec.title))
		sb.WriteString("\n")
		for _, b := range sec.bindings {
			// %-10s pads the key cell so descriptions line up across rows.
			key := theme.HelpKey.Render(fmt.Sprintf("%-10s", b.key))
			desc := dimDesc.Render(b.desc)
			sb.WriteString(key + desc + "\n")
		}
		if sec.footer != "" {
			sb.WriteString(dimDesc.Render(sec.footer) + "\n")
		}
		columns = append(columns, sb.String())
	}

	// Pair-and-stack: 4 sections → 2×2 grid (cols 0+2 left, 1+3 right);
	// 3 sections → first column on its own, last two stacked on the right;
	// 2 sections → side-by-side; 1 section → single column.
	var left, right string
	switch len(columns) {
	case 4:
		left = columns[0] + "\n" + columns[2]
		right = columns[1] + "\n" + columns[3]
	case 3:
		left = columns[0]
		right = columns[1] + "\n" + columns[2]
	case 2:
		left = columns[0]
		right = columns[1]
	case 1:
		left = columns[0]
	}

	var content string
	if right != "" {
		content = lipgloss.JoinHorizontal(lipgloss.Top, left, "    ", right)
	} else {
		content = left
	}

	return tea.NewView(RenderOverlay(content, 0))
}
