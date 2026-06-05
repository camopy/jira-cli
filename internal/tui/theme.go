package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	clibtheme "github.com/gechr/clib/theme"
	"github.com/gechr/primer/key"
)

var baseTheme = clibtheme.Dark()

func ResolveTheme(name string) *clibtheme.Theme {
	if strings.TrimSpace(name) == "" {
		return clibtheme.Dark()
	}
	var th clibtheme.Theme
	if err := th.UnmarshalText([]byte(name)); err != nil {
		return clibtheme.Dark()
	}
	return &th
}

func ApplyTheme(th *clibtheme.Theme) {
	if th == nil {
		th = clibtheme.Dark()
	}
	baseTheme = th
}

func DefaultHelpRenderer() key.Renderer {
	return key.Renderer{
		Styles: key.Styles{
			Key:  lipgloss.NewStyle().Foreground(baseTheme.Yellow.GetForeground()).Bold(true),
			Text: *baseTheme.Dim,
		},
		Inline: true,
	}
}

func RenderShell(width, height int, body, footer string) string {
	title := lipgloss.NewStyle().
		Foreground(baseTheme.Blue.GetForeground()).
		Bold(true).
		Render("jira")
	if width > 0 {
		body = lipgloss.NewStyle().Width(width).Render(body)
	}
	if height > 0 {
		return title + "\n" + body + "\n" + footer
	}
	return title + "\n" + body + "\n" + footer
}
