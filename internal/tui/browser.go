package tui

import (
	"context"
	"os/exec"
	"runtime"
	"time"

	tea "charm.land/bubbletea/v2"
)

// browserOpenedMsg is emitted after a browser-open attempt completes.
type browserOpenedMsg struct {
	url string
	err error
}

// openBrowserCmd returns a tea.Cmd that opens the given URL in the OS
// default browser. Times out after 10s. Mirrors pdc/internal/browser.
func openBrowserCmd(parent context.Context, url string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 10*time.Second)
		defer cancel()
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.CommandContext(ctx, "open", url) //nolint:gosec // URL validated by caller
		case "windows":
			cmd = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", url) //nolint:gosec // URL validated by caller
		default:
			cmd = exec.CommandContext(ctx, "xdg-open", url) //nolint:gosec // URL validated by caller
		}
		err := cmd.Run()
		return browserOpenedMsg{url: url, err: err}
	}
}
