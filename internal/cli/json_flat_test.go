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

// Compact is the lean, token-economical view: null-valued keys are
// dropped recursively. Empty collections, false, and 0 stay (they carry
// meaning); numbers keep their type. json mode keeps the full schema.
func TestWriteCompactDropsNullsRecursively(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]any{
		"key":      "ABC-1",
		"assignee": nil,
		"priority": "Medium",
		"count":    0,
		"done":     false,
		"labels":   []any{},
		"meta":     map[string]any{},
		"parent":   map[string]any{"key": "ABC-9", "epic": nil},
		"rows":     []any{map[string]any{"id": "1", "owner": nil}},
	}
	if err := cli.WriteCompact(&buf, data); err != nil {
		t.Fatalf("WriteCompact: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("compact must remain valid JSON: %v\n%s", err, buf.String())
	}
	if _, ok := got["assignee"]; ok {
		t.Errorf("top-level null key not dropped: %s", buf.String())
	}
	if parent, _ := got["parent"].(map[string]any); parent != nil {
		if _, ok := parent["epic"]; ok {
			t.Errorf("nested null key not dropped: %s", buf.String())
		}
	}
	if rows, _ := got["rows"].([]any); len(rows) == 1 {
		if row, _ := rows[0].(map[string]any); row != nil {
			if _, ok := row["owner"]; ok {
				t.Errorf("null inside array element not dropped: %s", buf.String())
			}
		}
	}
	// 0, false, and empty collections are meaningful and must survive.
	for _, k := range []string{"count", "done", "labels", "meta"} {
		if _, ok := got[k]; !ok {
			t.Errorf("compact dropped a meaningful key %q: %s", k, buf.String())
		}
	}
	if n, ok := got["count"].(float64); !ok || n != 0 {
		t.Errorf("numeric value not preserved: %s", buf.String())
	}
}

func TestWriteEnvelopeKeepsNulls(t *testing.T) {
	var buf bytes.Buffer
	env := cli.Envelope{
		OK:       true,
		Meta:     cli.Meta{Command: "issue.list"},
		Data:     map[string]any{"key": "ABC-1", "assignee": nil},
		Errors:   []cli.Error{},
		Warnings: []cli.Warning{},
	}
	if err := cli.WriteEnvelope(&buf, env); err != nil {
		t.Fatalf("WriteEnvelope: %v", err)
	}
	if !strings.Contains(buf.String(), `"assignee":null`) {
		t.Fatalf("json envelope must keep the full schema incl nulls, got:\n%s", buf.String())
	}
}

func TestWriteHumanJSONEmitsPrettyJSON(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]any{
		"ok":   true,
		"data": map[string]any{"key": "ABC-1"},
	}
	if err := cli.WriteHumanJSON(&buf, data, nil); err != nil {
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
