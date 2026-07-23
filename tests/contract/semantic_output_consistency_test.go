package contract

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

var semanticIdentityMatrix = []struct {
	Operation  string
	Collection string
}{
	{"issue.comment.list", "comments"},
	{"issue.attachment.list", "attachments"},
	{"issue.watchers.list", "watchers"},
}

var semanticParityMatrix = []struct {
	Operation    string
	StableFields []string
}{
	{"issue.create", []string{"preview", "validated_remotely"}},
	{"issue.comment.edit", []string{"issue", "comment_id", "body_adf_summary", "visibility_change"}},
	{"issue.transition", []string{"issue", "fields", "comment", "update", "transition_validated"}},
	{"issue.clone", []string{"issue", "payload"}},
	{"issue.move", []string{"issue", "payload"}},
	{"issue.delete", []string{"issue", "payload"}},
	{"issue.attachment.add", []string{"issue", "files"}},
	{"issue.attachment.download", []string{"issue", "attachment_id", "mode", "target"}},
	{"issue.watchers.add", []string{"issue", "user", "user_resolved", "account_id_resolved"}},
	{"issue.watchers.remove", []string{"issue", "user", "user_resolved", "account_id_resolved"}},
	{"issue.weblink", []string{"issue", "url", "title", "url_remote_checked"}},
}

var semanticExtensionMatrix = []struct {
	Operation string
	Field     string
	Required  bool
}{
	{"issue.comment.list", "issue", true},
	{"issue.attachment.list", "issue", true},
	{"issue.watchers.list", "issue", true},
	{"issue.create", "preview", true},
	{"issue.create", "validated_remotely", true},
	{"issue.comment.edit", "comment_id", true},
	{"issue.comment.edit", "body_adf_summary", true},
	{"issue.comment.edit", "visibility_change", true},
	{"issue.transition", "transition_validated", true},
	{"issue.clone", "payload", true},
	{"issue.move", "payload", true},
	{"issue.delete", "payload", true},
	{"issue.attachment.add", "files", true},
	{"issue.attachment.download", "target", true},
	{"issue.watchers.add", "user", true},
	{"issue.watchers.add", "user_resolved", true},
	{"issue.watchers.add", "account_id_resolved", false},
	{"issue.watchers.remove", "user", true},
	{"issue.watchers.remove", "user_resolved", true},
	{"issue.watchers.remove", "account_id_resolved", false},
	{"issue.weblink", "url_remote_checked", true},
}

var semanticConditionalOutcomeMatrix = []struct {
	Operation string
	Fields    []string
}{
	{"issue.create", []string{"issue", "verification"}},
	{"issue.comment.edit", []string{"comment"}},
	{"issue.clone", []string{"result"}},
	{"issue.move", []string{"result"}},
	{"issue.delete", nil},
	{"issue.attachment.add", []string{"attachments"}},
	{"issue.attachment.download", []string{"written_to", "bytes"}},
	{"issue.watchers.add", []string{"account_id", "attempted", "watchers", "is_watching", "watch_count", "was_already_watching"}},
	{"issue.watchers.remove", []string{"account_id", "attempted", "watchers", "is_watching", "watch_count", "was_already_watching"}},
}

