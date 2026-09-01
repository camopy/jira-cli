package theme

import (
	"hash/fnv"
	"image/color"
	"os"
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
	clibtheme "github.com/gechr/clib/theme"
	"github.com/matcra587/jira-cli/internal/config"
)

var applyOnce sync.Once

// Theme is the shared clib theme instance.
var Theme = config.DefaultTheme()

// autoDetected caches the "auto" background detection from startup. The
// detection writes an OSC query and reads the terminal's reply — safe only
// before the Bubble Tea program owns stdin. Run mid-program, the reply
// arrives as keystrokes and types garbage into whatever holds focus, so
// Resolve must never re-query once the dashboard is up.
var autoDetected *clibtheme.Theme

// DetectAutoOnce performs the terminal-background detection for the "auto"
// theme and caches it. Call it exactly once, before the Bubble Tea program
// starts; every later Resolve("auto") — a theme preview, a config reload —
// reuses the cached answer instead of racing the program for stdin.
func DetectAutoOnce() {
	if autoDetected == nil {
		autoDetected = config.AutoTheme(os.Stdout)
	}
}

// Resolve returns a clib theme for the given preset name. The "auto" name
// picks clib's light or dark theme from the startup background detection
// (see DetectAutoOnce) so hash-based entity colors contrast. An empty or
// unrecognized name falls back to the process default, which honors the
// JIRA_THEME override before the dark built-in.
func Resolve(name string) *clibtheme.Theme {
	if config.IsAutoTheme(name) {
		if autoDetected == nil {
			// No startup detection ran (a test path, a future entrypoint):
			// fall back to the process default rather than querying — the
			// OSC round-trip mid-program is exactly the reply-as-keystrokes
			// bug DetectAutoOnce exists to prevent.
			return config.ThemeForName("")
		}
		return autoDetected
	}
	return config.ThemeForName(name)
}

// Apply resets all derived styles to the given clib theme. Runs at most
// once per process; subsequent calls are no-ops (matching pdc's pattern,
// which avoids style flicker mid-render). A deliberate theme change at
// runtime goes through Reload instead.
func Apply(t *clibtheme.Theme) {
	applyOnce.Do(func() {
		applyTheme(t)
	})
}

// Reload swaps the active theme at runtime — the config hot-reload path.
// Unlike Apply it is not once-guarded: the caller signals a real theme
// change, owns the frame cadence (the reload lands between renders on the
// Update goroutine), and rebuilds anything that cached derived styles.
func Reload(t *clibtheme.Theme) {
	if t == nil {
		return
	}
	applyTheme(t)
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
	StatusOK   = lipgloss.NewStyle().Foreground(Theme.Green.GetForeground()).Bold(true)
	StatusErr  = lipgloss.NewStyle().Foreground(Theme.Red.GetForeground()).Bold(true)
	FlaggedRow = flaggedRowStyle(Theme)
)

