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
	"strings"
	"testing"
)

func TestIssueKeyBulkMutationCommandsPreservePartialFailures(t *testing.T) {
	bin := buildJiraBinary(t)
	attachmentPath := filepath.Join(t.TempDir(), "bulk.txt")
	if err := os.WriteFile(attachmentPath, []byte("bulk attachment"), 0o600); err != nil {
		t.Fatalf("WriteFile(attachment) error = %v", err)
	}

	tests := []struct {
		name   string
		args   []string
		handle func(*testing.T, *http.Request, []byte, func(string))
	}{
		{
			name: "comment add",
			args: []string{"issue", "comment", "add", "PROJ-1..3", "-p", "3", "--markdown", "bulk"},
			handle: bulkPartialNestedIssueHandler("comment", http.MethodPost, func(w http.ResponseWriter, key string) {
				_, _ = io.WriteString(w, `{"id":"c-`+key+`","body":{"type":"doc","version":1,"content":[]},"created":"2026-05-05T11:00:00.000+0000","updated":"2026-05-05T11:00:00.000+0000"}`)
			}),
		},
		{
			name: "legacy comment alias",
			args: []string{"issue", "comment", "PROJ-1..3", "-p", "3", "--markdown", "bulk"},
			handle: bulkPartialNestedIssueHandler("comment", http.MethodPost, func(w http.ResponseWriter, key string) {
				_, _ = io.WriteString(w, `{"id":"c-`+key+`","body":{"type":"doc","version":1,"content":[]},"created":"2026-05-05T11:00:00.000+0000","updated":"2026-05-05T11:00:00.000+0000"}`)
			}),
		},
		{
			name: "worklog add",
			args: []string{"worklog", "add", "PROJ-1..3", "-p", "3", "--time-spent", "15m", "--markdown", "bulk"},
			handle: bulkPartialNestedIssueHandler("worklog", http.MethodPost, func(w http.ResponseWriter, key string) {
				_, _ = io.WriteString(w, `{"id":"w-`+key+`","timeSpentSeconds":900}`)
			}),
		},
		{
			name:   "issue watch",
			args:   []string{"issue", "watch", "PROJ-1..3", "-p", "3", "--no-readback"},
			handle: bulkPartialWatcherHandler(http.MethodPost, true),
		},
		{
			name:   "issue unwatch",
			args:   []string{"issue", "unwatch", "PROJ-1..3", "-p", "3", "--no-readback"},
			handle: bulkPartialWatcherHandler(http.MethodDelete, true),
		},
		{
			name:   "watchers add",
			args:   []string{"issue", "watchers", "add", "PROJ-1..3", "-p", "3", "--user", "accountId:bulk-user", "--no-readback"},
			handle: bulkPartialWatcherHandler(http.MethodPost, false),
		},
		{
			name:   "watchers remove",
			args:   []string{"issue", "watchers", "remove", "PROJ-1..3", "-p", "3", "--user", "accountId:bulk-user", "--no-readback"},
			handle: bulkPartialWatcherHandler(http.MethodDelete, false),
		},
		{
			name:   "epic add",
			args:   []string{"epic", "add", "PROJ-1..3", "EPIC-1", "-p", "3"},
			handle: bulkPartialIssueMethodHandler(http.MethodPut),
		},
		{
			name:   "epic remove",
			args:   []string{"epic", "remove", "PROJ-1..3", "-p", "3"},
			handle: bulkPartialIssueMethodHandler(http.MethodPut),
		},
		{
			name: "attachment add",
			args: []string{"issue", "attachment", "add", "PROJ-1..3", "-p", "3", "--file", attachmentPath},
			handle: bulkPartialNestedIssueHandler("attachments", http.MethodPost, func(w http.ResponseWriter, key string) {
				_, _ = io.WriteString(w, `[{"id":"a-`+key+`","filename":"bulk.txt"}]`)
			}),
		},
		{
			name:   "issue link create",
			args:   []string{"issue", "link", "PROJ-1..3", "-p", "3", "--to", "PROJ-99", "--type", "Blocks"},
			handle: bulkPartialLinkCreateHandler,
		},
		{
			name: "issue weblink",
			args: []string{"issue", "weblink", "PROJ-1..3", "-p", "3", "--url", "https://example.com/bulk", "--title", "Bulk"},
			handle: bulkPartialNestedIssueHandler("remotelink", http.MethodPost, func(w http.ResponseWriter, _ string) {
				w.WriteHeader(http.StatusCreated)
			}),
		},
		{
			name:   "issue edit",
			args:   []string{"issue", "edit", "PROJ-1..3", "-p", "3", "--summary", "Bulk edit"},
			handle: bulkPartialIssueEditHandler,
		},
		{
			name: "issue transition execute",
			args: []string{"issue", "transition", "PROJ-1..3", "-p", "3", "--transition", "21"},
			handle: bulkPartialNestedIssueHandler("transitions", http.MethodPost, func(w http.ResponseWriter, _ string) {
				w.WriteHeader(http.StatusNoContent)
			}),
		},
		{
			name:   "issue clone",
			args:   []string{"issue", "clone", "PROJ-1..3", "-p", "3", "--force"},
			handle: bulkPartialIssueCloneHandler,
		},
		{
			name:   "issue move",
			args:   []string{"issue", "move", "PROJ-1..3", "-p", "3", "--force"},
			handle: bulkPartialIssueMethodHandler(http.MethodPut),
		},
		{
			name:   "issue delete",
			args:   []string{"issue", "delete", "PROJ-1..3", "-p", "3", "--force"},
			handle: bulkPartialIssueMethodHandler(http.MethodDelete),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				r.Body = io.NopCloser(bytes.NewReader(body))
				ctx := contextWithResponseWriter(r.Context(), w)
				tt.handle(t, r.WithContext(ctx), body, func(key string) {
					if key != "PROJ-1" && key != "PROJ-3" {
						t.Errorf("unexpected successful key %q", key)
					}
				})
			}))
			defer srv.Close()

			cmd := exec.Command(bin, append([]string{"--config", jiraConfig(t, srv.URL), "--output=json"}, tt.args...)...)
			cmd.Env = append(os.Environ(), "JIRA_TOKEN_DEFAULT=test-token")
			var env struct {
				OK   bool `json:"ok"`
				Data struct {
					Succeeded int `json:"succeeded"`
					Failed    int `json:"failed"`
					Results   []struct {
						Key   string          `json:"key"`
						OK    bool            `json:"ok"`
						Data  json.RawMessage `json:"data"`
						Error json.RawMessage `json:"error"`
					} `json:"results"`
				} `json:"data"`
				Errors []struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"errors"`
			}
			_, _, _ = runCommandExpectErrorEnvelope(t, cmd, &env)

			if env.OK || env.Data.Succeeded != 2 || env.Data.Failed != 1 {
				t.Fatalf("%s partial summary = ok %v succeeded %d failed %d",
					tt.name, env.OK, env.Data.Succeeded, env.Data.Failed)
			}
			if len(env.Data.Results) != 3 {
				t.Fatalf("%s results length = %d, want 3", tt.name, len(env.Data.Results))
			}
			for i, wantKey := range []string{"PROJ-1", "PROJ-2", "PROJ-3"} {
				got := env.Data.Results[i]
				if got.Key != wantKey {
					t.Fatalf("%s result[%d].key = %q, want %q; results=%+v", tt.name, i, got.Key, wantKey, env.Data.Results)
				}
				if wantKey == "PROJ-2" {
					if got.OK || len(got.Error) == 0 || string(got.Error) == "null" {
						t.Fatalf("%s failed result = %+v, want per-key error", tt.name, got)
					}
					continue
				}
				if !got.OK || len(got.Data) == 0 || string(got.Data) == "null" {
					t.Fatalf("%s successful result = %+v, want per-key data", tt.name, got)
				}
			}
			if len(env.Errors) == 0 || env.Errors[0].Code == "" {
				t.Fatalf("%s top-level error missing stable code: %+v", tt.name, env.Errors)
			}
		})
	}
}

func bulkPartialNestedIssueHandler(
	leaf string,
	method string,
	writeSuccess func(http.ResponseWriter, string),
) func(*testing.T, *http.Request, []byte, func(string)) {
	return func(t *testing.T, r *http.Request, _ []byte, hit func(string)) {
		key, ok := bulkNestedIssuePath(r, leaf)
		if !ok || r.Method != method {
			bulkUnexpected(t, r)
			return
		}
		if key == "PROJ-2" {
			writeBulkPartialNotFound(r.Context().Value(httpResponseWriterKey{}).(http.ResponseWriter))
			return
		}
		hit(key)
		writeSuccess(r.Context().Value(httpResponseWriterKey{}).(http.ResponseWriter), key)
	}
}

func bulkPartialWatcherHandler(method string, needsMyself bool) func(*testing.T, *http.Request, []byte, func(string)) {
	return func(t *testing.T, r *http.Request, _ []byte, hit func(string)) {
		if needsMyself && r.Method == http.MethodGet && r.URL.Path == "/rest/api/3/myself" {
			_, _ = io.WriteString(r.Context().Value(httpResponseWriterKey{}).(http.ResponseWriter), `{"accountId":"bulk-user","displayName":"Bulk User"}`)
			return
		}
		key, ok := bulkNestedIssuePath(r, "watchers")
		if !ok || r.Method != method {
			bulkUnexpected(t, r)
			return
		}
		if key == "PROJ-2" {
			writeBulkPartialNotFound(r.Context().Value(httpResponseWriterKey{}).(http.ResponseWriter))
			return
		}
		hit(key)
		r.Context().Value(httpResponseWriterKey{}).(http.ResponseWriter).WriteHeader(http.StatusNoContent)
	}
}

func bulkPartialIssueMethodHandler(method string) func(*testing.T, *http.Request, []byte, func(string)) {
	return func(t *testing.T, r *http.Request, _ []byte, hit func(string)) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/editmeta") {
			_, _ = io.WriteString(r.Context().Value(httpResponseWriterKey{}).(http.ResponseWriter), bulkEditMetaBody())
			return
		}
		key, ok := bulkIssuePath(r.URL.Path)
		if !ok || r.Method != method {
			bulkUnexpected(t, r)
			return
		}
		if key == "PROJ-2" {
			writeBulkPartialNotFound(r.Context().Value(httpResponseWriterKey{}).(http.ResponseWriter))
			return
		}
		hit(key)
		if method == http.MethodDelete {
			r.Context().Value(httpResponseWriterKey{}).(http.ResponseWriter).WriteHeader(http.StatusNoContent)
			return
		}
		_, _ = io.WriteString(r.Context().Value(httpResponseWriterKey{}).(http.ResponseWriter), `{"id":"`+key+`","key":"`+key+`","fields":{"summary":"bulk"}}`)
	}
}

func bulkPartialIssueEditHandler(t *testing.T, r *http.Request, body []byte, hit func(string)) {
	if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/editmeta") {
		_, _ = io.WriteString(r.Context().Value(httpResponseWriterKey{}).(http.ResponseWriter), bulkEditMetaBody())
		return
	}
	key, ok := bulkIssuePath(r.URL.Path)
	if !ok || r.Method != http.MethodPut {
		bulkUnexpected(t, r)
		return
	}
	if key == "PROJ-2" {
		writeBulkPartialNotFound(r.Context().Value(httpResponseWriterKey{}).(http.ResponseWriter))
		return
	}
	bulkIssueEditHandler(t, r, body, hit)
}

func bulkPartialIssueCloneHandler(t *testing.T, r *http.Request, body []byte, hit func(string)) {
	if r.Method == http.MethodGet {
		key, ok := bulkIssuePath(r.URL.Path)
		if !ok {
			bulkUnexpected(t, r)
			return
		}
		if key == "PROJ-2" {
			writeBulkPartialNotFound(r.Context().Value(httpResponseWriterKey{}).(http.ResponseWriter))
			return
		}
	}
	bulkIssueCloneHandler(t, r, body, hit)
}

func bulkPartialLinkCreateHandler(t *testing.T, r *http.Request, body []byte, hit func(string)) {
	if r.Method != http.MethodPost || r.URL.Path != "/rest/api/3/issueLink" {
		bulkUnexpected(t, r)
		return
	}
	key := bulkJSONIssueKey(t, body, "inwardIssue")
	if key == "PROJ-2" {
		writeBulkPartialNotFound(r.Context().Value(httpResponseWriterKey{}).(http.ResponseWriter))
		return
	}
	hit(key)
	r.Context().Value(httpResponseWriterKey{}).(http.ResponseWriter).WriteHeader(http.StatusCreated)
}

func writeBulkPartialNotFound(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNotFound)
	_, _ = io.WriteString(w, `{"errorMessages":["issue does not exist"],"errors":{}}`)
}
