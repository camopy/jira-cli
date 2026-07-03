package contract

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// commentWithMention is a stored comment whose body exercises the nodes the
// old Markdown flattening destroyed: a mention (accountId), a status
// lozenge, and marked text.
const commentWithMention = `{
	"comments": [
		{"id":"200","body":{"type":"doc","version":1,"content":[{"type":"paragraph","content":[
			{"type":"mention","attrs":{"id":"557058:abc","text":"@Alice"}},
			{"type":"text","text":" please review "},
			{"type":"status","attrs":{"text":"BLOCKED","color":"red"}}
		]}]},"author":{"accountId":"u1","displayName":"Alice"},"created":"2026-04-01T10:00:00.000+0000","updated":"2026-04-01T10:00:00.000+0000"}
	],
	"startAt":0,"maxResults":50,"total":1,"isLast":true
}`

// comment list must return the body as native ADF — mention accountIds and
// every other node intact — matching what issue view already does. The old
// behavior flattened to Markdown, collapsing the mention to "@Alice".
func TestCommentListPreservesNativeADFBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/3/issue/PROJ-1/comment" {
			_, _ = w.Write([]byte(commentWithMention))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	cfg := jiraConfig(t, srv.URL)
	stdout, _, code := runJira(t, "--config", cfg, "issue", "comment", "list", "PROJ-1", "--output=json")
	if code != 0 {
		t.Fatalf("exit = %d; want 0\nstdout=%s", code, stdout)
	}
	body := commentBodyFromList(t, stdout)
	if body["type"] != "doc" {
		t.Fatalf("body.type = %v, want doc (native ADF, not a flattened string)", body["type"])
	}
	mention := findNodeOfType(body, "mention")
	if mention == nil {
		t.Fatalf("mention node missing from body: %s", stdout)
	}
	attrs, _ := mention["attrs"].(map[string]any)
	if id, _ := attrs["id"].(string); id != "557058:abc" {
		t.Errorf("mention accountId = %q, want %q preserved", id, "557058:abc")
	}
	if findNodeOfType(body, "status") == nil {
		t.Errorf("status node missing — non-Markdown nodes must survive the read path")
	}
}

// Read → reuse: the body returned by comment list must survive a dry-run
// comment edit without loss — the dry-run payload echoes the same document.
func TestCommentBodyRoundTripsThroughDryRunEdit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/3/issue/PROJ-1/comment" {
			_, _ = w.Write([]byte(commentWithMention))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	cfg := jiraConfig(t, srv.URL)
	stdout, _, code := runJira(t, "--config", cfg, "issue", "comment", "list", "PROJ-1", "--output=json")
	if code != 0 {
		t.Fatalf("comment list exit = %d\nstdout=%s", code, stdout)
	}
	body := commentBodyFromList(t, stdout)

	bodyJSON, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	bodyFile := filepath.Join(t.TempDir(), "body.json")
	if err := os.WriteFile(bodyFile, bodyJSON, 0o600); err != nil {
		t.Fatalf("write body file: %v", err)
	}

	stdout, _, code = runJira(t,
		"--config", cfg, "issue", "comment", "edit", "PROJ-1", "200",
		"--json-input", bodyFile, "--no-input", "--dry-run", "--output=json")
	if code != 0 {
		t.Fatalf("dry-run edit exit = %d; want 0 (lossless reuse)\nstdout=%s", code, stdout)
	}
	var env struct {
		Data struct {
			BodyADFSummary map[string]any `json:"body_adf_summary"`
			DryRun         bool           `json:"dry_run"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout, &env); err != nil {
		t.Fatalf("decode dry-run envelope: %v\nstdout=%s", err, stdout)
	}
	if !env.Data.DryRun {
		t.Fatal("dry_run = false, want true")
	}
	if !reflect.DeepEqual(env.Data.BodyADFSummary, body) {
		want, _ := json.MarshalIndent(body, "", "  ")
		got, _ := json.MarshalIndent(env.Data.BodyADFSummary, "", "  ")
		t.Fatalf("round-trip changed the document\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func commentBodyFromList(t *testing.T, stdout []byte) map[string]any {
	t.Helper()
	var env struct {
		Data struct {
			Comments []struct {
				Body map[string]any `json:"body"`
			} `json:"comments"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout, &env); err != nil {
		t.Fatalf("decode comment list envelope: %v\nstdout=%s", err, stdout)
	}
	if len(env.Data.Comments) != 1 {
		t.Fatalf("comments = %d, want 1\nstdout=%s", len(env.Data.Comments), stdout)
	}
	return env.Data.Comments[0].Body
}

func findNodeOfType(node map[string]any, nodeType string) map[string]any {
	if node["type"] == nodeType {
		return node
	}
	content, _ := node["content"].([]any)
	for _, child := range content {
		if m, ok := child.(map[string]any); ok {
			if found := findNodeOfType(m, nodeType); found != nil {
				return found
			}
		}
	}
	return nil
}
