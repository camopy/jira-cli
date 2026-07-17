package core

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	pkey "github.com/gechr/primer/key"

	"github.com/matcra587/jira-cli/internal/tui/components/activity"
	"github.com/matcra587/jira-cli/internal/tui/components/dialog"
	"github.com/matcra587/jira-cli/internal/tui/theme"
)

// logDialog is the activity registry's operation log as a stack dialog: every
// recorded mutation, newest first, with its status glyph and a hyperlinked
// issue key. Like the help sheet it closes on any key or click; the Shell
// frames and (when it overflows) scrolls it, so the newest entries — the ones
// that matter — always sit at the top.
type logDialog struct {
	entries []activity.Entry
	baseURL string
}

// newLogDialog snapshots the log at open time. The dialog is momentary (it
// closes on the next key), so a live view would add nothing.
func (a App) newLogDialog() logDialog {
	return logDialog{entries: a.ctx.Activity.Log(), baseURL: a.ctx.BaseURL}
}

// Title captions the overlay.
func (d logDialog) Title() string { return "Activity log" }

// Update closes the log on the first key or mouse click, matching the help
// sheet's any-key dismissal.
func (d logDialog) Update(msg tea.Msg) (dialog.Dialog, tea.Cmd, dialog.Result) {
	return d, nil, dialog.DismissResult(msg)
}

// Content renders the log lines, or a muted placeholder when nothing has been
// recorded yet.
func (d logDialog) Content(int) string {
	if len(d.entries) == 0 {
		return theme.HelpDesc.Render("No activity yet.")
	}
	var b strings.Builder
	for i, e := range d.entries {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(d.line(e))
	}
	return b.String()
}

// line renders one log entry: a status glyph and the pending/resolved text,
// with the issue key hyperlinked (OSC-8) when the entry names one.
func (d logDialog) line(e activity.Entry) string {
	var glyph, text string
	switch e.Status {
	case activity.StatusPending:
		glyph, text = theme.StatusInProgress.Render("•"), e.Pending+"…"
	case activity.StatusDone:
		glyph, text = theme.StatusDone.Render("✓"), e.Done
	case activity.StatusFailed:
		msg := e.Pending + " failed"
		if e.Err != nil {
			msg = e.Pending + " failed: " + e.Err.Error()
		}
		glyph, text = theme.StatusErr.Render("✗"), msg
	}
	return glyph + " " + linkIssueKey(text, e.IssueKey, d.baseURL)
}

// Hints shows the dismissal affordance.
func (d logDialog) Hints() []pkey.Hint { return []pkey.Hint{{Key: "esc", Desc: "close"}} }
