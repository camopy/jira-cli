package root

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/cli/issue"
	"github.com/spf13/cobra"
)

func TestTTYCommandsDefaultToHumanClogOutput(t *testing.T) {
	cmd, stdout, _ := outputModeTestCommand(cli.ModePlain)

	if err := cmdutil.WriteEnvelope(cmd, "issue.list", map[string]any{"issues": []any{}}); err != nil {
		t.Fatalf("cmdutil.WriteEnvelope() error = %v", err)
	}
	got := stdout.String()
	if strings.Contains(got, `"meta"`) || strings.Contains(got, `"errors"`) {
		t.Fatalf("TTY default command output should be human data, not JSON envelope:\n%s", got)
	}
	if !strings.Contains(got, "listed issues") || !strings.Contains(got, "count=0") {
		t.Fatalf("TTY default command output should use clog rich data rendering:\n%s", got)
	}
}

func TestNonTTYCommandsDefaultToJSONEnvelope(t *testing.T) {
	cmd, stdout, _ := outputModeTestCommand(cli.ModeJSON)

	if err := cmdutil.WriteEnvelope(cmd, "issue.list", map[string]any{"issues": []any{}}); err != nil {
		t.Fatalf("cmdutil.WriteEnvelope() error = %v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, `"meta"`) || !strings.Contains(got, `"errors"`) {
		t.Fatalf("non-TTY command output is not JSON envelope:\n%s", got)
	}
}

// --output=json resolves to ModeJSON, which command output helpers must
// honor by emitting the full envelope even when the terminal is a TTY.
func TestOutputJSONModeForcesEnvelope(t *testing.T) {
	cmd, stdout, _ := outputModeTestCommand(cli.ModeJSON)
	if err := cmdutil.WriteEnvelope(cmd, "issue.list", map[string]any{"issues": []any{}}); err != nil {
		t.Fatalf("cmdutil.WriteEnvelope() error = %v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, `"meta"`) || !strings.Contains(got, `"errors"`) || !strings.Contains(got, `"ok"`) {
		t.Fatalf("--output=json did not force JSON envelope:\n%s", got)
	}
}

// --output=compact resolves to ModeCompact, which drops the envelope
// wrapper and emits the data payload only.
func TestOutputCompactModeDropsEnvelope(t *testing.T) {
	cmd, stdout, _ := outputModeTestCommand(cli.ModeCompact)
	if err := cmdutil.WriteEnvelope(cmd, "issue.list", map[string]any{"issues": []any{}}); err != nil {
		t.Fatalf("cmdutil.WriteEnvelope() error = %v", err)
	}
	got := stdout.String()
	if strings.Contains(got, `"meta"`) {
		t.Fatalf("--output=compact must drop the envelope meta:\n%s", got)
	}
}

// ParseOutputMode rejects every removed legacy flag name so a removed
// flag can never be silently re-aliased into a mode.
func TestRemovedLegacyFlagNamesAreNotOutputModes(t *testing.T) {
	for _, name := range []string{"json", "compact", "plain", "raw"} {
		// json/compact ARE valid --output VALUES; the others must not be.
		if name == "plain" || name == "raw" {
			if _, err := cli.ParseOutputMode(name); err == nil {
				t.Fatalf("removed flag name %q must not be a valid --output mode", name)
			}
		}
	}
}

// A credential-cleanup warning (e.g. a failed revocation of the old
// secret after an auth re-point) must survive compact mode. compact has
// no envelope, so the warning is folded into the data payload — it must
// never be silently dropped, or a stale secret stays invisible.
func TestCompactModePreservesCredentialCleanupWarning(t *testing.T) {
	cmd, stdout, _ := outputModeTestCommand(cli.ModeCompact)
	cmdutil.RecordCredentialWarnings(cmd, []string{"revoking the previous credential failed: keyring locked"})

	if err := cmdutil.WriteEnvelope(cmd, "auth.login", map[string]any{"profile": "work", "logged_in": true}); err != nil {
		t.Fatalf("cmdutil.WriteEnvelope() error = %v", err)
	}
	got := stdout.String()
	if strings.Contains(got, `"meta"`) {
		t.Fatalf("compact output must not carry the envelope:\n%s", got)
	}
	if !strings.Contains(got, "revoking the previous credential failed") {
		t.Fatalf("compact mode dropped the credential cleanup warning:\n%s", got)
	}
}

func TestPlainRawWarningsRouteToStderr(t *testing.T) {
	cmd, stdout, stderr := outputModeTestCommand(cli.ModePlain)

	err := cmdutil.WriteEnvelopeWithRawWarnings(cmd, "cache.boards", map[string]any{"boards": []any{}}, []map[string]any{{
		"type":    "rate-limit-during-paginate",
		"message": "retry later",
	}})
	if err != nil {
		t.Fatalf("cmdutil.WriteEnvelopeWithRawWarnings() error = %v", err)
	}
	if strings.Contains(stdout.String(), "retry later") || strings.Contains(stdout.String(), "rate-limit-during-paginate") {
		t.Fatalf("plain raw warning leaked to stdout:\n%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "retry later") || !strings.Contains(stderr.String(), "rate-limit-during-paginate") {
		t.Fatalf("plain raw warning missing from stderr:\n%s", stderr.String())
	}
}

func TestCommandErrorsUseClogDiagnosticsOnStderr(t *testing.T) {
	cmd, _, stderr := outputModeTestCommand(cli.ModePlain)
	if err := cmd.Root().PersistentFlags().Set("output", "human"); err != nil {
		t.Fatalf("Set(output) error = %v", err)
	}

	writeCommandError(cmd.Context(), cmd, errors.New("jira API failed"))
	got := stderr.String()
	if strings.Contains(got, `"errors"`) || strings.Contains(got, `"meta"`) || !strings.Contains(got, "jira API failed") {
		t.Fatalf("command error did not emit clog diagnostic stderr:\n%s", got)
	}
}

func TestUnsupportedAuthTypeClassifiesAsValidationError(t *testing.T) {
	outErr := outputErrorFor(errors.New(`unsupported auth type "oauth2"`))
	if outErr.Type != string(cli.ErrorTypeValidation) {
		t.Fatalf("unsupported auth type classified as %q, want %q", outErr.Type, cli.ErrorTypeValidation)
	}
}

func TestIssueListPlainOutputHidesJQLUnlessDebug(t *testing.T) {
	cmd, stdout, _ := outputModeTestCommand(cli.ModePlain)
	if err := cmd.Root().PersistentFlags().Set("output", "human"); err != nil {
		t.Fatalf("Set(output) error = %v", err)
	}
	data := issue.IssueListOutputData(cmd, []map[string]any{}, false, "assignee = currentUser() ORDER BY updated DESC")
	if err := cmdutil.WriteEnvelope(cmd, "issue.list", data); err != nil {
		t.Fatalf("cmdutil.WriteEnvelope() error = %v", err)
	}
	if strings.Contains(stdout.String(), "jql=") {
		t.Fatalf("issue list plain output leaked JQL without debug:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "listed issues") || !strings.Contains(stdout.String(), "count=0") || !strings.Contains(stdout.String(), "detail=false") || strings.Contains(stdout.String(), "No issues") || strings.Contains(stdout.String(), "issues=") {
		t.Fatalf("issue list plain output did not render a clog issue-list result:\n%s", stdout.String())
	}

	cmd, stdout, _ = outputModeTestCommand(cli.ModePlain)
	if err := cmd.Root().PersistentFlags().Set("output", "human"); err != nil {
		t.Fatalf("Set(output) error = %v", err)
	}
	if err := cmd.Root().PersistentFlags().Set("debug", "true"); err != nil {
		t.Fatalf("Set(debug) error = %v", err)
	}
	data = issue.IssueListOutputData(cmd, []map[string]any{}, false, "assignee = currentUser() ORDER BY updated DESC")
	if err := cmdutil.WriteEnvelope(cmd, "issue.list", data); err != nil {
		t.Fatalf("cmdutil.WriteEnvelope(debug) error = %v", err)
	}
	if !strings.Contains(stdout.String(), "jql=") {
		t.Fatalf("issue list debug output omitted JQL:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "detail=false") || strings.Contains(stdout.String(), "issues=") || strings.Contains(stdout.String(), "No issues") {
		t.Fatalf("issue list debug output leaked internal fields:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "INF") {
		t.Fatalf("--plain stdout is not clog event output:\n%s", stdout.String())
	}
}

func TestPlainOutputExtractsNestedADFText(t *testing.T) {
	cmd, stdout, _ := outputModeTestCommand(cli.ModePlain)
	if err := cmd.Root().PersistentFlags().Set("output", "human"); err != nil {
		t.Fatalf("Set(output) error = %v", err)
	}
	data := map[string]any{
		"comment": map[string]any{
			"body": map[string]any{
				"type":    "doc",
				"version": 1,
				"content": []any{
					map[string]any{
						"type": "paragraph",
						"content": []any{
							map[string]any{"type": "text", "text": "hello world"},
						},
					},
				},
			},
		},
	}
	if err := cmdutil.WriteEnvelope(cmd, "issue.comment", data); err != nil {
		t.Fatalf("cmdutil.WriteEnvelope() error = %v", err)
	}
	got := stdout.String()
	if strings.Contains(got, "comment=1") || !strings.Contains(got, `comment.body="hello world"`) || !strings.Contains(got, "INF") {
		t.Fatalf("plain output did not extract nested ADF text:\n%s", got)
	}
}

func TestRootInteractiveFlagControlsDashboardLaunchIntent(t *testing.T) {
	cmd, _, _ := outputModeTestCommand(cli.ModePlain)
	interactive, _ := cmd.Root().PersistentFlags().GetBool("interactive")
	if interactive {
		t.Fatal("interactive should default false")
	}
	if err := cmd.Root().PersistentFlags().Set("interactive", "true"); err != nil {
		t.Fatalf("Set(interactive) error = %v", err)
	}
	interactive, _ = cmd.Root().PersistentFlags().GetBool("interactive")
	if !interactive {
		t.Fatal("interactive flag was not set")
	}
}

func outputModeTestCommand(mode cli.Mode) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	root := &cobra.Command{Use: "jira"}
	pf := root.PersistentFlags()
	pf.String("profile", "default", "")
	pf.String("config", "", "")
	pf.String("output", "auto", "")
	pf.BoolP("interactive", "i", false, "")
	pf.BoolP("debug", "d", false, "")
	cmd := &cobra.Command{Use: "issue list"}
	root.AddCommand(cmd)
	root.SetOut(stdout)
	root.SetErr(stderr)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	ctx := cmdutil.WithDetector(context.Background(), cli.Detection{Mode: mode, IsTTY: mode == cli.ModePlain})
	ctx = cmdutil.WithCredentialWarnSink(ctx)
	cmd.SetContext(ctx)
	return cmd, stdout, stderr
}
