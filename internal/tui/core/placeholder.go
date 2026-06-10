package core

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

var _ Section = (*placeholderSection)(nil)

// placeholderSection is a minimal Section used to prove the App orchestration
// before the real Issues and Search sections land, and as a stand-in for views
// (boards, epics, worklogs) that are registered but not yet built.
type placeholderSection struct {
	id    SectionID
	title string
	ctx   *ProgramContext
}

// NewPlaceholderSection returns a factory for a do-nothing section with the
// given id and title.
func NewPlaceholderSection(id SectionID, title string) SectionFactory {
	return func(ctx *ProgramContext) Section {
		return &placeholderSection{id: id, title: title, ctx: ctx}
	}
}

func (s *placeholderSection) ID() SectionID { return s.id }
func (s *placeholderSection) Title() string { return s.title }

func (s *placeholderSection) Init(ctx *ProgramContext) tea.Cmd {
	s.ctx = ctx
	return nil
}

func (s *placeholderSection) Update(tea.Msg) (Section, tea.Cmd) { return s, nil }

func (s *placeholderSection) View() string {
	return s.title + " — coming soon"
}

func (s *placeholderSection) HelpBindings() []key.Binding { return nil }
func (s *placeholderSection) CapturesInput() bool         { return false }
