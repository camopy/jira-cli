package cmdutil

import (
	"context"
	"sync"
	"time"

	"github.com/spf13/cobra"
)

type apiElapsedKeyType struct{}

var apiElapsedKey apiElapsedKeyType

// apiElapsedSink accumulates the wall time a command spent inside blocking
// operations. Writers are sequential today — Spin phases run one after
// another, and the fanout executor records once for the whole batch (a
// per-key fn must never Spin; the fanout owns that lifecycle, and a nested
// Spin would count the same wall time twice). The mutex is defensive for
// future callers; the total is only read after the work completes, when the
// envelope renders.
type apiElapsedSink struct {
	mu    sync.Mutex
	total time.Duration
}

func (s *apiElapsedSink) add(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.total += d
}

func (s *apiElapsedSink) sum() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.total
}

// WithAPIElapsedSink returns a context carrying a fresh elapsed-time sink.
// PersistentPreRunE installs one per command invocation, so the total never
// leaks across runs.
func WithAPIElapsedSink(ctx context.Context) context.Context {
	return context.WithValue(ctx, apiElapsedKey, &apiElapsedSink{})
}

// recordAPIElapsed adds one blocking operation's wall time to the command's
// sink. Calls without a sink (direct test invocations) drop the signal
// silently, matching the rate-warning sink's behavior.
func recordAPIElapsed(ctx context.Context, d time.Duration) {
	if sink, ok := ctx.Value(apiElapsedKey).(*apiElapsedSink); ok {
		sink.add(d)
	}
}

// APIElapsedFor returns the total blocking time recorded for the current
// command, or zero when nothing blocking ran (local-only commands, dry-run
// previews that never spun, tests without a sink).
func APIElapsedFor(cmd *cobra.Command) time.Duration {
	if cmd == nil {
		return 0
	}
	sink, ok := cmd.Context().Value(apiElapsedKey).(*apiElapsedSink)
	if !ok {
		return 0
	}
	return sink.sum()
}
