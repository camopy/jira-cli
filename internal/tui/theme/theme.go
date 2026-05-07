// Package theme defines lipgloss styles shared across the Jira TUI.
// The structure mirrors pagerduty-client's theme: package-level style vars
// derived from a clib theme, applied once via Apply(). All chrome and
// content styles are derived from clib theme colors so that swapping the
// underlying theme reskins the entire TUI.
package theme

import (
	"fmt"
	"hash/fnv"
	"image/color"
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
	clibtheme "github.com/gechr/clib/theme"
)

var applyOnce sync.Once

// Theme is the shared clib theme instance.
var Theme = clibtheme.Default()

// Resolve returns a clib theme for the given preset name. An empty or
// unrecognized name returns the clib default (which honors JIRA_THEME /
// CLIB_THEME env vars before falling back to the built-in default).
func Resolve(name string) *clibtheme.Theme {
	if strings.TrimSpace(name) == "" {
		return clibtheme.Default()
	}
	var th clibtheme.Theme
	if err := th.UnmarshalText([]byte(name)); err != nil {
		return clibtheme.Default()
	}
	return &th
}

// Apply resets all derived styles to the given clib theme. Runs at most
// once per process; subsequent calls are no-ops (matching pdc's pattern,
// which avoids style flicker mid-render).
func Apply(t *clibtheme.Theme) {
	applyOnce.Do(func() {
		applyTheme(t)
	})
}

// Status styles — mapped to Jira issue status categories.
var (
	StatusToDo       = lipgloss.NewStyle().Foreground(Theme.Blue.GetForeground())
	StatusInProgress = lipgloss.NewStyle().Foreground(Theme.Yellow.GetForeground()).Bold(true)
	StatusDone       = lipgloss.NewStyle().Foreground(Theme.Green.GetForeground())
	StatusBlocked    = lipgloss.NewStyle().Foreground(Theme.Red.GetForeground()).Bold(true)
)

// Priority styles — Jira priority labels.
var PriorityStyles = map[string]lipgloss.Style{
	"Highest": lipgloss.NewStyle().Foreground(Theme.Red.GetForeground()).Bold(true),
	"High":    lipgloss.NewStyle().Foreground(Theme.Orange.GetForeground()).Bold(true),
	"Medium":  lipgloss.NewStyle().Foreground(Theme.Yellow.GetForeground()),
	"Low":     lipgloss.NewStyle().Foreground(Theme.Blue.GetForeground()),
	"Lowest":  lipgloss.NewStyle().Faint(true),
}

// PriorityStyle returns the style for a Jira priority name plus an ok flag.
func PriorityStyle(name string) (lipgloss.Style, bool) {
	s, ok := PriorityStyles[name]
	return s, ok
}

// StatusStyle returns the style for a Jira status name. Falls back to
// the dim style for unknown statuses.
func StatusStyle(name string) lipgloss.Style {
	switch strings.ToLower(name) {
	case "to do", "todo", "open", "backlog", "new":
		return StatusToDo
	case "in progress", "in review", "in-progress":
		return StatusInProgress
	case "done", "closed", "resolved", "complete":
		return StatusDone
	case "blocked", "impeded", "on hold":
		return StatusBlocked
	default:
		return *Theme.Dim
	}
}

// Status flash styles for action feedback.
var (
	StatusOK  = lipgloss.NewStyle().Foreground(Theme.Green.GetForeground()).Bold(true)
	StatusErr = lipgloss.NewStyle().Foreground(Theme.Red.GetForeground()).Bold(true)
)

// Pill styles for header counts.
var (
	Pill        = lipgloss.NewStyle().Padding(0, 1)
	PillDanger  = Pill.Foreground(Theme.Red.GetForeground()).Bold(true)
	PillWarning = Pill.Foreground(Theme.Yellow.GetForeground())
	PillDim     = Pill.Faint(true)
	PillOK      = Pill.Foreground(Theme.Green.GetForeground())
)

// UI chrome colors.
var (
	ColorStatusBarFg = Theme.MarkdownText.GetForeground()
	ColorTitleFg     = Theme.MarkdownText.GetForeground()
	ColorHeaderFg    = Theme.Blue.GetForeground()
)

// TableHeader is the column header style for the issue list.
var TableHeader = lipgloss.NewStyle().
	Foreground(ColorHeaderFg).
	Bold(true).
	BorderStyle(lipgloss.NormalBorder()).
	BorderBottom(true)

// CursorBg / SelectedBg are raw ANSI background escapes used by the
// list view to highlight the cursor row and multi-selected rows.
var (
	CursorBg   = tintBg(Theme.Blue.GetForeground(), 0.18)
	SelectedBg = tintBg(Theme.Green.GetForeground(), 0.12)
)

// Title for section/panel titles.
var Title = lipgloss.NewStyle().Foreground(ColorTitleFg).Bold(true).Padding(0, 1)

// HelpOverlay outer container style (rounded border, padded, no bg).
var HelpOverlay = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(Theme.Dim.GetForeground()).
	Padding(1, 2)

