package input

import (
	"errors"
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// EditorCommand returns the configured external editor: $JIRA_EDITOR wins,
// $EDITOR is the fallback, "" means none — multiline fields then use the
// in-TUI textarea instead. One rule, no per-field knobs.
func EditorCommand() string {
	if ed := os.Getenv("JIRA_EDITOR"); ed != "" {
		return ed
	}
	return os.Getenv("EDITOR")
}

// EditorFinishedMsg delivers the text written in the external editor. ID is
// the opener's routing tag (e.g. "comment:JCT-12") so the consumer knows which
// flow to resume. Err is set when the editor could not run or the buffer could
// not be read back.
type EditorFinishedMsg struct {
	ID   string
	Text string
	Err  error
}

// Edit suspends the TUI and opens the external editor on a temp file seeded
// with initial; the result returns as an EditorFinishedMsg. The editor value
// runs through the shell so commands with flags ("nvim -f") work.
func Edit(id, initial string) tea.Cmd {
	ed := EditorCommand()
	if ed == "" {
		return func() tea.Msg {
			return EditorFinishedMsg{ID: id, Err: errors.New("no JIRA_EDITOR or EDITOR configured")}
		}
	}
	f, err := os.CreateTemp("", "jira-edit-*.md")
	if err != nil {
		return func() tea.Msg { return EditorFinishedMsg{ID: id, Err: err} }
	}
	path := f.Name()
	if _, err := f.WriteString(initial); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return func() tea.Msg { return EditorFinishedMsg{ID: id, Err: err} }
	}
	_ = f.Close()

	// ed is deliberately unquoted so values with flags ("nvim -f", "code -w")
	// work; the trade-off is that an editor binary living at a path with
	// spaces must be wrapped in quotes inside the variable itself, same as git.
	cmd := exec.Command("sh", "-c", ed+" "+shellQuote(path)) //nolint:gosec // running the user's own $EDITOR is the feature; path is our quoted temp file
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		defer func() { _ = os.Remove(path) }()
		if err != nil {
			return EditorFinishedMsg{ID: id, Err: err}
		}
		b, rerr := os.ReadFile(path) //nolint:gosec // path is the temp file created above, not user input
		if rerr != nil {
			return EditorFinishedMsg{ID: id, Err: rerr}
		}
		return EditorFinishedMsg{ID: id, Text: strings.TrimRight(string(b), "\n")}
	})
}

// shellQuote single-quotes s for sh -c, escaping embedded quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
