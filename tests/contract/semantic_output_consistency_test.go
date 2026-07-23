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

func TestAttachmentListCarriesCanonicalIssueAndPreservesProjectedBytes(t *testing.T) {
	srv, _ := newCommentServer(t, map[string]http.HandlerFunc{
		"GET /rest/api/3/issue/PROJ-1": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, `{"key":"PROJ-1","fields":{"attachment":[{`+
				`"id":"200","filename":"baseline.txt","mimeType":"text/plain","size":8,`+
				`"author":{"accountId":"acc-a","displayName":"Alice"},`+
				`"created":"2026-07-01T10:00:00.000+0000"}]}}`)
		},
	})
	stdout, stderr, code := runJira(
		t,
		"--config", jiraConfig(t, srv.URL),
		"--output=json",
		"issue", "attachment", "list", "PROJ-1",
	)
	if code != 0 {
		t.Fatalf("attachment list exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}

	var env struct {
		Data struct {
			Issue struct {
				Key string `json:"key"`
			} `json:"issue"`
			Attachments []json.RawMessage `json:"attachments"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout, &env); err != nil {
		t.Fatalf("decode envelope: %v\n%s", err, stdout)
	}
	if env.Data.Issue.Key != "PROJ-1" {
		t.Fatalf("data.issue.key = %q, want PROJ-1\n%s", env.Data.Issue.Key, stdout)
	}
	if len(env.Data.Attachments) != 1 {
		t.Fatalf("attachments = %d, want 1\n%s", len(env.Data.Attachments), stdout)
	}
	const frozenV015Row = `{"author":{"account_id":"acc-a","display_name":"Alice"},"created":"2026-07-01T10:00:00.000+0000","filename":"baseline.txt","id":"200","mime_type":"text/plain","size":8}`
	if got := string(env.Data.Attachments[0]); got != frozenV015Row {
		t.Fatalf("attachment projection bytes changed\n got: %s\nwant: %s", got, frozenV015Row)
	}
}

func TestWatcherListCarriesCanonicalIssueAndPreservesProjectedBytes(t *testing.T) {
	srv, _ := newCommentServer(t, map[string]http.HandlerFunc{
		"GET /rest/api/3/issue/PROJ-1/watchers": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, `{"isWatching":true,"watchCount":1,"watchers":[{`+
				`"accountId":"acc-a","displayName":"Alice","emailAddress":"alice@example.com"}]}`)
		},
	})
	stdout, stderr, code := runJira(
		t,
		"--config", jiraConfig(t, srv.URL),
		"--output=json",
		"issue", "watchers", "list", "PROJ-1",
	)
	if code != 0 {
		t.Fatalf("watcher list exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}

	var env struct {
		Data struct {
			Issue struct {
				Key string `json:"key"`
			} `json:"issue"`
			Watchers []json.RawMessage `json:"watchers"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout, &env); err != nil {
		t.Fatalf("decode envelope: %v\n%s", err, stdout)
	}
	if env.Data.Issue.Key != "PROJ-1" {
		t.Fatalf("data.issue.key = %q, want PROJ-1\n%s", env.Data.Issue.Key, stdout)
	}
	if len(env.Data.Watchers) != 1 {
		t.Fatalf("watchers = %d, want 1\n%s", len(env.Data.Watchers), stdout)
	}
	const frozenV015Row = `{"account_id":"acc-a","display_name":"Alice","email_address":"alice@example.com"}`
	if got := string(env.Data.Watchers[0]); got != frozenV015Row {
		t.Fatalf("watcher projection bytes changed\n got: %s\nwant: %s", got, frozenV015Row)
	}
}