// HelpKey / HelpDesc style key labels and descriptions in help/hint bars.
var (
	HelpKey  = lipgloss.NewStyle().Foreground(Theme.Yellow.GetForeground()).Bold(true)
	HelpDesc = *Theme.Dim
)

// Detail view styles for the issue detail viewport.
var (
	DetailHeader = lipgloss.NewStyle().Bold(true).Foreground(Theme.Magenta.GetForeground())
	DetailLabel  = lipgloss.NewStyle().Bold(true).Foreground(Theme.Green.GetForeground())
	DetailValue  = lipgloss.NewStyle().Foreground(Theme.MarkdownText.GetForeground())
	DetailDim    = lipgloss.NewStyle().Faint(true)
)

// Refresh indicator styles.
var (
	Paused = lipgloss.NewStyle().Foreground(Theme.Red.GetForeground()).Bold(true)
	Active = lipgloss.NewStyle().Foreground(Theme.Green.GetForeground()).Bold(true)
)

func applyTheme(t *clibtheme.Theme) {
	Theme = t

	StatusToDo = lipgloss.NewStyle().Foreground(t.Blue.GetForeground())
	StatusInProgress = lipgloss.NewStyle().Foreground(t.Yellow.GetForeground()).Bold(true)
	StatusDone = lipgloss.NewStyle().Foreground(t.Green.GetForeground())
	StatusBlocked = lipgloss.NewStyle().Foreground(t.Red.GetForeground()).Bold(true)

	PriorityStyles = map[string]lipgloss.Style{
		"Highest": lipgloss.NewStyle().Foreground(t.Red.GetForeground()).Bold(true),
		"High":    lipgloss.NewStyle().Foreground(t.Orange.GetForeground()).Bold(true),
		"Medium":  lipgloss.NewStyle().Foreground(t.Yellow.GetForeground()),
		"Low":     lipgloss.NewStyle().Foreground(t.Blue.GetForeground()),
		"Lowest":  lipgloss.NewStyle().Faint(true),
	}

	StatusOK = lipgloss.NewStyle().Foreground(t.Green.GetForeground()).Bold(true)
	StatusErr = lipgloss.NewStyle().Foreground(t.Red.GetForeground()).Bold(true)

	PillDanger = Pill.Foreground(t.Red.GetForeground()).Bold(true)
	PillWarning = Pill.Foreground(t.Yellow.GetForeground())
	PillDim = Pill.Faint(true)
	PillOK = Pill.Foreground(t.Green.GetForeground())

	ColorStatusBarFg = t.MarkdownText.GetForeground()
	ColorTitleFg = t.MarkdownText.GetForeground()
	ColorHeaderFg = t.Blue.GetForeground()
	CursorBg = tintBg(t.Blue.GetForeground(), 0.18)
	SelectedBg = tintBg(t.Green.GetForeground(), 0.12)

	TableHeader = lipgloss.NewStyle().
		Foreground(ColorHeaderFg).
		Bold(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true)
	Title = lipgloss.NewStyle().Foreground(ColorTitleFg).Bold(true).Padding(0, 1)

	HelpOverlay = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Dim.GetForeground()).
		Padding(1, 2)
	HelpKey = lipgloss.NewStyle().Foreground(t.Yellow.GetForeground()).Bold(true)
	HelpDesc = *t.Dim

	DetailHeader = lipgloss.NewStyle().Bold(true).Foreground(t.Magenta.GetForeground())
	DetailLabel = lipgloss.NewStyle().Bold(true).Foreground(t.Green.GetForeground())
	DetailValue = lipgloss.NewStyle().Foreground(t.MarkdownText.GetForeground())
	DetailDim = lipgloss.NewStyle().Faint(true)

	Paused = lipgloss.NewStyle().Foreground(t.Red.GetForeground()).Bold(true)
	Active = lipgloss.NewStyle().Foreground(t.Green.GetForeground()).Bold(true)
}

// tintBg mixes a color with black at the given intensity (0–1) and
// returns a 24-bit ANSI bg escape. Low intensities produce barely-visible
// tints that don't fight foreground text.
func tintBg(c color.Color, intensity float64) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm",
		int(float64(r>>8)*intensity),
		int(float64(g>>8)*intensity),
		int(float64(b>>8)*intensity),
	)
}

// EntityColor returns a deterministic style for a named entity (assignee,
// project, etc) by hashing into the clib theme's EntityColors palette.
func EntityColor(name string) lipgloss.Style {
	if name == "" {
		return lipgloss.NewStyle()
	}
	colors := Theme.EntityColors
	if len(colors) == 0 {
		return lipgloss.NewStyle()
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))

	c := colors[h.Sum32()%uint32(len(colors))]
	return lipgloss.NewStyle().Foreground(c)
}

// RenderEntityNames colors each name individually and joins with ", ".
func RenderEntityNames(names []string) string {
	var styled []string
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		styled = append(styled, EntityColor(name).Render(name))
	}
	return strings.Join(styled, ", ")
}
