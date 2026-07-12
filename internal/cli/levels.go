package cli

import (
	"charm.land/lipgloss/v2"
	"github.com/gechr/clog"
)

// LevelSuccess is the custom clog level for completion lines of Jira
// mutations that actually succeeded — visually distinct from the INF lines
// informational reads emit, so a scan of scrollback separates "changed
// something" from "looked at something". It sits between Dry (2) and Warn
// (5), mirroring clog's own custom-level example, and registration makes it
// round-trip through ParseLevel/MarshalText so level filtering keeps
// working.
const LevelSuccess clog.Level = clog.LevelDry + 1

func init() {
	clog.RegisterLevel(LevelSuccess, clog.LevelConfig{
		Name:   "success",
		Label:  "SCS",
		Symbol: "✅",
		Style:  new(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("2"))),
	})
}