func flaggedRowStyle(t *clibtheme.Theme) lipgloss.Style {
	if t.Background == clibtheme.BackgroundLight {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#332B00")).Background(lipgloss.Color("#FFF3C4"))
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#FFF2B2")).Background(lipgloss.Color("#4A3B00"))
}

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

// Code is the inline-code accent, matching the markdown renderer's code color
// so a `span` reads the same in a summary cell and a rendered description.
var Code = lipgloss.NewStyle().Foreground(Theme.Yellow.GetForeground())

// CodeSpans styles `backtick` spans in s with the Code accent, backticks
// kept — the display width is unchanged, so cell-budgeted callers truncate
// first and style after. An unpaired backtick renders as-is.
func CodeSpans(s string) string { return CodeSpansWith(s, lipgloss.NewStyle()) }

// CodeSpansWith is CodeSpans over a styled base: plain segments render with
// base and spans with the Code accent layered on it, segment by segment, so
// a span's SGR reset can never cut the base style off mid-string. Callers
// wrap and truncate the raw text first — this must be the last styling pass.
func CodeSpansWith(s string, base lipgloss.Style) string {
	if !strings.Contains(s, "`") {
		return base.Render(s)
	}
	code := Code.Inherit(base)
	var b strings.Builder
	for {
		start := strings.IndexByte(s, '`')
		if start < 0 {
			break
		}
		end := strings.IndexByte(s[start+1:], '`')
		if end < 0 {
			break
		}
		if start > 0 {
			b.WriteString(base.Render(s[:start]))
		}
		b.WriteString(code.Render(s[start : start+end+2]))
		s = s[start+end+2:]
	}
	if s != "" {
		b.WriteString(base.Render(s))
	}
	return b.String()
}

// Issue-type badge styles — color is the primary signal for the type glyph.
var (
	TypeEpic    = lipgloss.NewStyle().Foreground(Theme.Magenta.GetForeground())
	TypeStory   = lipgloss.NewStyle().Foreground(Theme.Green.GetForeground())
	TypeTask    = lipgloss.NewStyle().Foreground(Theme.Blue.GetForeground())
	TypeSubtask = lipgloss.NewStyle().Foreground(Theme.Blue.GetForeground()).Faint(true)
	TypeBug     = lipgloss.NewStyle().Foreground(Theme.Red.GetForeground())
	TypeOther   = lipgloss.NewStyle().Foreground(Theme.Yellow.GetForeground())
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
	FlaggedRow = flaggedRowStyle(t)

	PillDanger = Pill.Foreground(t.Red.GetForeground()).Bold(true)
	PillWarning = Pill.Foreground(t.Yellow.GetForeground())
	PillDim = Pill.Faint(true)
	PillOK = Pill.Foreground(t.Green.GetForeground())

	ColorStatusBarFg = t.MarkdownText.GetForeground()
	ColorTitleFg = t.MarkdownText.GetForeground()
	ColorHeaderFg = t.Blue.GetForeground()

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
	Code = lipgloss.NewStyle().Foreground(t.Yellow.GetForeground())

	Paused = lipgloss.NewStyle().Foreground(t.Red.GetForeground()).Bold(true)
	Active = lipgloss.NewStyle().Foreground(t.Green.GetForeground()).Bold(true)

	TypeEpic = lipgloss.NewStyle().Foreground(t.Magenta.GetForeground())
	TypeStory = lipgloss.NewStyle().Foreground(t.Green.GetForeground())
	TypeTask = lipgloss.NewStyle().Foreground(t.Blue.GetForeground())
	TypeSubtask = lipgloss.NewStyle().Foreground(t.Blue.GetForeground()).Faint(true)
	TypeBug = lipgloss.NewStyle().Foreground(t.Red.GetForeground())
	TypeOther = lipgloss.NewStyle().Foreground(t.Yellow.GetForeground())
}

// entityHues mirrors the CLI plain renderer's fixed mid-tone entity
// palette (internal/cli/plain.go entityHues): theme palettes are designed
// for one background, and identity hints must stay legible on both black
// and white terminals.
var entityHues = []color.Color{
	lipgloss.Color("#8f7ee7"), lipgloss.Color("#e56910"),
	lipgloss.Color("#227d9b"), lipgloss.Color("#da62ac"),
	lipgloss.Color("#82b536"), lipgloss.Color("#1d7afc"),
	lipgloss.Color("#1f845a"), lipgloss.Color("#b3822e"),
}

// EntityColor returns a deterministic style for a named entity (assignee,
// project, etc) by hashing into the fixed mid-tone entity palette.
func EntityColor(name string) lipgloss.Style {
	// An empty EntityColors slice is the monochrome/plain presets'
	// deliberate opt-out of entity coloring; the hues themselves come
	// from the fixed palette above.
	if name == "" || len(Theme.EntityColors) == 0 {
		return lipgloss.NewStyle()
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))

	c := entityHues[h.Sum32()%uint32(len(entityHues))]
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
