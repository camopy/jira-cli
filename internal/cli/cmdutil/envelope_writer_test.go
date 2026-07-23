package cmdutil_test

import (
	"context"
	"errors"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/spf13/cobra"
)

var errEnvelopeWrite = errors.New("envelope write")

type failingEnvelopeWriter struct{}

func (failingEnvelopeWriter) Write([]byte) (int, error) {
	return 0, errEnvelopeWrite
}

func TestWriteEnvelopeReturnsWriterFailure(t *testing.T) {
	cmd := &cobra.Command{Use: "x"}
	cmd.SetContext(cmdutil.WithDetector(context.Background(), cli.Detection{Mode: cli.ModeJSON}))
	cmd.SetOut(failingEnvelopeWriter{})

	err := cmdutil.WriteEnvelope(cmd, "version", map[string]any{"version": "test"})
	if !errors.Is(err, errEnvelopeWrite) {
		t.Fatalf("WriteEnvelope() error = %v, want writer failure", err)
	}
}
