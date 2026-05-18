package cmdutil

import (
	"context"
	"slices"
	"sync"

	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/spf13/cobra"
)

// credentialWarnSink is a per-command collector for credential notices (a
// migration cleanup note, a kept user-named 1Password item on logout, an
// orphaned credential after a site change). It is installed into the command
// context by PersistentPreRunE, so each command invocation owns a fresh,
// isolated sink — a notice raised by one command can never reach another.
type credentialWarnSink struct {
	mu    sync.Mutex
	warns []string
}

func (s *credentialWarnSink) add(warns []string) {
	if len(warns) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, w := range warns {
		if !slices.Contains(s.warns, w) {
			s.warns = append(s.warns, w)
		}
	}
}

func (s *credentialWarnSink) collected() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.warns)
}

// WithCredentialWarnSink returns a context carrying a fresh credential-warning
// sink. PersistentPreRunE installs one per command invocation.
func WithCredentialWarnSink(ctx context.Context) context.Context {
	return context.WithValue(ctx, credentialWarnSinkKey, &credentialWarnSink{})
}

// RecordCredentialWarnings appends resolution warnings to the command's sink,
// if one is installed. Commands without a sink (direct test calls) drop the
// warnings silently — they are diagnostics, not results.
func RecordCredentialWarnings(cmd *cobra.Command, warns []string) {
	if len(warns) == 0 || cmd == nil {
		return
	}
	if sink, ok := cmd.Context().Value(credentialWarnSinkKey).(*credentialWarnSink); ok {
		sink.add(warns)
	}
}

// collectedCredentialWarnings returns the credential warnings recorded for the
// current command as envelope warnings. The sink is per-command, so this
// only ever returns warnings from the command currently executing.
func collectedCredentialWarnings(cmd *cobra.Command) []cli.Warning {
	if cmd == nil {
		return nil
	}
	sink, ok := cmd.Context().Value(credentialWarnSinkKey).(*credentialWarnSink)
	if !ok {
		return nil
	}
	return credentialWarningsToEnvelope(sink.collected())
}

// credentialWarningsToEnvelope renders credential notices — a migration
// cleanup note, a kept user-named 1Password item, an orphaned credential
// after a site change — as envelope warnings under one informational type.
func credentialWarningsToEnvelope(msgs []string) []cli.Warning {
	if len(msgs) == 0 {
		return nil
	}
	out := make([]cli.Warning, 0, len(msgs))
	for _, msg := range msgs {
		out = append(out, cli.Warning{
			Type:    "credential_notice",
			Message: msg,
			Lossy:   false,
		})
	}
	return out
}
