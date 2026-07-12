// Package settings is the dashboard's configuration view: it shows where the
// active config came from and what it resolved to, offers a manual reload, and
// watches the file (on the shared refresh heartbeat) so edits hot-apply
// without restarting the TUI. Theme and credentials still need a restart —
// the theme applies once per process and auth requires a new client.
package settings

import (
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	xstrings "github.com/gechr/x/strings"

	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/internal/tui/core"
	"github.com/matcra587/jira-cli/internal/tui/theme"
)

// ID is the section identifier.
const ID core.SectionID = "settings"

var _ core.Section = (*Model)(nil)

// Model is the settings section. It holds no config copy of its own — it
// renders straight from the shared context, so a hot-reload elsewhere is
// reflected on the next frame.
type Model struct {
	ctx *core.ProgramContext
	// lastMod is the config file's mtime at the last load, the change signal
	// for auto-reload. The zero value means "unknown" and never triggers.
	lastMod time.Time
	notice  string
}

// New builds the settings section.
func New(ctx *core.ProgramContext) core.Section { return &Model{ctx: ctx} }

func (m *Model) ID() core.SectionID { return ID }
func (m *Model) Title() string      { return "Settings" }

// Init records the config file's current mtime as the auto-reload baseline.
func (m *Model) Init(ctx *core.ProgramContext) tea.Cmd {
	m.ctx = ctx
	m.lastMod = m.mtime()
	return nil
}

// mtime returns the config file's modification time, or the zero time when
// there is no path or the file is unreadable.
func (m *Model) mtime() time.Time {
	if m.ctx.ConfigPath == "" {
		return time.Time{}
	}
	fi, err := os.Stat(m.ctx.ConfigPath)
	if err != nil {
		return time.Time{}
	}
	return fi.ModTime()
}

// maybeReload fires a reload when the file's mtime moved since the last load.
// It runs on the shared refresh heartbeat, so auto-reload is only as live as
// tui.refresh_interval (and off with it) — r always reloads immediately.
func (m *Model) maybeReload() tea.Cmd {
	cur := m.mtime()
	if cur.IsZero() || cur.Equal(m.lastMod) {
		return nil
	}
	m.lastMod = cur
	return m.reloadCmd()
}

// reload is the manual path (the r key): it refreshes the mtime baseline and
// re-reads the file. maybeReload records the mtime it already observed and
// calls reloadCmd directly — a second stat here could see a newer write than
// the one the load reads, and the follow-up change would then be skipped.
func (m *Model) reload() tea.Cmd {
	if m.ctx.ConfigPath == "" {
		m.notice = "no config file to reload"
		return nil
	}
	m.lastMod = m.mtime()
	return m.reloadCmd()
}

// reloadCmd re-reads the config file and hands it to the App as a
// ConfigReloadedMsg; a parse failure surfaces in the footer instead.
func (m *Model) reloadCmd() tea.Cmd {
	path := m.ctx.ConfigPath
	return func() tea.Msg {
		cfg, err := config.Load(config.WithPath(path))
		if err != nil {
			return core.ErrorMsg{Err: fmt.Errorf("reload config: %w", err)}
		}
		return core.ConfigReloadedMsg{Config: cfg}
	}
}

// Update handles the reload key, the heartbeat poll, and the reload echo.
func (m *Model) Update(msg tea.Msg) (core.Section, tea.Cmd) {
	switch msg := msg.(type) {
	case core.RefreshTickMsg:
		return m, m.maybeReload()
	case core.ConfigReloadedMsg:
		m.notice = "config reloaded " + time.Now().Format("15:04:05")
		return m, nil
	case tea.KeyPressMsg:
		if key.Matches(msg, m.ctx.Keys.Refresh) {
			m.notice = ""
			return m, m.reload()
		}
	}
	return m, nil
}

// View renders the resolved configuration as labeled rows.
func (m *Model) View() string {
	label := theme.DetailLabel
	dim := theme.DetailDim
	var b strings.Builder
	row := func(name, value string) {
		fmt.Fprintf(&b, "%s  %s\n", label.Render(fmt.Sprintf("%-14s", name)), value)
	}

	path := m.ctx.ConfigPath
	if path == "" {
		path = dim.Render("(none)")
	}
	row("Config", path)
	prof := m.ctx.ProfileName
	if m.ctx.BaseURL != "" {
		prof += dim.Render("  " + m.ctx.BaseURL)
	}
	row("Profile", prof)
	if xstrings.AnyNonEmpty(m.ctx.Project, m.ctx.Board) {
		row("Project", strings.TrimSpace(m.ctx.Project+"  "+m.ctx.Board))
	}

	if cfg := m.ctx.Config; cfg != nil {
		refresh := "off " + dim.Render("(auto-reload off too — r still works)")
		if cfg.TUI.RefreshInterval > 0 {
			refresh = fmt.Sprintf("%ds · auto-reloads this file on change", cfg.TUI.RefreshInterval)
		}
		row("Refresh", refresh)
		preview := cfg.TUI.Preview
		if preview == "" {
			preview = "auto"
		}
		row("Preview", preview+dim.Render("  p cycles right → bottom → left → hidden"))
		row("Default tab", cfg.TUI.DefaultTab)
		row("Tabs", strings.Join(cfg.TUI.Tabs, ", "))
		for i, sec := range cfg.TUI.Sections {
			name := fmt.Sprintf("Section %d", i+1)
			title := sec.Title
			if title == "" {
				title = dim.Render("(untitled)")
			}
			row(name, title+dim.Render("  "+xstrings.Truncate(sec.JQL, 60, "…")))
		}
	} else {
		b.WriteString(dim.Render("no config loaded — built-in defaults\n"))
	}

	b.WriteString("\n" + dim.Render("theme and credentials need a restart"))
	if m.notice != "" {
		b.WriteString("\n" + theme.StatusOK.Render(m.notice))
	}
	hint := m.ctx.Styles.HintKey.Render("r") + " " + m.ctx.Styles.HintDesc.Render("reload now")
	b.WriteString("\n\n" + hint)
	return lipgloss.NewStyle().Padding(0, 1).Render(b.String())
}

// CapturesInput: the settings view has no text input.
func (m *Model) CapturesInput() bool { return false }

// HelpBindings lists the section's contextual bindings.
func (m *Model) HelpBindings() []key.Binding {
	return []key.Binding{m.ctx.Keys.Refresh}
}
