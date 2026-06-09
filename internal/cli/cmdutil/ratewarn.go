package cmdutil

import (
	"context"
	"sync"

	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/spf13/cobra"
)

// rateWarnSink is a per-command collector for the "approaching Jira's rate
// limit" notice. It is installed into the command context by PersistentPreRunE
// so each invocation owns a fresh sink, and it dedups to a single warning no
// matter how many near-limit responses a fan-out produces.
type rateWarnSink struct {
	mu      sync.Mutex
	tripped bool
	reason  string
}

func (s *rateWarnSink) trip(reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// First near-limit response wins the reason; later ones only confirm the
	// state already recorded, so the warning stays a single line.
	if !s.tripped {
		s.tripped = true
		s.reason = reason
	}
}

func (s *rateWarnSink) collected() (bool, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tripped, s.reason
}

// WithRateWarnSink returns a context carrying a fresh rate-limit-warning sink.
// PersistentPreRunE installs one per command invocation.
func WithRateWarnSink(ctx context.Context) context.Context {
	return context.WithValue(ctx, rateWarnSinkKey, &rateWarnSink{})
}

// RecordRateNearLimit is the jira.RateObserver the CLI installs on the client.
// It runs on each successful response — possibly concurrently under -p — and
// trips the command's sink when Jira flags the response as near the limit.
// Calls without a sink (direct test clients) drop the signal silently.
func RecordRateNearLimit(ctx context.Context, rate jira.Rate) {
	if !rate.NearLimit {
		return
	}
	if sink, ok := ctx.Value(rateWarnSinkKey).(*rateWarnSink); ok {
		sink.trip(rate.Reason)
	}
}

// collectedRateWarnings returns the near-limit warning recorded for the current
// command, or nil. The sink is per-command, so this never leaks across runs.
func collectedRateWarnings(cmd *cobra.Command) []cli.Warning {
	if cmd == nil {
		return nil
	}
	sink, ok := cmd.Context().Value(rateWarnSinkKey).(*rateWarnSink)
	if !ok {
		return nil
	}
	tripped, reason := sink.collected()
	if !tripped {
		return nil
	}
	msg := "Approaching Jira's rate limit; requests may soon be throttled. " +
		"Slow down or lower --parallelism."
	if reason != "" {
		msg = "Approaching Jira's rate limit (" + reason + "); requests may soon be " +
			"throttled. Slow down or lower --parallelism."
	}
	return []cli.Warning{{Type: "rate_limit_near", Message: msg, Lossy: false}}
}

// collectedCommandWarnings gathers every per-command diagnostic the envelope
// should carry — credential notices and the rate near-limit notice. One drain
// point keeps the envelope writers from each having to remember every sink.
func collectedCommandWarnings(cmd *cobra.Command) []cli.Warning {
	return append(collectedCredentialWarnings(cmd), collectedRateWarnings(cmd)...)
}
