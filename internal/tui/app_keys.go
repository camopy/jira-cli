package tui

import "charm.land/bubbles/v2/key"

// appKeyMap holds the global key bindings owned by App.
type appKeyMap struct {
	Quit        key.Binding
	Help        key.Binding
	TogglePause key.Binding
	Refresh     key.Binding
	Profile     key.Binding
	FilterOpts  key.Binding
	Tab         key.Binding
	ShiftTab    key.Binding
	Back        key.Binding

	// Per-view action keys for the issues tab.
	Open       key.Binding
	OpenBrowse key.Binding
	Edit       key.Binding
	AssignMe   key.Binding
	Transition key.Binding
	Comment    key.Binding
	Worklog    key.Binding
	Create     key.Binding
	Delete     key.Binding
}

func newAppKeyMap() appKeyMap {
	return appKeyMap{
		Quit:        key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		Help:        key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		TogglePause: key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "toggle refresh")),
		Refresh:     key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh now")),
		Profile:     key.NewBinding(key.WithKeys("P"), key.WithHelp("P", "switch profile")),
		FilterOpts:  key.NewBinding(key.WithKeys("O"), key.WithHelp("O", "filter options")),
		Tab:         key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next tab")),
		ShiftTab:    key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev tab")),
		Back:        key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),

		Open:       key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open detail")),
		OpenBrowse: key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open in browser")),
		Edit:       key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit")),
		AssignMe:   key.NewBinding(key.WithKeys("A"), key.WithHelp("A", "assign to me")),
		Transition: key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "transition")),
		Comment:    key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "comment")),
		Worklog:    key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "log work")),
		Create:     key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new issue")),
		Delete:     key.NewBinding(key.WithKeys("D"), key.WithHelp("D", "delete")),
	}
}
