package contract

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
)

// TestEpicBoardCallsServiceWithPerEpicChildIssueCounts asserts that `epic board`
// is no longer a stub: with a configured client, it lists epics and computes
// {To Do, In Progress, Done} child-issue counts per epic.
func TestEpicBoardCallsServiceWithPerEpicChildIssueCounts(t *testing.T) {
	type recordedReq struct {
		path string
		jql  string
	}
	var requests []recordedReq

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var parsed struct {
			JQL string `json:"jql"`
		}
		_ = json.Unmarshal(body, &parsed)
		requests = append(requests, recordedReq{path: r.URL.Path, jql: parsed.JQL})

		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(parsed.JQL, "issuetype=Epic") || parsed.JQL == "issuetype=Epic":
			_, _ = w.Write([]byte(`{"isLast":true,"maxResults":50,"issues":[
				{"key":"EPIC-1","fields":{"summary":"Quarter goal","status":{"name":"In Progress"}}}
			]}`))
		case strings.Contains(parsed.JQL, "parent=EPIC-1"):
			_, _ = w.Write([]byte(`{"isLast":true,"maxResults":50,"issues":[
				{"key":"PROJ-1","fields":{"status":{"name":"To Do"}}},
				{"key":"PROJ-2","fields":{"status":{"name":"In Progress"}}},
				{"key":"PROJ-3","fields":{"status":{"name":"Done"}}},
				{"key":"PROJ-4","fields":{"status":{"name":"Done"}}}
			]}`))
		default:
			t.Errorf("unexpected request path=%s jql=%q", r.URL.Path, parsed.JQL)
			_, _ = w.Write([]byte(`{"isLast":true,"issues":[]}`))
		}
	}))
	defer srv.Close()

	cfg := jiraConfig(t, srv.URL)
	cmd := exec.Command("go", "run", "../../cmd/jira", "--config", cfg, "epic", "board", "--output=json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("epic board error = %v\n%s", err, out)
	}

	var env struct {
		Data struct {
			Epics []struct {
				Key     string         `json:"key"`
				Summary string         `json:"summary"`
				Status  string         `json:"status"`
				Counts  map[string]int `json:"counts"`
			} `json:"epics"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("epic board output is not JSON: %v\n%s", err, out)
	}

	if len(env.Data.Epics) != 1 {
		t.Fatalf("epic board returned %d epics, want 1: %+v", len(env.Data.Epics), env.Data.Epics)
	}
	got := env.Data.Epics[0]
	if got.Key != "EPIC-1" || got.Summary != "Quarter goal" || got.Status != "In Progress" {
		t.Fatalf("epic board epic = %+v", got)
	}
	wantCounts := map[string]int{"To Do": 1, "In Progress": 1, "Done": 2}
	for status, want := range wantCounts {
		if got.Counts[status] != want {
			t.Fatalf("epic board counts[%q] = %d, want %d (full counts: %+v)", status, got.Counts[status], want, got.Counts)
		}
	}
	if len(requests) < 2 {
		t.Fatalf("expected list + per-epic child-issue requests, got: %+v", requests)
	}
}
