package core

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// SectionID identifies a view (e.g. "issues", "search").
type SectionID string

// Counter is an optional Section capability: a section that knows how many
// items it holds reports them for the tab bar ("Issues (6)").
// ok is false until the section has loaded, so an unvisited tab shows no count.
type Counter interface {
	Count() (n int, ok bool)
}

// SectionMsg is a message addressed to a specific section. The App
// broadcasts these to every section instead of routing them to the active
// one; the addressee matches the ID itself.
type SectionMsg interface {
	Section() SectionID
}

// Section is one top-level view. It is a real Bubble Tea component: it owns its
// state, reacts to messages, and renders into the body region the App lays out.
// The App passes the shared *ProgramContext into Init and the Section keeps the
// pointer, so layout and theme changes are observed on the next View without
// any prop-drilling.
type Section interface {
	// ID is the stable identifier used for tab routing and the registry.
	ID() SectionID
	// Title is the human label shown in the tab bar.
	Title() string
	// Init wires the shared context and returns any startup command
	// (typically the first data fetch).
	Init(ctx *ProgramContext) tea.Cmd
	// Update handles a message and returns the (possibly new) Section value
	// plus a command. Value semantics keep Sections cheap to copy and easy
	// to test.
	Update(msg tea.Msg) (Section, tea.Cmd)
	// View renders the section body. The App owns the surrounding chrome
	// (tab bar, footer) and the sidebar split lives in ProgramContext.
	View() string
	// HelpBindings returns the bindings shown in the contextual help bar for
	// this section in its current state.
	HelpBindings() []key.Binding
	// CapturesInput reports whether the section is currently consuming raw key
	// input (e.g. a filter or editor is focused). When true, the App routes
	// keys to the section before applying global shortcuts, so typing a query
	// containing "q" or "tab" does not quit or switch views.
	CapturesInput() bool
}