func TestSemanticContractTablesStayExplicit(t *testing.T) {
	schemas := declaredOutputSchemas(t)
	if got := identityOperations(); !reflect.DeepEqual(got, []string{
		"issue.comment.list",
		"issue.attachment.list",
		"issue.watchers.list",
	}) {
		t.Fatalf("identity matrix operations = %v", got)
	}
	if got := parityOperations(); !reflect.DeepEqual(got, []string{
		"issue.create",
		"issue.comment.edit",
		"issue.transition",
		"issue.clone",
		"issue.move",
		"issue.delete",
		"issue.attachment.add",
		"issue.attachment.download",
		"issue.watchers.add",
		"issue.watchers.remove",
		"issue.weblink",
	}) {
		t.Fatalf("parity matrix operations = %v", got)
	}
	for _, identity := range semanticIdentityMatrix {
		schema, _ := schemas[identity.Operation].(map[string]any)
		properties, _ := schema["properties"].(map[string]any)
		if _, ok := properties["issue"]; !ok || !schemaRequires(schema, "issue") {
			t.Fatalf("%s must declare required issue identity: %#v", identity.Operation, schema)
		}
		if _, ok := properties[identity.Collection]; !ok {
			t.Fatalf("%s omits identity collection %q: %#v", identity.Operation, identity.Collection, schema)
		}
	}
	for _, parity := range semanticParityMatrix {
		schema, _ := schemas[parity.Operation].(map[string]any)
		properties, _ := schema["properties"].(map[string]any)
		for _, field := range parity.StableFields {
			if _, ok := properties[field]; !ok {
				t.Fatalf("%s omits stable field %q: %#v", parity.Operation, field, schema)
			}
		}
	}
	seen := map[string]bool{}
	for _, extension := range semanticExtensionMatrix {
		key := extension.Operation + "." + extension.Field
		if extension.Operation == "" || extension.Field == "" || seen[key] {
			t.Fatalf("invalid or duplicate extension row %q", key)
		}
		seen[key] = true
		schema, _ := schemas[extension.Operation].(map[string]any)
		properties, _ := schema["properties"].(map[string]any)
		if _, ok := properties[extension.Field]; !ok {
			t.Fatalf("%s is absent from the published schema", key)
		}
		if extension.Required != schemaRequires(schema, extension.Field) {
			t.Fatalf("%s required = %v, want %v", key, schemaRequires(schema, extension.Field), extension.Required)
		}
	}
	for _, outcome := range semanticConditionalOutcomeMatrix {
		if outcome.Operation == "" {
			t.Fatal("conditional-outcome matrix has a blank operation")
		}
		schema, _ := schemas[outcome.Operation].(map[string]any)
		properties, _ := schema["properties"].(map[string]any)
		for _, field := range outcome.Fields {
			if _, ok := properties[field]; !ok {
				t.Fatalf("%s omits conditional outcome %q: %#v", outcome.Operation, field, schema)
			}
			if schemaRequires(schema, field) {
				t.Fatalf("%s outcome %q must stay conditional: %#v", outcome.Operation, field, schema)
			}
		}
	}
}

func identityOperations() []string {
	out := make([]string, 0, len(semanticIdentityMatrix))
	for _, row := range semanticIdentityMatrix {
		out = append(out, row.Operation)
	}
	return out
}

func parityOperations() []string {
	out := make([]string, 0, len(semanticParityMatrix))
	for _, row := range semanticParityMatrix {
		out = append(out, row.Operation)
	}
	return out
}

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

