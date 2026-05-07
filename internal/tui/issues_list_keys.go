package tui

import "charm.land/bubbles/v2/key"

// listKeyMap holds key bindings for the issuesList view.
type listKeyMap struct {
	Up     key.Binding
	Down   key.Binding
	Top    key.Binding
	Bottom key.Binding
	Filter key.Binding
}

func newListKeyMap() listKeyMap {
	return listKeyMap{
		Up:     key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/up", "move up")),
		Down:   key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/down", "move down")),
		Top:    key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "jump to top")),
		Bottom: key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "jump to bottom")),
		Filter: key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
	}
}
