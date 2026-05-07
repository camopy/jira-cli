package contract

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
)

func TestRawIssueViewOutputsJiraNativeIssueShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/rest/api/3/issue/PROJ-1" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":"10001","key":"PROJ-1","fields":{"summary":"Native issue","description":{"type":"doc","version":1,"content":[]}}}`))
	}))
	defer srv.Close()

	cmd := exec.Command("go", "run", "../../cmd/jira", "--config", jiraConfig(t, srv.URL), "--raw", "issue", "view", "PROJ-1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("issue view --raw error = %v\n%s", err, out)
	}
	if strings.Contains(string(out), `"meta"`) || strings.Contains(string(out), `"errors"`) || strings.Contains(string(out), `"issue"`) {
		t.Fatalf("raw issue view should not emit CLI envelope/wrapper:\n%s", out)
	}
	var raw struct {
		ID     string `json:"id"`
		Key    string `json:"key"`
		Fields struct {
			Summary     string         `json:"summary"`
			Description map[string]any `json:"description"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		t.Fatalf("raw issue output is not JSON: %v\n%s", err, out)
	}
	if raw.ID != "10001" || raw.Key != "PROJ-1" || raw.Fields.Summary != "Native issue" || raw.Fields.Description["type"] != "doc" {
		t.Fatalf("raw issue shape = %+v\n%s", raw, out)
	}
}

func TestRawIssueListOutputsJiraSearchShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/rest/api/3/search/jql" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"isLast":true,"maxResults":50,"total":1,"issues":[{"id":"10001","key":"PROJ-1","fields":{"summary":"Native issue","description":{"type":"doc","version":1,"content":[]}}}]}`))
	}))
	defer srv.Close()

	cmd := exec.Command("go", "run", "../../cmd/jira", "--config", jiraConfig(t, srv.URL), "--raw", "issue", "list")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("issue list --raw error = %v\n%s", err, out)
	}
	if strings.Contains(string(out), `"meta"`) || strings.Contains(string(out), `"errors"`) || strings.Contains(string(out), `"detail"`) || strings.Contains(string(out), `"jql"`) {
		t.Fatalf("raw issue list should not emit CLI envelope or CLI fields:\n%s", out)
	}
	var raw struct {
		IsLast     bool `json:"isLast"`
		MaxResults int  `json:"maxResults"`
		Total      int  `json:"total"`
		Issues     []struct {
			ID     string `json:"id"`
			Key    string `json:"key"`
			Fields struct {
				Summary     string         `json:"summary"`
				Description map[string]any `json:"description"`
			} `json:"fields"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		t.Fatalf("raw issue list output is not JSON: %v\n%s", err, out)
	}
	if !raw.IsLast || raw.MaxResults != 50 || raw.Total != 1 || len(raw.Issues) != 1 || raw.Issues[0].ID != "10001" || raw.Issues[0].Fields.Summary != "Native issue" {
		t.Fatalf("raw issue list shape = %+v\n%s", raw, out)
	}
}
