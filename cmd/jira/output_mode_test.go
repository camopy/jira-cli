package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/spf13/cobra"
)

func TestTTYCommandsDefaultToHumanClogOutput(t *testing.T) {
	cmd, stdout, _ := outputModeTestCommand(cli.ModePlain)

	if err := writeEnvelope(cmd, "issue.list", map[string]any{"issues": []any{}}); err != nil {
		t.Fatalf("writeEnvelope() error = %v", err)
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

	if err := writeEnvelope(cmd, "issue.list", map[string]any{"issues": []any{}}); err != nil {
		t.Fatalf("writeEnvelope() error = %v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, `"meta"`) || !strings.Contains(got, `"errors"`) {
		t.Fatalf("non-TTY command output is not JSON envelope:\n%s", got)
	}
}

func TestJSONFlagOverridesTTYHumanDefault(t *testing.T) {
	cmd, stdout, _ := outputModeTestCommand(cli.ModePlain)
	if err := cmd.Root().PersistentFlags().Set("json", "true"); err != nil {
		t.Fatalf("Set(json) error = %v", err)
	}

	if err := writeEnvelope(cmd, "issue.list", map[string]any{"issues": []any{}}); err != nil {
		t.Fatalf("writeEnvelope() error = %v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, `"meta"`) || !strings.Contains(got, `"errors"`) {
		t.Fatalf("--json did not force JSON envelope:\n%s", got)
	}
}

func TestCommandErrorsUseClogDiagnosticsOnStderr(t *testing.T) {
	cmd, _, stderr := outputModeTestCommand(cli.ModePlain)

	writeCommandError(cmd, errors.New("jira API failed"))
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
	if err := cmd.Root().PersistentFlags().Set("plain", "true"); err != nil {
		t.Fatalf("Set(plain) error = %v", err)
	}
	data := issueListOutputData(cmd, []map[string]any{}, false, "assignee = currentUser() ORDER BY updated DESC")
	if err := writeEnvelope(cmd, "issue.list", data); err != nil {
		t.Fatalf("writeEnvelope() error = %v", err)
	}
	if strings.Contains(stdout.String(), "jql=") {
		t.Fatalf("issue list plain output leaked JQL without debug:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "listed issues") || !strings.Contains(stdout.String(), "count=0") || !strings.Contains(stdout.String(), "detail=false") || strings.Contains(stdout.String(), "No issues") || strings.Contains(stdout.String(), "issues=") {
		t.Fatalf("issue list plain output did not render a clog issue-list result:\n%s", stdout.String())
	}

	cmd, stdout, _ = outputModeTestCommand(cli.ModePlain)
	if err := cmd.Root().PersistentFlags().Set("plain", "true"); err != nil {
		t.Fatalf("Set(plain) error = %v", err)
	}
	if err := cmd.Root().PersistentFlags().Set("debug", "true"); err != nil {
		t.Fatalf("Set(debug) error = %v", err)
	}
	data = issueListOutputData(cmd, []map[string]any{}, false, "assignee = currentUser() ORDER BY updated DESC")
	if err := writeEnvelope(cmd, "issue.list", data); err != nil {
		t.Fatalf("writeEnvelope(debug) error = %v", err)
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
	if err := cmd.Root().PersistentFlags().Set("plain", "true"); err != nil {
		t.Fatalf("Set(plain) error = %v", err)
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
	if err := writeEnvelope(cmd, "issue.comment", data); err != nil {
		t.Fatalf("writeEnvelope() error = %v", err)
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
	pf.Bool("json", false, "")
	pf.Bool("compact", false, "")
	pf.Bool("plain", false, "")
	pf.Bool("raw", false, "")
	pf.BoolP("interactive", "i", false, "")
	pf.BoolP("debug", "d", false, "")
	cmd := &cobra.Command{Use: "issue list"}
	root.AddCommand(cmd)
	root.SetOut(stdout)
	root.SetErr(stderr)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetContext(context.WithValue(context.Background(), detectorKey, cli.Detection{Mode: mode, IsTTY: mode == cli.ModePlain}))
	return cmd, stdout, stderr
}
