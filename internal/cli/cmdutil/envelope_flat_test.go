package cmdutil_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
)

// The raw-warnings envelope path must emit single-line JSON like the typed
// path, so agent and log consumers get one envelope per line regardless of
// which writer a command happens to use.
func TestWriteEnvelopeWithRawWarningsEmitsSingleLineJSON(t *testing.T) {
	var buf bytes.Buffer
	cmd := &cobra.Command{Use: "x"}
	cmd.SetContext(cmdutil.WithDetector(context.Background(), cli.Detection{Mode: cli.ModeJSON}))
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	warnings := []map[string]any{{"type": "cache_truncated", "message": "list truncated"}}
	if err := cmdutil.WriteEnvelopeWithRawWarnings(cmd, "issue.list", map[string]any{"count": 1}, warnings); err != nil {
		t.Fatalf("WriteEnvelopeWithRawWarnings: %v", err)
	}
	body := strings.TrimRight(buf.String(), "\n")
	if strings.Contains(body, "\n") {
		t.Fatalf("raw-warnings envelope must be single-line, got:\n%s", buf.String())
	}
}
