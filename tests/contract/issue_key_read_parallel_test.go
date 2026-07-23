package contract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestIssueKeyReadCommandsAcceptDoubleDotRangesAndParallelism(t *testing.T) {
	bin := buildJiraBinary(t)
	tests := []struct {
		name  string
		args  []string
		match func(*http.Request) (string, bool)
		body  func(string) string
	}{
		{
			name: "watchers list",
			args: []string{"issue", "watchers", "list", "PROJ-1..2", "-p", "2"},
			match: func(r *http.Request) (string, bool) {
				return issueKeyFromNestedPath(r.URL.Path, "watchers")
			},
			body: func(string) string {
				return `{"isWatching":false,"watchCount":0,"watchers":[]}`
			},
		},
		{
			name: "attachment list",
			args: []string{"issue", "attachment", "list", "PROJ-1..2", "-p", "2"},
			match: func(r *http.Request) (string, bool) {
				key, ok := issueKeyFromIssuePath(r.URL.Path)
				return key, ok && r.URL.Query().Get("fields") == "attachment"
			},
			body: func(key string) string {
				return fmt.Sprintf(`{"id":"%[1]s","key":"%[1]s","fields":{"attachment":[]}}`, key)
			},
		},
		{
			name: "comment list",
			args: []string{"issue", "comment", "list", "PROJ-1..2", "-p", "2"},
			match: func(r *http.Request) (string, bool) {
				return issueKeyFromNestedPath(r.URL.Path, "comment")
			},
			body: func(string) string {
				return `{"comments":[],"startAt":0,"maxResults":50,"total":0,"isLast":true}`
			},
		},
		{
			name: "link list",
			args: []string{"issue", "link", "list", "PROJ-1..2", "-p", "2"},
			match: func(r *http.Request) (string, bool) {
				key, ok := issueKeyFromIssuePath(r.URL.Path)
				return key, ok && r.URL.Query().Get("fields") == "issuelinks"
			},
			body: func(key string) string {
				return fmt.Sprintf(`{"id":"%[1]s","key":"%[1]s","fields":{"issuelinks":[]}}`, key)
			},
		},
		{
			name: "worklog list",
			args: []string{"worklog", "list", "PROJ-1..2", "-p", "2"},
			match: func(r *http.Request) (string, bool) {
				return issueKeyFromNestedPath(r.URL.Path, "worklog")
			},
			body: func(string) string {
				return `{"worklogs":[],"startAt":0,"maxResults":50,"total":0}`
			},
		},
		{
			name: "transition list",
			args: []string{"issue", "transition", "PROJ-1..2", "-p", "2"},
			match: func(r *http.Request) (string, bool) {
				return issueKeyFromNestedPath(r.URL.Path, "transitions")
			},
			body: func(string) string {
				return `{"transitions":[{"id":"11","name":"Done"}]}`
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var current atomic.Int32
			var peak atomic.Int32
			var requests atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("unexpected method %s %s", r.Method, r.URL.String())
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				key, ok := tt.match(r)
				if !ok {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
					w.WriteHeader(http.StatusInternalServerError)
					return
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
				_, _ = w.Write([]byte(tt.body(key)))
			}))
			defer srv.Close()

			args := append([]string{"--config", jiraConfig(t, srv.URL), "--output=json"}, tt.args...)
			cmd := exec.Command(bin, args...)
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
				t.Fatalf("stdout envelope is not JSON: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
			}
			if !env.OK || env.Data.Succeeded != 2 || env.Data.Failed != 0 || len(env.Data.Results) != 2 {
				t.Fatalf("batch summary = ok %v succeeded %d failed %d results %d\n%s",
					env.OK, env.Data.Succeeded, env.Data.Failed, len(env.Data.Results), stdout.String())
			}
			if env.Data.Results[0].Key != "PROJ-1" || env.Data.Results[1].Key != "PROJ-2" ||
				!env.Data.Results[0].OK || !env.Data.Results[1].OK ||
				len(env.Data.Results[0].Data) == 0 || len(env.Data.Results[1].Data) == 0 {
				t.Fatalf("batch results = %+v", env.Data.Results)
			}
			if tt.name == "comment list" {
				for _, result := range env.Data.Results {
					var data struct {
						Issue struct {
							Key string `json:"key"`
						} `json:"issue"`
					}
					if err := json.Unmarshal(result.Data, &data); err != nil {
						t.Fatalf("decode %s data: %v\n%s", result.Key, err, result.Data)
					}
					if data.Issue.Key != result.Key {
						t.Fatalf("%s data.issue.key = %q, want matching result key", result.Key, data.Issue.Key)
					}
				}
			}
			if requests.Load() != 2 {
				t.Fatalf("requests = %d, want one request per expanded key", requests.Load())
			}
			if peak.Load() < 2 {
				t.Fatalf("peak concurrency = %d, want -p 2 to allow two in-flight requests", peak.Load())
			}
		})
	}
}

func issueKeyFromNestedPath(path, leaf string) (string, bool) {
	prefix := "/rest/api/3/issue/"
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	rest := strings.TrimPrefix(path, prefix)
	suffix := "/" + leaf
	if !strings.HasSuffix(rest, suffix) {
		return "", false
	}
	key := strings.TrimSuffix(rest, suffix)
	return key, strings.HasPrefix(key, "PROJ-")
}

func issueKeyFromIssuePath(path string) (string, bool) {
	prefix := "/rest/api/3/issue/"
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	key := strings.TrimPrefix(path, prefix)
	return key, strings.HasPrefix(key, "PROJ-")
}
