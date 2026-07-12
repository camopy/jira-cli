package pager

import (
	"os"
	"os/exec"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/gechr/x/shell"
	"github.com/gechr/x/terminal"
)

// Overflows reports whether content is taller than the terminal f writes
// to. A non-terminal (or size probe failure) reports false, so callers that
// gate paging on it degrade to plain streaming.
func Overflows(f *os.File, content string) bool {
	height := terminal.Height(f)
	if height <= 0 {
		return false
	}
	return strings.Count(content, "\n")+1 > height
}

// Run pages content on the controlling terminal. An explicit JIRA_PAGER
// (then PAGER) environment value runs as an external pager with the content
// on stdin — users who want less get less — and an empty value or lookup
// failure falls back to the built-in viewport. The variables are split with
// POSIX display semantics so "less -R" style values work; they are the
// user's own local configuration, not remote input.
func Run(content string) error {
	for _, env := range []string{"JIRA_PAGER", "PAGER"} {
		cmdline := strings.TrimSpace(os.Getenv(env))
		if cmdline == "" {
			continue
		}
		parts, err := shell.Split(cmdline)
		if err != nil || len(parts) == 0 {
			continue
		}
		cmd := exec.Command(parts[0], parts[1:]...) //nolint:gosec // the user's own PAGER configuration, same trust as $EDITOR
		cmd.Stdin = strings.NewReader(content)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		// A pager that cannot START (missing binary, bad path) must not eat
		// the document — fall through to the built-in viewport. One that
		// started and then failed keeps its error, git-style: the user's
		// own pager broke mid-run and hiding that would be worse.
		if err := cmd.Start(); err != nil {
			continue
		}
		return cmd.Wait()
	}
	return runBuiltin(content)
}

// runBuiltin pages content with a minimal alt-screen viewport: the same
// bubbletea lineage as the glamour rendering that produced the content, so
// no extra dependency. Keys follow pager convention — arrows and page keys
// scroll, g/G jump, q/esc/ctrl+c quit and restore the screen.
func runBuiltin(content string) error {
	m := model{content: content}
	_, err := tea.NewProgram(&m).Run()
	return err
}

type model struct {
	content string
	view    viewport.Model
	ready   bool
}

func (m *model) Init() tea.Cmd { return nil }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Reserve the last row for the status footer.
		if !m.ready {
			m.view = viewport.New(viewport.WithWidth(msg.Width), viewport.WithHeight(msg.Height-1))
			m.view.SetContent(m.content)
			m.ready = true
		} else {
			m.view.SetWidth(msg.Width)
			m.view.SetHeight(msg.Height - 1)
		}
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "g", "home":
			m.view.GotoTop()
			return m, nil
		case "G", "end":
			m.view.GotoBottom()
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.view, cmd = m.view.Update(msg)
	return m, cmd
}

func (m *model) View() tea.View {
	if !m.ready {
		return tea.NewView("")
	}
	v := tea.NewView(m.view.View() + "\n(q to quit)")
	// Alt screen, like less: the document scrolls in its own screen and the
	// shell's scrollback is restored intact on quit.
	v.AltScreen = true
	return v
}
