package components

import (
	"github.com/gechr/primer/helpbar"
	"github.com/gechr/primer/key"
)

type Footer struct {
	Help helpbar.Model
}

func NewFooter() Footer {
	return Footer{Help: helpbar.Model{Hints: []key.Hint{
		{Key: "j/k", Desc: "navigate"},
		{Key: "/", Desc: "search"},
		{Key: "e", Desc: "edit"},
		{Key: "m", Desc: "move"},
		{Key: "q", Desc: "quit"},
	}}}
}

func (f Footer) Render() string {
	return f.Help.Render()
}