func TestAttachmentAddStableContextMatchesDryRunAndLive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[{"id":"200","filename":"stable.txt","mimeType":"text/plain","size":6,"author":{"accountId":"acc-a","displayName":"Alice"},"created":"2026-07-01T10:00:00.000+0000"}]`)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("JIRA_TOKEN_DEFAULT", "test-token")
	file := filepath.Join(t.TempDir(), "stable.txt")
	if err := os.WriteFile(file, []byte("stable"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg := jiraConfig(t, srv.URL)

	for _, keys := range [][]string{{"PROJ-1"}, {"PROJ-1", "PROJ-2"}} {
		keyMode := "single"
		if len(keys) > 1 {
			keyMode = "multi"
		}
		t.Run(keyMode, func(t *testing.T) {
			base := []string{"--config", cfg, "--output=json", "issue", "attachment", "add"}
			base = append(base, keys...)
			base = append(base, "--file", file)
			if len(keys) == 1 {
				dry := successfulData(t, append(base, "--dry-run")...)
				live := successfulData(t, base...)
				requireSameJSONField(t, dry, live, "issue")
				requireSameJSONField(t, dry, live, "files")
				if _, exists := live["attachments"]; !exists {
					t.Fatalf("live upload omitted attachments: %#v", live)
				}
				return
			}
			dry := successfulKeyedData(t, append(base, "--dry-run")...)
			live := successfulKeyedData(t, base...)
			for _, key := range keys {
				requireSameJSONField(t, dry[key], live[key], "issue")
				requireSameJSONField(t, dry[key], live[key], "files")
				if _, exists := live[key]["attachments"]; !exists {
					t.Fatalf("live upload for %s omitted attachments: %#v", key, live[key])
				}
			}
		})
	}
}

func TestAttachmentDownloadStableContextMatchesDryRunAndLive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "stable download")
	}))
	t.Cleanup(srv.Close)
	t.Setenv("JIRA_TOKEN_DEFAULT", "test-token")
	workDir := t.TempDir()
	base := []string{
		"--config", jiraConfig(t, srv.URL),
		"--output=json",
		"issue", "attachment", "download",
		"PROJ-1", "200",
		"--to", "stable.bin",
	}
	dry := successfulDataInDir(t, workDir, append(base, "--dry-run")...)
	live := successfulDataInDir(t, workDir, base...)

	for _, field := range []string{"issue", "attachment_id", "mode", "target"} {
		requireSameJSONField(t, dry, live, field)
	}
	if _, exists := dry["written_to"]; exists {
		t.Fatalf("dry-run fabricated written_to: %#v", dry)
	}
	if _, exists := live["written_to"]; !exists {
		t.Fatalf("live download omitted written_to: %#v", live)
	}
}

func TestWatcherMutationStableContextMatchesDryRunAndLive(t *testing.T) {
	for _, operation := range []string{"add", "remove"} {
		for _, noReadback := range []bool{false, true} {
			for _, keys := range [][]string{{"PROJ-1"}, {"PROJ-1", "PROJ-2"}} {
				name := operation
				if noReadback {
					name += "-no-readback"
				}
				if len(keys) == 1 {
					name += "-single"
				} else {
					name += "-multi"
				}
				t.Run(name, func(t *testing.T) {
					srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Type", "application/json")
						switch r.Method {
						case http.MethodGet:
							_, _ = io.WriteString(w, `{"isWatching":true,"watchCount":1,"watchers":[{"accountId":"acc-a","displayName":"Alice"}]}`)
						case http.MethodPost, http.MethodDelete:
							w.WriteHeader(http.StatusNoContent)
						default:
							w.WriteHeader(http.StatusNotFound)
						}
					}))
					t.Cleanup(srv.Close)
					t.Setenv("JIRA_TOKEN_DEFAULT", "test-token")
					base := []string{"--config", jiraConfig(t, srv.URL), "--output=json", "issue", "watchers", operation}
					base = append(base, keys...)
					base = append(base, "--user", "accountId:acc-a")
					if noReadback {
						base = append(base, "--no-readback")
					}
					if len(keys) == 1 {
						dry := successfulData(t, append(base, "--dry-run")...)
						live := successfulData(t, base...)
						requireWatcherStableContext(t, dry, live)
						requireWatcherOutcome(t, live, noReadback)
						return
					}
					dry := successfulKeyedData(t, append(base, "--dry-run")...)
					live := successfulKeyedData(t, base...)
					for _, key := range keys {
						requireWatcherStableContext(t, dry[key], live[key])
						requireWatcherOutcome(t, live[key], noReadback)
					}
				})
			}
		}
	}
}

func TestWebLinkStableContextMatchesDryRunAndLive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("JIRA_TOKEN_DEFAULT", "test-token")
	cfg := jiraConfig(t, srv.URL)
	for _, keys := range [][]string{{"PROJ-1"}, {"PROJ-1", "PROJ-2"}} {
		base := []string{"--config", cfg, "--output=json", "issue", "weblink"}
		base = append(base, keys...)
		base = append(base, "--url", "https://example.com/stable", "--title", "Stable link")
		if len(keys) == 1 {
			dry := successfulData(t, append(base, "--dry-run")...)
			live := successfulData(t, base...)
			for _, field := range []string{"issue", "url", "title", "url_remote_checked"} {
				requireSameJSONField(t, dry, live, field)
			}
			continue
		}
		dry := successfulKeyedData(t, append(base, "--dry-run")...)
		live := successfulKeyedData(t, base...)
		for _, key := range keys {
			for _, field := range []string{"issue", "url", "title", "url_remote_checked"} {
				requireSameJSONField(t, dry[key], live[key], field)
			}
		}
	}
}

func successfulData(t *testing.T, args ...string) map[string]any {
	t.Helper()
	stdout, stderr, code := runJira(t, args...)
	if code != 0 {
		t.Fatalf("jira %v exit = %d\nstdout=%s\nstderr=%s", args, code, stdout, stderr)
	}
	var env struct {
		Meta struct {
			Command string `json:"command"`
		} `json:"meta"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(stdout, &env); err != nil {
		t.Fatalf("decode envelope: %v\n%s", err, stdout)
	}
	requireSemanticDataConforms(t, env.Meta.Command, env.Data)
	return env.Data
}

