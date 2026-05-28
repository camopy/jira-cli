package cli_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli"
)

// Machine JSON is one envelope per line so agent and log consumers can read
// it line-by-line. WriteEnvelope must emit a single line (plus the trailing
// newline), never indented multi-line JSON.
func TestWriteEnvelopeEmitsSingleLineJSON(t *testing.T) {
	var buf bytes.Buffer
	env := cli.Envelope{
		OK:       true,
		Meta:     cli.Meta{Command: "issue.view"},
		Data:     map[string]any{"key": "ABC-1"},
		Errors:   []cli.Error{},
		Warnings: []cli.Warning{},
	}
	if err := cli.WriteEnvelope(&buf, env); err != nil {
		t.Fatalf("WriteEnvelope: %v", err)
	}
	body := strings.TrimRight(buf.String(), "\n")
	if strings.Contains(body, "\n") {
		t.Fatalf("envelope JSON must be single-line, got:\n%s", buf.String())
	}
}

func TestWriteHumanJSONEmitsPrettyJSON(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]any{
		"ok":   true,
		"data": map[string]any{"key": "ABC-1"},
	}
	if err := cli.WriteHumanJSON(&buf, data); err != nil {
		t.Fatalf("WriteHumanJSON: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "\n  \"") {
		t.Fatalf("human JSON must be pretty-printed, got:\n%s", got)
	}
	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("human JSON must remain valid JSON: %v\n%s", err, got)
	}
}
