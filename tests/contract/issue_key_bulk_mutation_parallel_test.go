package contract

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestIssueKeyBulkMutationCommandsAcceptRangesAndParallelism(t *testing.T) {
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
			args: []string{"issue", "comment", "add", "PROJ-1..2", "-p", "2", "--markdown", "bulk"},
			handle: func(t *testing.T, r *http.Request, _ []byte, hit func(string)) {
				key, ok := bulkNestedIssuePath(r, "comment")
				if !ok || r.Method != http.MethodPost {
					bulkUnexpected(t, r)
					return
				}
				hit(key)
				_, _ = io.WriteString(r.Context().Value(httpResponseWriterKey{}).(http.ResponseWriter), `{"id":"c-`+key+`","body":{"type":"doc","version":1,"content":[]},"created":"2026-05-05T11:00:00.000+0000","updated":"2026-05-05T11:00:00.000+0000"}`)
			},
		},
		{
			name: "legacy comment alias",
			args: []string{"issue", "comment", "PROJ-1..2", "-p", "2", "--markdown", "bulk"},
			handle: func(t *testing.T, r *http.Request, _ []byte, hit func(string)) {
				key, ok := bulkNestedIssuePath(r, "comment")
				if !ok || r.Method != http.MethodPost {
					bulkUnexpected(t, r)
					return
				}
				hit(key)
				_, _ = io.WriteString(r.Context().Value(httpResponseWriterKey{}).(http.ResponseWriter), `{"id":"c-`+key+`","body":{"type":"doc","version":1,"content":[]},"created":"2026-05-05T11:00:00.000+0000","updated":"2026-05-05T11:00:00.000+0000"}`)
			},
		},
		{
			name: "worklog add",
			args: []string{"worklog", "add", "PROJ-1..2", "-p", "2", "--time-spent", "15m", "--markdown", "bulk"},
			handle: func(t *testing.T, r *http.Request, _ []byte, hit func(string)) {
				key, ok := bulkNestedIssuePath(r, "worklog")
				if !ok || r.Method != http.MethodPost {
					bulkUnexpected(t, r)
					return
				}
				hit(key)
				_, _ = io.WriteString(r.Context().Value(httpResponseWriterKey{}).(http.ResponseWriter), `{"id":"w-`+key+`","timeSpentSeconds":900}`)
			},
		},
		{
			name:   "issue watch",
			args:   []string{"issue", "watch", "PROJ-1..2", "-p", "2", "--no-readback"},
			handle: bulkWatcherHandler(http.MethodPost, true),
		},
		{
			name:   "issue unwatch",
			args:   []string{"issue", "unwatch", "PROJ-1..2", "-p", "2", "--no-readback"},
			handle: bulkWatcherHandler(http.MethodDelete, true),
		},
		{
			name:   "watchers add",
			args:   []string{"issue", "watchers", "add", "PROJ-1..2", "-p", "2", "--user", "accountId:bulk-user", "--no-readback"},
			handle: bulkWatcherHandler(http.MethodPost, false),
		},
		{
			name:   "watchers remove",
			args:   []string{"issue", "watchers", "remove", "PROJ-1..2", "-p", "2", "--user", "accountId:bulk-user", "--no-readback"},
			handle: bulkWatcherHandler(http.MethodDelete, false),
		},
		{
			name:   "epic add",
			args:   []string{"epic", "add", "PROJ-1..2", "EPIC-1", "-p", "2"},
			handle: bulkIssueMethodHandler(http.MethodPut),
		},
		{
			name:   "epic remove",
			args:   []string{"epic", "remove", "PROJ-1..2", "-p", "2"},
			handle: bulkIssueMethodHandler(http.MethodPut),
		},
		{
			name: "attachment add",
			args: []string{"issue", "attachment", "add", "PROJ-1..2", "-p", "2", "--file", attachmentPath},
			handle: func(t *testing.T, r *http.Request, _ []byte, hit func(string)) {
				key, ok := bulkNestedIssuePath(r, "attachments")
				if !ok || r.Method != http.MethodPost {
					bulkUnexpected(t, r)
					return
				}
				hit(key)
				_, _ = io.WriteString(r.Context().Value(httpResponseWriterKey{}).(http.ResponseWriter), `[{"id":"a-`+key+`","filename":"bulk.txt"}]`)
			},
		},
		{
			name: "issue link create",
			args: []string{"issue", "link", "PROJ-1..2", "-p", "2", "--to", "PROJ-99", "--type", "Blocks"},
			handle: func(t *testing.T, r *http.Request, body []byte, hit func(string)) {
				// The preview resolution fetches the link types once when
				// the per-profile cache is cold; it is not a per-key hit.
				if r.Method == http.MethodGet && r.URL.Path == "/rest/api/3/issueLinkType" {
					_, _ = io.WriteString(r.Context().Value(httpResponseWriterKey{}).(http.ResponseWriter),
						`{"issueLinkTypes":[{"id":"10000","name":"Blocks","inward":"is blocked by","outward":"blocks"}]}`)
					return
				}
				if r.Method != http.MethodPost || r.URL.Path != "/rest/api/3/issueLink" {
					bulkUnexpected(t, r)
					return
				}
				hit(bulkJSONIssueKey(t, body, "inwardIssue"))
				r.Context().Value(httpResponseWriterKey{}).(http.ResponseWriter).WriteHeader(http.StatusCreated)
			},
		},
		{
			name: "issue weblink",
			args: []string{"issue", "weblink", "PROJ-1..2", "-p", "2", "--url", "https://example.com/bulk", "--title", "Bulk"},
			handle: func(t *testing.T, r *http.Request, _ []byte, hit func(string)) {
				key, ok := bulkNestedIssuePath(r, "remotelink")
				if !ok || r.Method != http.MethodPost {
					bulkUnexpected(t, r)
					return
				}
				hit(key)
				r.Context().Value(httpResponseWriterKey{}).(http.ResponseWriter).WriteHeader(http.StatusCreated)
			},
		},
		{
			name:   "issue edit",
			args:   []string{"issue", "edit", "PROJ-1..2", "-p", "2", "--summary", "Bulk edit"},
			handle: bulkIssueEditHandler,
		},
		{
			name: "issue transition execute",
			args: []string{"issue", "transition", "PROJ-1..2", "-p", "2", "--transition", "21"},
			handle: func(t *testing.T, r *http.Request, _ []byte, hit func(string)) {
				key, ok := bulkNestedIssuePath(r, "transitions")
				if !ok || r.Method != http.MethodPost {
					bulkUnexpected(t, r)
					return
				}
				hit(key)
				r.Context().Value(httpResponseWriterKey{}).(http.ResponseWriter).WriteHeader(http.StatusNoContent)
			},
		},
		{
			name:   "issue clone",
			args:   []string{"issue", "clone", "PROJ-1..2", "-p", "2", "--force"},
			handle: bulkIssueCloneHandler,
		},
		{
			name:   "issue move",
			args:   []string{"issue", "move", "PROJ-1..2", "-p", "2", "--force"},
			handle: bulkIssueMethodHandler(http.MethodPut),
		},
		{
			name:   "issue delete",
			args:   []string{"issue", "delete", "PROJ-1..2", "-p", "2", "--force"},
			handle: bulkIssueMethodHandler(http.MethodDelete),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var current atomic.Int32
			var peak atomic.Int32
			var requests atomic.Int32
			hit := func(key string) {
				if key != "PROJ-1" && key != "PROJ-2" {
					t.Errorf("unexpected counted key %q", key)
				}
				requests.Add(1)
				now := current.Add(1)
				for {
					old := peak.Load()
					if now <= old || peak.CompareAndSwap(old, now) {
						break
					}
				}
				time.Sleep(25 * time.Millisecond)
				current.Add(-1)
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				r.Body = io.NopCloser(bytes.NewReader(body))
				ctx := r.Context()
				ctx = contextWithResponseWriter(ctx, w)
				tt.handle(t, r.WithContext(ctx), body, hit)
			}))
			defer srv.Close()

			cmd := exec.Command(bin, append([]string{"--config", jiraConfig(t, srv.URL), "--output=json"}, tt.args...)...)
			cmd.Env = append(os.Environ(), "JIRA_TOKEN_DEFAULT=test-token")
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("%s error = %v\nstdout=%s\nstderr=%s", tt.name, err, stdout.String(), stderr.String())
			}

			var env struct {
				OK   bool `json:"ok"`
				Data struct {
					Succeeded int `json:"succeeded"`
					Failed    int `json:"failed"`
					Results   []struct {
						Key  string          `json:"key"`
						OK   bool            `json:"ok"`
						Data json.RawMessage `json:"data"`
					} `json:"results"`
				} `json:"data"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
				t.Fatalf("%s stdout envelope is not JSON: %v\nstdout=%s\nstderr=%s", tt.name, err, stdout.String(), stderr.String())
			}
			if !env.OK || env.Data.Succeeded != 2 || env.Data.Failed != 0 || len(env.Data.Results) != 2 {
				t.Fatalf("%s summary = ok %v succeeded %d failed %d results %d\n%s",
					tt.name, env.OK, env.Data.Succeeded, env.Data.Failed, len(env.Data.Results), stdout.String())
			}
			if env.Data.Results[0].Key != "PROJ-1" || env.Data.Results[1].Key != "PROJ-2" ||
				!env.Data.Results[0].OK || !env.Data.Results[1].OK ||
				len(env.Data.Results[0].Data) == 0 || len(env.Data.Results[1].Data) == 0 {
				t.Fatalf("%s results = %+v", tt.name, env.Data.Results)
			}
			if requests.Load() != 2 {
				t.Fatalf("%s counted requests = %d, want one per expanded key", tt.name, requests.Load())
			}
			if peak.Load() < 2 {
				t.Fatalf("%s peak concurrency = %d, want -p 2 to allow two in-flight key operations", tt.name, peak.Load())
			}
		})
	}
}

type httpResponseWriterKey struct{}

func contextWithResponseWriter(ctx context.Context, w http.ResponseWriter) context.Context {
	return context.WithValue(ctx, httpResponseWriterKey{}, w)
}

func bulkWatcherHandler(method string, needsMyself bool) func(*testing.T, *http.Request, []byte, func(string)) {
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
		hit(key)
		r.Context().Value(httpResponseWriterKey{}).(http.ResponseWriter).WriteHeader(http.StatusNoContent)
	}
}

func bulkIssueMethodHandler(method string) func(*testing.T, *http.Request, []byte, func(string)) {
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
		hit(key)
		if method == http.MethodDelete {
			r.Context().Value(httpResponseWriterKey{}).(http.ResponseWriter).WriteHeader(http.StatusNoContent)
			return
		}
		_, _ = io.WriteString(r.Context().Value(httpResponseWriterKey{}).(http.ResponseWriter), `{"id":"`+key+`","key":"`+key+`","fields":{"summary":"bulk"}}`)
	}
}

func bulkIssueEditHandler(t *testing.T, r *http.Request, _ []byte, hit func(string)) {
	if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/editmeta") {
		_, _ = io.WriteString(r.Context().Value(httpResponseWriterKey{}).(http.ResponseWriter), bulkEditMetaBody())
		return
	}
	key, ok := bulkIssuePath(r.URL.Path)
	if !ok || r.Method != http.MethodPut {
		bulkUnexpected(t, r)
		return
	}
	hit(key)
	_, _ = io.WriteString(r.Context().Value(httpResponseWriterKey{}).(http.ResponseWriter), `{"id":"`+key+`","key":"`+key+`","fields":{"summary":"Bulk edit"}}`)
}

func bulkIssueCloneHandler(t *testing.T, r *http.Request, body []byte, hit func(string)) {
	if r.Method == http.MethodGet {
		key, ok := bulkIssuePath(r.URL.Path)
		if !ok {
			bulkUnexpected(t, r)
			return
		}
		hit(key)
		_, _ = io.WriteString(r.Context().Value(httpResponseWriterKey{}).(http.ResponseWriter), `{"id":"`+key+`","key":"`+key+`","fields":{"summary":"source `+key+`","project":{"key":"PROJ"},"issuetype":{"name":"Task"}}}`)
		return
	}
	if r.Method != http.MethodPost || r.URL.Path != "/rest/api/3/issue" {
		bulkUnexpected(t, r)
		return
	}
	key := "CLONE"
	if strings.Contains(string(body), "PROJ-1") {
		key = "CLONE-1"
	} else if strings.Contains(string(body), "PROJ-2") {
		key = "CLONE-2"
	}
	_, _ = io.WriteString(r.Context().Value(httpResponseWriterKey{}).(http.ResponseWriter), `{"id":"`+key+`","key":"`+key+`","fields":{"summary":"clone"}}`)
}

func bulkEditMetaBody() string {
	return `{"fields":{"summary":{"name":"Summary","key":"summary","schema":{"type":"string"}},"project":{"name":"Project","key":"project","schema":{"type":"project"}},"issuetype":{"name":"Issue Type","key":"issuetype","schema":{"type":"issuetype"}}}}`
}

func bulkNestedIssuePath(r *http.Request, leaf string) (string, bool) {
	prefix := "/rest/api/3/issue/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		return "", false
	}
	rest := strings.TrimPrefix(r.URL.Path, prefix)
	suffix := "/" + leaf
	if !strings.HasSuffix(rest, suffix) {
		return "", false
	}
	key := strings.TrimSuffix(rest, suffix)
	return key, strings.HasPrefix(key, "PROJ-")
}

func bulkIssuePath(path string) (string, bool) {
	prefix := "/rest/api/3/issue/"
	if !strings.HasPrefix(path, prefix) || strings.TrimPrefix(path, prefix) == "" {
		return "", false
	}
	key := strings.TrimPrefix(path, prefix)
	if strings.Contains(key, "/") {
		return "", false
	}
	return key, strings.HasPrefix(key, "PROJ-")
}

func bulkJSONIssueKey(t *testing.T, body []byte, field string) string {
	t.Helper()
	var payload map[string]map[string]string
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode link body: %v\n%s", err, body)
	}
	key := payload[field]["key"]
	if key == "" {
		t.Fatalf("link body missing %s.key: %s", field, body)
	}
	return key
}

func bulkUnexpected(t *testing.T, r *http.Request) {
	t.Helper()
	t.Errorf("unexpected request %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
	r.Context().Value(httpResponseWriterKey{}).(http.ResponseWriter).WriteHeader(http.StatusInternalServerError)
}
