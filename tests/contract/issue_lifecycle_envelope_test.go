package contract

// Every new command added by the issue-lifecycle work MUST emit an
// envelope with `meta.command`, `meta.profile`, `meta.timestamp`, and
// `meta.request_id` populated. The cross-cutting test below drives one
// happy-path invocation per command against a single multiplexed httptest
// server and asserts the four meta fields land in the JSON envelope.
//
// Failure here means an envelope writer regressed — usually because a new
// dispatcher path forgot to call writeEnvelope / writeEnvelopeWith*.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// envelopeFixtureServer returns a mux that fakes Atlassian's REST surface
// for every command exercised by the cross-cutting envelope test below.
// Bodies are minimal but valid for the CLI's parsing path.
func envelopeFixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/api/3/myself", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accountId":"712020:test-user","emailAddress":"user@example.com","displayName":"Test","active":true}`))
	})
	mux.HandleFunc("/rest/api/3/user/search", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"accountId":"a-1","displayName":"Alice","active":true}]`))
	})
	mux.HandleFunc("/rest/api/3/issue/PROJ-1", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"key":"PROJ-1","fields":{"attachment":[]}}`))
	})
	mux.HandleFunc("/rest/api/3/issue/PROJ-1/attachments", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"99","filename":"x.bin","mimeType":"application/octet-stream","size":1,
			"author":{"accountId":"a","displayName":"A"},"created":"2026-05-01T00:00:00.000+0000",
			"content":"http://x/99","self":"http://x/99"}]`))
	})
	mux.HandleFunc("/rest/api/3/attachment/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/rest/api/3/issue/PROJ-1/comment", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			_, _ = w.Write([]byte(`{"id":"500","body":{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"hi"}]}]},"created":"2026-05-01T00:00:00.000+0000","updated":"2026-05-01T00:00:00.000+0000"}`))
		default:
			_, _ = w.Write([]byte(`{"comments":[],"startAt":0,"maxResults":50,"total":0,"isLast":true}`))
		}
	})
	mux.HandleFunc("/rest/api/3/issue/PROJ-1/comment/500", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_, _ = w.Write([]byte(`{"id":"500","body":{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"updated"}]}]},"created":"2026-05-01T00:00:00.000+0000","updated":"2026-05-01T00:00:00.000+0000"}`))
	})
	mux.HandleFunc("/rest/api/3/issue/PROJ-1/watchers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost, http.MethodDelete:
			_, _ = w.Write([]byte(`{"isWatching":true,"watchCount":1,"watchers":[{"accountId":"712020:test-user","displayName":"Test","active":true}]}`))
		default:
			_, _ = w.Write([]byte(`{"isWatching":true,"watchCount":1,"watchers":[{"accountId":"712020:test-user","displayName":"Test","active":true}]}`))
		}
	})
	mux.HandleFunc("/rest/api/3/issue/PROJ-1/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"key":"PROJ-1","fields":{"issuelinks":[]}}`))
	})
	mux.HandleFunc("/rest/api/3/issueLink/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/rest/api/3/issueLinkType", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"issueLinkTypes":[{"id":"10000","name":"Blocks","inward":"is blocked by","outward":"blocks"}]}`))
	})
	// 003 boards endpoints — paged board list + per-board projects.
	mux.HandleFunc("/rest/agile/1.0/board", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"maxResults":50,"startAt":0,"isLast":true,"values":[{"id":42,"name":"Engineering Sprint","type":"scrum"}]}`))
	})
	mux.HandleFunc("/rest/agile/1.0/board/42/project", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"maxResults":50,"startAt":0,"isLast":true,"values":[{"key":"ENG"}]}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// TestLifecycleCommandsEmitMetaEnvelope drives one happy-path invocation
// per new command and asserts the meta fields all land in the envelope.
func TestLifecycleCommandsEmitMetaEnvelope(t *testing.T) {
	srv := envelopeFixtureServer(t)
	cfg := jiraConfig(t, srv.URL)
	t.Setenv("JIRA_TOKEN_DEFAULT", "test-token")

	tmpFile := filepath.Join(t.TempDir(), "x.bin")
	if err := os.WriteFile(tmpFile, []byte("a"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	bodyJSON := filepath.Join(t.TempDir(), "body.json")
	if err := os.WriteFile(bodyJSON, []byte(`{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"hi"}]}]}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cases := []struct {
		name string
		args []string
		want string // expected meta.command
	}{
		{"attachment list", []string{"issue", "attachment", "list", "PROJ-1"}, "issue.attachment.list"},
		{"attachment add", []string{"issue", "attachment", "add", "PROJ-1", "--file", tmpFile}, "issue.attachment.add"},
		{"attachment delete", []string{"issue", "attachment", "delete", "PROJ-1", "99", "--force"}, "issue.attachment.delete"},
		{"comment list", []string{"issue", "comment", "list", "PROJ-1"}, "issue.comment.list"},
		{"comment add", []string{"issue", "comment", "add", "PROJ-1", "--json-input", bodyJSON, "--no-input"}, "issue.comment.add"},
		{"comment edit", []string{"issue", "comment", "edit", "PROJ-1", "500", "--json-input", bodyJSON, "--no-input"}, "issue.comment.edit"},
		{"comment delete", []string{"issue", "comment", "delete", "PROJ-1", "500", "--force"}, "issue.comment.delete"},
		{"watchers list", []string{"issue", "watchers", "list", "PROJ-1"}, "issue.watchers.list"},
		{"watchers add", []string{"issue", "watchers", "add", "PROJ-1", "--user", "me"}, "issue.watchers.add"},
		{"link types", []string{"issue", "link", "types"}, "issue.link.types"},
		// 003: boards cache + listing + --board-scoped issue list.
		{"boards list", []string{"boards", "list"}, "boards.list"},
		{"cache boards", []string{"cache", "boards"}, "cache.boards"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"--config", cfg, "--output=json"}, tc.args...)
			stdout, stderr, code := runJira(t, args...)
			if code != 0 {
				t.Fatalf("exit = %d; want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
			}
			var env map[string]any
			if err := json.Unmarshal(stdout, &env); err != nil {
				t.Fatalf("envelope not JSON: %v\n%s", err, stdout)
			}
			meta, ok := env["meta"].(map[string]any)
			if !ok {
				t.Fatalf("envelope.meta missing or wrong type: %s", stdout)
			}
			if got, _ := meta["command"].(string); got != tc.want {
				t.Errorf("meta.command = %q; want %q", got, tc.want)
			}
			// Machine envelopes omit meta.profile entirely — a command
			// that must report a profile puts it in command-specific data.
			if _, has := meta["profile"]; has {
				t.Errorf("meta.profile must not appear in a machine envelope")
			}
			ts, _ := meta["timestamp"].(string)
			if ts == "" {
				t.Errorf("meta.timestamp empty")
			} else if _, err := time.Parse(time.RFC3339, ts); err != nil {
				t.Errorf("meta.timestamp not RFC3339: %q (err=%v)", ts, err)
			}
			if reqID, _ := meta["request_id"].(string); strings.TrimSpace(reqID) == "" {
				t.Errorf("meta.request_id empty")
			}
		})
	}
}
