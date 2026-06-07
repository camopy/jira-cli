package cmdutil

import (
	"context"
	"errors"
	"time"

	"github.com/gechr/clog"
	"github.com/spf13/cobra"

	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/jira"
)

// Spin runs a blocking network or disk operation while giving the user
// feedback: a present-progressive spinner on a TTY ("Caching boards") and a
// structured debug lifecycle that records the elapsed time and any failure
// reason as fields rather than baking them into prose:
//
//	DBG Caching boards
//	DBG Cached boards time=320ms
//	DBG Failed to cache boards time=110ms status=403 reason="403 Forbidden"
//
// The spinner mirrors the auth-login gating: NonTTYSilent suppresses it whenever
// stderr is not a terminal (piped, redirected, or an agent capturing output),
// and clog writes to stderr, so machine output on stdout stays clean even under
// --output=json.
//
// Under --debug the spinner is suppressed entirely: the debug lifecycle below
// already narrates progress, and an animated spinner sharing stderr with the
// verbose request/response logging only corrupts both — each redraw frame
// strands onto its own line between the debug records. Verbose narration and
// the spinner are alternatives, never shown together.
func Spin(cmd *cobra.Command, op string, fn func(context.Context) error) error {
	verb := cli.VerbFor(op)
	logger := clog.Ctx(cmd.Context())
	logger.Debug().Msg(verb.Gerundf())

	start := time.Now()
	var err error
	if clog.IsVerbose() {
		err = fn(cmd.Context())
	} else {
		// The spinner label is user-facing UI, so it is Sentence-cased; the
		// debug lifecycle above/below stays lower case as a structured log.
		err = clog.Spinner(cli.SentenceCase(verb.Gerundf())).
			NonTTYSilent(true).
			Wait(cmd.Context(), fn).
			Silent()
	}
	elapsed := time.Since(start)

	if err != nil {
		event := logger.Debug().Duration("time", elapsed)
		// Surface the HTTP status as its own field rather than burying it in
		// the reason string, so failures stay greppable (status=403).
		var apiErr *jira.APIError
		if errors.As(err, &apiErr) {
			event = event.Int("status", apiErr.StatusCode)
		}
		event.AnErr("reason", err).Msg(verb.Failuref())
		return err
	}
	logger.Debug().Duration("time", elapsed).Msg(verb.Pastf())
	return nil
}
