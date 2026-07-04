package cli

import (
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"
	"github.com/gechr/clog"
)

// clog's default backtick rendering styles a `code` span by dropping the
// two delimiter characters — two visible cells gone per span. Plain
// renderers emit grid-aligned rows, so their loggers select BacktickKeep,
// which styles the span at exactly the written width. This test pins both
// sides: the default DOES shrink a padded row (the failure mode this
// guards against), and the plain-logger styles do not.
func TestPlainLoggerKeepsBacktickedRowsWidthStable(t *testing.T) {
	// A grid-style row: padded columns, a code span in the middle one.
	const row = "KEY-1    fix the `cmd` runner      Done   "
	want := xansi.WcWidth.StringWidth(row)

	render := func(logger *clog.Logger, buf *strings.Builder) string {
		buf.Reset()
		logger.Info().Parts(clog.PartMessage).Msg(row)
		return strings.TrimRight(buf.String(), "\n")
	}

	var buf strings.Builder
	// ColorAlways forces the styled message path a TTY would take.
	defaultLogger := clog.New(clog.NewOutput(&buf, clog.ColorAlways))
	if got := xansi.WcWidth.StringWidth(render(defaultLogger, &buf)); got == want {
		t.Log("clog's default styles no longer shrink backticked messages — " +
			"BacktickKeep in plainLoggerStyles may be redundant now")
	} else if got != want-2 {
		t.Errorf("expected the default styles to shrink the row by exactly the two delimiters, got %d vs %d", got, want)
	}

	plainLogger := clog.New(clog.NewOutput(&buf, clog.ColorAlways))
	plainLogger.SetStyles(plainLoggerStyles())
	rendered := render(plainLogger, &buf)
	if got := xansi.WcWidth.StringWidth(rendered); got != want {
		t.Errorf("plain logger changed the row width: %d, want %d (row %q)", got, want, xansi.Strip(rendered))
	}
	if !strings.Contains(xansi.Strip(rendered), "`cmd`") {
		t.Errorf("keep mode should preserve the literal backticks, got %q", xansi.Strip(rendered))
	}
}
