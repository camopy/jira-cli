package contract

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
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

func TestCreateStableContextMatchesDryRunAndLive(t *testing.T) {
	mux := http.NewServeMux()
	registerCreatemeta(
		mux,
		"PROJ",
		"Task",
		"10001",
		`[{"fieldId":"summary","name":"Summary","required":true,"schema":{"type":"string"}}]`,
	)
	mux.HandleFunc("POST /rest/api/3/issue", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"10001","key":"PROJ-9","self":"https://example.invalid/PROJ-9"}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("JIRA_TOKEN_DEFAULT", "test-token")
	cfg := jiraConfig(t, srv.URL)
	base := []string{
		"--config", cfg,
		"--output=json",
		"issue", "create",
		"--no-input",
		"--summary", "Stable create",
		"--project", "PROJ",
		"--type", "Task",
	}
	dry := successfulData(t, append(base, "--dry-run")...)
	live := successfulData(t, base...)

	requireSameJSONField(t, dry, live, "preview")
	requireJSONType(t, dry, "validated_remotely", "boolean")
	requireJSONType(t, live, "validated_remotely", "boolean")
	if _, exists := dry["issue"]; exists {
		t.Fatalf("dry-run fabricated a server issue: %#v", dry)
	}
	if _, exists := live["issue"]; !exists {
		t.Fatalf("live create omitted its server issue: %#v", live)
	}
}

func TestCommentEditStableContextMatchesDryRunAndLive(t *testing.T) {
	srv, _ := newCommentServer(t, map[string]http.HandlerFunc{
		"PUT /rest/api/3/issue/PROJ-1/comment/55": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, `{"id":"55","body":{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"stable edit"}]}]},"author":{"accountId":"a","displayName":"Alice"},"created":"2026-07-01T10:00:00.000+0000","updated":"2026-07-02T10:00:00.000+0000"}`)
		},
	})
	cfg := jiraConfig(t, srv.URL)
	base := []string{
		"--config", cfg,
		"--output=json",
		"issue", "comment", "edit",
		"PROJ-1", "55",
		"--markdown", "stable edit",
		"--visibility-role", "Developers",
	}
	dry := successfulData(t, append(base, "--dry-run")...)
	live := successfulData(t, base...)

	for _, field := range []string{"issue", "comment_id", "body_adf_summary", "visibility_change"} {
		requireSameJSONField(t, dry, live, field)
	}
	if _, exists := dry["comment"]; exists {
		t.Fatalf("dry-run fabricated a server comment: %#v", dry)
	}
	if _, exists := live["comment"]; !exists {
		t.Fatalf("live edit omitted its server comment: %#v", live)
	}
}

func TestTransitionStableContextMatchesDryRunAndLive(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/issue/PROJ-1/transitions", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"transitions":[{"id":"31","name":"Done","hasScreen":true}]}`)
	})
	mux.HandleFunc("POST /rest/api/3/issue/PROJ-1/transitions", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("JIRA_TOKEN_DEFAULT", "test-token")
	payload := writeJSON(
		t,
		"transition-context.json",
		`{"fields":{"resolution":{"name":"Done"}},"comment":{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"closing"}]}]},"update":{"labels":[{"add":"released"}]}}`,
	)
	cfg := jiraConfig(t, srv.URL)
	base := []string{
		"--config", cfg,
		"--output=json",
		"issue", "transition",
		"PROJ-1", "Done",
		"--no-input",
		"--json-input", payload,
	}
	dry := successfulData(t, append(base, "--dry-run")...)
	live := successfulData(t, base...)

	for _, field := range []string{"issue", "fields", "comment", "update"} {
		requireSameJSONField(t, dry, live, field)
	}
	for _, data := range []map[string]any{dry, live} {
		requireJSONType(t, data, "transition", "string")
		requireJSONType(t, data, "transition_validated", "boolean")
	}
}

func TestDestructiveStableContextMatchesDryRunAndLive(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/issue/PROJ-1/editmeta", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"fields":{"summary":{"name":"Summary","fieldId":"summary","required":true,"schema":{"type":"string"}}}}`)
	})
	mux.HandleFunc("GET /rest/api/3/issue/PROJ-1", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"key":"PROJ-1","fields":{"summary":"Original","issuetype":{"name":"Task"},"project":{"key":"PROJ"}}}`)
	})
	mux.HandleFunc("POST /rest/api/3/issue", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"key":"PROJ-2"}`)
	})
	mux.HandleFunc("PUT /rest/api/3/issue/PROJ-1", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"key":"PROJ-1"}`)
	})
	mux.HandleFunc("DELETE /rest/api/3/issue/PROJ-1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("JIRA_TOKEN_DEFAULT", "test-token")
	cfg := jiraConfig(t, srv.URL)
	payload := writeJSON(t, "destructive-context.json", `{"fields":{"summary":"Stable payload"}}`)

	for _, operation := range []string{"clone", "move", "delete"} {
		t.Run(operation, func(t *testing.T) {
			base := []string{
				"--config", cfg,
				"--output=json",
				"issue", operation,
				"PROJ-1",
				"--no-input",
			}
			if operation != "delete" {
				base = append(base, "--json-input", payload)
			}
			dry := successfulData(t, append(base, "--dry-run")...)
			live := successfulData(t, append(base, "--force")...)
			requireSameJSONField(t, dry, live, "issue")
			requireSameJSONField(t, dry, live, "payload")
			if operation == "delete" {
				if _, exists := live["result"]; exists {
					t.Fatalf("live delete fabricated a result: %#v", live)
				}
			} else if _, exists := live["result"]; !exists {
				t.Fatalf("live %s omitted its server result: %#v", operation, live)
			}
		})
	}
}

func successfulData(t *testing.T, args ...string) map[string]any {
	t.Helper()
	stdout, stderr, code := runJira(t, args...)
	if code != 0 {
		t.Fatalf("jira %v exit = %d\nstdout=%s\nstderr=%s", args, code, stdout, stderr)
	}
	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(stdout, &env); err != nil {
		t.Fatalf("decode envelope: %v\n%s", err, stdout)
	}
	return env.Data
}

func requireSameJSONField(t *testing.T, dry, live map[string]any, field string) {
	t.Helper()
	dryValue, dryExists := dry[field]
	liveValue, liveExists := live[field]
	if !dryExists || !liveExists {
		t.Fatalf("%q presence differs: dry=%#v live=%#v", field, dry, live)
	}
	if jsonType(dryValue) != jsonType(liveValue) {
		t.Fatalf("%q type differs: dry=%s live=%s", field, jsonType(dryValue), jsonType(liveValue))
	}
	if !reflect.DeepEqual(dryValue, liveValue) {
		t.Fatalf("%q value differs:\n dry=%#v\nlive=%#v", field, dryValue, liveValue)
	}
}

func requireJSONType(t *testing.T, data map[string]any, field, want string) {
	t.Helper()
	value, exists := data[field]
	if !exists {
		t.Fatalf("%q absent: %#v", field, data)
	}
	if got := jsonType(value); got != want {
		t.Fatalf("%q type = %s, want %s: %#v", field, got, want, data)
	}
}

func jsonType(value any) string {
	switch value.(type) {
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "boolean"
	case nil:
		return "null"
	default:
		return "unknown"
	}
}
