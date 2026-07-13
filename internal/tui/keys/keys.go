package keys

import (
	"charm.land/bubbles/v2/key"

	"github.com/matcra587/jira-cli/internal/tui/components/keybind"
)

// Map is the global key map. Fields are grouped by purpose but all live in one
// struct so [Map.Rebind] can address any of them by a stable lower-case name.
type Map struct {
	// Navigation.
	Up          key.Binding
	Down        key.Binding
	Top         key.Binding
	Bottom      key.Binding
	PageUp      key.Binding
	PageDown    key.Binding
	NextSection key.Binding
	PrevSection key.Binding
	Open        key.Binding
	Back        key.Binding

	// Queue control.
	Refresh     key.Binding
	TogglePause key.Binding
	Filter      key.Binding
	Facet       key.Binding
	Jumplist    key.Binding
	Search      key.Binding
	Presets     key.Binding
	NextLens    key.Binding
	PrevLens    key.Binding

	// Jira verbs (the triage loop).
	Transition key.Binding
	Comment    key.Binding
	Assign     key.Binding
	AssignMe   key.Binding
	Labels     key.Binding
	Worklog    key.Binding
	Edit       key.Binding
	Create     key.Binding

	// Cross-cutting.
	TogglePreview key.Binding
	GrowPreview   key.Binding
	ShrinkPreview key.Binding
	Zoom          key.Binding
	OpenBrowse    key.Binding
	CopyKey       key.Binding
	CopyURL       key.Binding
	Select        key.Binding
	SelectAll     key.Binding
	SelectInvert  key.Binding
	SelectRange   key.Binding
	Help          key.Binding
	Quit          key.Binding
}

// Default returns the stock key map. Bindings favor a Jira-native triage loop:
// j/k to move, enter to open, and single-key verbs (t/c/a/w) for the actions a
// reviewer repeats all day.
func Default() Map {
	return Map{
		Up:          key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:        key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Top:         key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "top")),
		Bottom:      key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "bottom")),
		PageUp:      key.NewBinding(key.WithKeys("ctrl+u", "pgup"), key.WithHelp("ctrl+u", "page up")),
		PageDown:    key.NewBinding(key.WithKeys("ctrl+d", "pgdown"), key.WithHelp("ctrl+d", "page down")),
		NextSection: key.NewBinding(key.WithKeys("tab", "right"), key.WithHelp("tab/→", "next view")),
		PrevSection: key.NewBinding(key.WithKeys("shift+tab", "left"), key.WithHelp("shift+tab/←", "prev view")),
		Open:        key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		Back:        key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),

		Refresh:     key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		TogglePause: key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "pause refresh")),
		Filter:      key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		Facet:       key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "facet")),
		Jumplist:    key.NewBinding(key.WithKeys("ctrl+o"), key.WithHelp("ctrl+o", "recent issues")),
		Search:      key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "search")),
		Presets:     key.NewBinding(key.WithKeys("ctrl+p"), key.WithHelp("ctrl+p", "saved queries")),
		NextLens:    key.NewBinding(key.WithKeys("]"), key.WithHelp("]", "next lens")),
		PrevLens:    key.NewBinding(key.WithKeys("["), key.WithHelp("[", "prev lens")),

		Transition: key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "transition")),
		Comment:    key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "comment")),
		Assign:     key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "assign")),
		AssignMe:   key.NewBinding(key.WithKeys("A"), key.WithHelp("A", "assign me")),
		Labels:     key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "labels")),
		Worklog:    key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "log work")),
		Edit:       key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit")),
		Create:     key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new issue")),

		TogglePreview: key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "move preview")),
		GrowPreview:   key.NewBinding(key.WithKeys("+", "="), key.WithHelp("+", "grow preview")),
		ShrinkPreview: key.NewBinding(key.WithKeys("-"), key.WithHelp("-", "shrink preview")),
		Zoom:          key.NewBinding(key.WithKeys("z"), key.WithHelp("z", "zoom")),
		OpenBrowse:    key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open browser")),
		CopyKey:       key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy key")),
		CopyURL:       key.NewBinding(key.WithKeys("Y"), key.WithHelp("Y", "copy url")),
		Select:        key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "select")),
		SelectAll:     key.NewBinding(key.WithKeys("ctrl+a"), key.WithHelp("ctrl+a", "select all/none")),
		SelectInvert:  key.NewBinding(key.WithKeys("X"), key.WithHelp("X", "invert selection")),
		SelectRange:   key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "select range")),
		Help:          key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:          key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

// index maps a stable name to the address of each binding so rebinding and
// enumeration share one source of truth. The registry holds pointers, so
// [Map.Rebind] mutates the caller's map in place.
func (m *Map) index() keybind.Registry {
	return keybind.Registry{
		"up": &m.Up, "down": &m.Down, "top": &m.Top, "bottom": &m.Bottom,
		"page_up": &m.PageUp, "page_down": &m.PageDown,
		"next_section": &m.NextSection, "prev_section": &m.PrevSection,
		"open": &m.Open, "back": &m.Back,
		"refresh": &m.Refresh, "toggle_pause": &m.TogglePause,
		"filter": &m.Filter, "facet": &m.Facet, "jumplist": &m.Jumplist, "search": &m.Search,
		"presets":   &m.Presets,
		"next_lens": &m.NextLens, "prev_lens": &m.PrevLens,
		"transition": &m.Transition, "comment": &m.Comment,
		"assign": &m.Assign, "assign_me": &m.AssignMe,
		"labels": &m.Labels, "worklog": &m.Worklog,
		"edit": &m.Edit, "create": &m.Create,
		"toggle_preview": &m.TogglePreview,
		"grow_preview":   &m.GrowPreview, "shrink_preview": &m.ShrinkPreview,
		"zoom":        &m.Zoom,
		"open_browse": &m.OpenBrowse, "copy_key": &m.CopyKey, "copy_url": &m.CopyURL,
		"select": &m.Select, "select_all": &m.SelectAll,
		"select_invert": &m.SelectInvert, "select_range": &m.SelectRange,
		"help": &m.Help, "quit": &m.Quit,
	}
}

// Names returns every rebindable binding name, sorted. Useful for docs and for
// validating a config file's keybinding overrides.
func (m *Map) Names() []string { return m.index().Names() }

// Rebind applies user overrides keyed by binding name (e.g. {"transition":
// {"x"}}); the override semantics — empty slices ignored, first key becomes
// the help label, unknown names error — live in the keybind registry.
func (m *Map) Rebind(overrides map[string][]string) error {
	return m.index().Rebind(overrides)
}