func successfulDataInDir(t *testing.T, dir string, args ...string) map[string]any {
	t.Helper()
	cmd := exec.Command(buildJiraBinary(t), args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "JIRA_TOKEN_DEFAULT=test-token")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("jira %v error = %v\nstdout=%s\nstderr=%s", args, err, stdout.Bytes(), stderr.Bytes())
	}
	var env struct {
		Meta struct {
			Command string `json:"command"`
		} `json:"meta"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v\n%s", err, stdout.Bytes())
	}
	requireSemanticDataConforms(t, env.Meta.Command, env.Data)
	return env.Data
}

func requireSemanticDataConforms(t *testing.T, operation string, data map[string]any) {
	t.Helper()
	schema, ok := declaredOutputSchemas(t)[operation].(map[string]any)
	if !ok {
		t.Fatalf("no declared output schema for %q", operation)
	}
	if results, keyed := data["results"].([]any); keyed {
		for _, raw := range results {
			result, _ := raw.(map[string]any)
			value, _ := result["data"].(map[string]any)
			if errs := conformanceErrors("data.results[].data", value, schema); len(errs) > 0 {
				t.Fatalf("%s keyed envelope does not conform to its declared schema:\n%s", operation, errs)
			}
		}
		return
	}
	if errs := conformanceErrors("data", data, schema); len(errs) > 0 {
		t.Fatalf("%s envelope does not conform to its declared schema:\n%s", operation, errs)
	}
}

func successfulKeyedData(t *testing.T, args ...string) map[string]map[string]any {
	t.Helper()
	data := successfulData(t, args...)
	results, ok := data["results"].([]any)
	if !ok {
		t.Fatalf("keyed result missing data.results: %#v", data)
	}
	out := make(map[string]map[string]any, len(results))
	for _, raw := range results {
		result, _ := raw.(map[string]any)
		key, _ := result["key"].(string)
		value, _ := result["data"].(map[string]any)
		out[key] = value
	}
	return out
}

func requireWatcherStableContext(t *testing.T, dry, live map[string]any) {
	t.Helper()
	for _, field := range []string{"issue", "user", "user_resolved", "account_id_resolved"} {
		requireSameJSONField(t, dry, live, field)
	}
}

func requireWatcherOutcome(t *testing.T, live map[string]any, noReadback bool) {
	t.Helper()
	if noReadback {
		for _, field := range []string{"account_id", "attempted"} {
			if _, exists := live[field]; !exists {
				t.Fatalf("no-readback outcome omitted %q: %#v", field, live)
			}
		}
		return
	}
	for _, field := range []string{"watchers", "is_watching", "watch_count", "was_already_watching"} {
		if _, exists := live[field]; !exists {
			t.Fatalf("readback outcome omitted %q: %#v", field, live)
		}
	}
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
