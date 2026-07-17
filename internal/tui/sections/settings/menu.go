// The interactive settings menu: typed rows over the config
// keys. Enums cycle in place and apply live (a theme change re-skins on the
// spot); numbers and lists open a small inline form. Every change saves
// through config.Save — the same struct write `jira config set` uses — and
// hot-reloads the dashboard.

package settings

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	xstrings "github.com/gechr/x/strings"

	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/internal/tui/components/form"
	"github.com/matcra587/jira-cli/internal/tui/core"
	"github.com/matcra587/jira-cli/internal/tui/icons"
)

// settingKind picks the row's interaction: cycle in place or open a form.
type settingKind int

const (
	kindEnum settingKind = iota
	kindNumber
	kindText
)

// setting is one menu row: how to read the current value, the choices (for
// enums), and how to apply a new value onto the config before saving.
type setting struct {
	name    string
	kind    settingKind
	hint    string
	options func(m *Model) []string
	current func(cfg *config.Config) string
	apply   func(cfg *config.Config, value string) error
	// complete plugs token autocompletion into a text row's form (e.g. tab
	// names while editing the comma-separated tabs list).
	complete func(m *Model) *form.Autocomplete
	// preview applies a candidate value to the live view while its picker is
	// still open — the dashboard shows the highlighted theme — without
	// touching the config file. Backing out previews the original back.
	preview func(m *Model, value string) tea.Cmd
}

// staticOptions adapts a fixed choice list.
func staticOptions(values ...string) func(*Model) []string {
	return func(*Model) []string { return values }
}

// settingsMenu is the row inventory. Only keys with a sane interactive shape
// appear; the structured tables (sections, lenses, keys) live in the raw file,
// one `e` away.
func settingsMenu() []setting {
	return []setting{
		{
			name: "Theme", kind: kindEnum,
			hint:    "re-skins every view live",
			options: staticOptions(config.ThemeNameValues...),
			current: func(cfg *config.Config) string { return cfg.Theme.Name },
			apply:   func(cfg *config.Config, v string) error { cfg.Theme.Name = v; return nil },
			preview: func(_ *Model, v string) tea.Cmd {
				return func() tea.Msg { return core.ThemePreviewMsg{Name: v} }
			},
		},
		{
			name: "Icons", kind: kindEnum,
			hint:    "nerd needs a Nerd Font in the terminal",
			options: staticOptions("auto", "nerd", "unicode"),
			current: func(cfg *config.Config) string { return cfg.TUI.Icons },
			apply:   func(cfg *config.Config, v string) error { cfg.TUI.Icons = v; return nil },
			preview: func(_ *Model, v string) tea.Cmd {
				// Glyphs are read per render, so installing the set is the
				// whole preview; the restyle re-renders cached rows.
				icons.Use(icons.For(icons.Resolve(v)))
				return func() tea.Msg { return core.RestyleMsg{} }
			},
		},
		{
			name: "Preview", kind: kindEnum,
			hint:    "where the issue preview docks",
			options: staticOptions("auto", "right", "left", "bottom", "hidden"),
			current: func(cfg *config.Config) string { return cfg.TUI.Preview },
			apply:   func(cfg *config.Config, v string) error { cfg.TUI.Preview = v; return nil },
		},
		{
			name: "Preview size", kind: kindNumber,
			hint:    "percent of the split, 20–80",
			current: func(cfg *config.Config) string { return strconv.Itoa(cfg.TUI.PreviewSize) },
			apply: func(cfg *config.Config, v string) error {
				n, err := strconv.Atoi(strings.TrimSpace(v))
				if err != nil {
					return fmt.Errorf("preview size must be a number: %q", v)
				}
				if n != 0 && (n < 20 || n > 80) {
					return fmt.Errorf("preview size %d is outside 20–80 (0 restores the default)", n)
				}
				cfg.TUI.PreviewSize = n
				return nil
			},
		},
		{
			name: "Refresh interval", kind: kindNumber,
			hint:    "seconds between auto-refreshes; 0 disables",
			current: func(cfg *config.Config) string { return strconv.Itoa(cfg.TUI.RefreshInterval) },
			apply: func(cfg *config.Config, v string) error {
				n, err := strconv.Atoi(strings.TrimSpace(v))
				if err != nil || n < 0 {
					return fmt.Errorf("refresh interval must be a non-negative number of seconds: %q", v)
				}
				cfg.TUI.RefreshInterval = n
				return nil
			},
		},
		{
			name: "Default tab", kind: kindEnum,
			hint: "the landing view",
			options: func(m *Model) []string {
				if cfg := m.ctx.Config; cfg != nil && len(cfg.TUI.Tabs) > 0 {
					return cfg.TUI.Tabs
				}
				return []string{"issues", "epics", "search", "settings"}
			},
			current: func(cfg *config.Config) string { return cfg.TUI.DefaultTab },
			apply:   func(cfg *config.Config, v string) error { cfg.TUI.DefaultTab = v; return nil },
		},
		{
			name: "Tabs", kind: kindText,
			hint:    "comma-separated, in order",
			current: func(cfg *config.Config) string { return strings.Join(cfg.TUI.Tabs, ", ") },
			apply: func(cfg *config.Config, v string) error {
				cfg.TUI.Tabs = xstrings.SplitCSV(v)
				return nil
			},
			complete: tabNameComplete,
		},
		{
			name: "Default lens", kind: kindText,
			hint:    "lens title the Issues tab lands on",
			current: func(cfg *config.Config) string { return cfg.TUI.DefaultLens },
			apply: func(cfg *config.Config, v string) error {
				cfg.TUI.DefaultLens = strings.TrimSpace(v)
				return nil
			},
		},
	}
}

// tabNameComplete completes tab names while editing the tabs list: the
// built-in section IDs plus every configured section title, prefix-matched
// against the token being typed. Bare mode with a comma boundary, so each
// comma-separated entry completes on its own.
func tabNameComplete(m *Model) *form.Autocomplete {
	known := []string{"issues", "epics", "search", "board", "settings"}
	if cfg := m.ctx.Config; cfg != nil {
		for _, sec := range cfg.TUI.Sections {
			if t := strings.TrimSpace(sec.Title); t != "" {
				known = append(known, t)
			}
		}
	}
	return &form.Autocomplete{
		IsBoundary: func(r rune) bool { return r == ',' || unicode.IsSpace(r) },
		Fetch: func(query string) []form.Suggestion {
			var out []form.Suggestion
			for _, name := range known {
				if strings.HasPrefix(strings.ToLower(name), strings.ToLower(query)) {
					// A tab name is its own value and label.
					out = append(out, form.Suggestion{Value: name, Label: name})
				}
			}
			return out
		},
	}
}

// displayValue renders a row's current value. A blank enum means the
// conservative default ("auto"); numbers show their real value (the hint
// explains what 0 means); blank text reads as unset rather than broken.
func displayValue(s setting, cfg *config.Config) string {
	v := s.current(cfg)
	if !xstrings.IsBlank(v) {
		return v
	}
	if s.kind == kindEnum {
		return "auto"
	}
	return "(unset)"
}
