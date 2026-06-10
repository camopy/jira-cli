package issue

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/spf13/cobra"
)

// newEditorTestCmd builds a cobra command wired the way issueEditWithEditor
// expects: persistent flags for profile/config/adf-mode resolution, a
// temp config pointing at the supplied httptest URL, and a detector
// context. The configured editor is "true" — a blocking no-op program
// that exits 0 without touching the buffer, i.e. the canonical "user
// opened the editor and saved without changes" path.
func newEditorTestCmd(t *testing.T, srvURL string) (*cobra.Command, *bytes.Buffer) {
	t.Helper()

	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	cfg := `default_profile = "default"
editor = "true"

[[profiles]]
name = "default"
base_url = "` + srvURL + `"
auth_type = "token"
secret_backend = "keyring"
refresh_interval = 30
timeout = 30
workday_seconds = 28800
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("JIRA_TOKEN_DEFAULT", "test-token")
	// A blocking no-op "editor": a script that ignores its path argument
	// and exits 0 after a short sleep, leaving the buffer byte-identical.
	// The sleep clears the non-blocking-spawn safety threshold so the
	// no-op save is treated as a real (unchanged) edit. JIRA_EDITOR wins
	// over every other editor-resolution layer.
	editorScript := filepath.Join(t.TempDir(), "noop-editor.sh")
	if err := os.WriteFile(editorScript, []byte("#!/bin/sh\nsleep 0.6\n"), 0o700); err != nil {
		t.Fatalf("write editor script: %v", err)
	}
	t.Setenv("JIRA_EDITOR", editorScript)

	stdout := &bytes.Buffer{}
	// cmdutil.ConfigPath / cmdutil.RequestedProfile read cmd.Root().PersistentFlags(), so
	// these must be persistent flags on the root command.
	root := &cobra.Command{Use: "jira"}
	rpf := root.PersistentFlags()
	rpf.String("profile", "default", "")
	rpf.String("config", cfgPath, "")
	rpf.Bool("json", true, "")
	rpf.Bool("compact", false, "")
	rpf.Bool("plain", false, "")
	rpf.Bool("raw", false, "")
	rpf.Bool("debug", false, "")

	cmd := &cobra.Command{Use: "edit"}
	pf := cmd.Flags()
	pf.Bool("adf-strict", false, "")
	pf.Bool("adf-best-effort", false, "")
	root.AddCommand(cmd)
	root.SetOut(stdout)
	root.SetErr(stdout)
	cmd.SetOut(stdout)
	cmd.SetErr(stdout)
	cmd.SetContext(cmdutil.WithDetector(context.Background(), cli.Detection{Mode: cli.ModeJSON, IsTTY: false}))
	return cmd, stdout
}

// TestIssueEditEditorPreservesPanelAcrossNoOpSave is the command-level
// contract for the external-editor edit path: an existing Jira
// description that contains a panel and an inlineCard — neither of which
// has a Markdown representation — must survive a no-op editor save and
// still appear in the live update payload.
//
// Before the round-trip helper was wired in, issueEditWithEditor rendered
// the description through lossy adf.ToMarkdown before the user ever saw
// it, so the panel/inlineCard were erased before the buffer opened and a
// no-op save shipped a stripped document. This test fails against that
// behavior and passes once issueEditWithEditor routes through
// editor.RoundTripADF.
func TestIssueEditEditorPreservesPanelAcrossNoOpSave(t *testing.T) {
	const description = `{
		"type": "doc", "version": 1, "content": [
			{"type": "paragraph", "content": [{"type": "text", "text": "intro"}]},
			{"type": "panel", "attrs": {"panelType": "info"}, "content": [
				{"type": "paragraph", "content": [{"type": "text", "text": "panel body"}]}
			]},
			{"type": "paragraph", "content": [
				{"type": "text", "text": "ref "},
				{"type": "inlineCard", "attrs": {"url": "https://example.com/JCT-1"}}
			]}
		]
	}`

	var putBody []byte
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Logf("unmatched request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusTeapot)
	})
	mux.HandleFunc("GET /rest/api/3/issue/PROJ-1", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"1","key":"PROJ-1","fields":{"summary":"orig","description":`+description+`}}`)
	})
	mux.HandleFunc("PUT /rest/api/3/issue/PROJ-1", func(w http.ResponseWriter, r *http.Request) {
		putBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cmd, _ := newEditorTestCmd(t, srv.URL)

	if err := issueEditWithEditor(cmd, "PROJ-1", false); err != nil {
		t.Fatalf("issueEditWithEditor: %v", err)
	}

	if putBody == nil {
		t.Fatal("no PUT body captured — issue edit did not submit an update")
	}
	got := string(putBody)
	for _, marker := range []string{`"panel"`, `"panelType"`, "panel body", `"inlineCard"`, "https://example.com/JCT-1"} {
		if !strings.Contains(got, marker) {
			t.Fatalf("editor round-trip erased %q from the update payload — opaque ADF was not preserved: %s", marker, got)
		}
	}

	// The payload must still be a structurally valid ADF document.
	var body struct {
		Fields struct {
			Description map[string]any `json:"description"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(putBody, &body); err != nil {
		t.Fatalf("PUT body is not JSON: %v\n%s", err, got)
	}
	if body.Fields.Description["type"] != "doc" {
		t.Fatalf("submitted description is not an ADF doc: %#v", body.Fields.Description)
	}
}
