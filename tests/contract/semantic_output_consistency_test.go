package contract

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestCommentListCarriesCanonicalIssueAndPreservesProjectedBytes(t *testing.T) {
	page := `{
		"comments": [{
			"id": "100",
			"body": {"type":"doc","version":1,"content":[]},
			"author": null,
			"updateAuthor": null,
			"created": "2026-07-01T10:00:00.000+0000",
			"updated": "2026-07-01T10:00:00.000+0000",
			"visibility": null
		}],
		"startAt": 0,
		"maxResults": 50,
		"total": 1
	}`
	srv, _ := newCommentServer(t, map[string]http.HandlerFunc{
		"GET /rest/api/3/issue/PROJ-1/comment": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, page)
		},
	})

	stdout, stderr, code := runJira(
		t,
		"--config", jiraConfig(t, srv.URL),
		"--output=json",
		"issue", "comment", "list", "PROJ-1",
	)
	if code != 0 {
		t.Fatalf("comment list exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}

	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(stdout, &env); err != nil {
		t.Fatalf("decode envelope: %v\n%s", err, stdout)
	}
	var data struct {
		Issue struct {
			Key string `json:"key"`
		} `json:"issue"`
		Comments []json.RawMessage `json:"comments"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("decode data: %v\n%s", err, env.Data)
	}
	if data.Issue.Key != "PROJ-1" {
		t.Fatalf("data.issue.key = %q, want PROJ-1\n%s", data.Issue.Key, env.Data)
	}
	if len(data.Comments) != 1 {
		t.Fatalf("comments = %d, want 1\n%s", len(data.Comments), env.Data)
	}

	const frozenV015Row = `{"author":null,"body":{"type":"doc","version":1},"created":"2026-07-01T10:00:00.000+0000","id":"100","update_author":null,"updated":"2026-07-01T10:00:00.000+0000","visibility":null}`
	if got := string(data.Comments[0]); got != frozenV015Row {
		t.Fatalf("comment projection bytes changed\n got: %s\nwant: %s", got, frozenV015Row)
	}
}
