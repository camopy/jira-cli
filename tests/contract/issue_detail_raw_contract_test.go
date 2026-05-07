package contract

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"
)

func TestIssueListDetailFetchesFullIssueRecords(t *testing.T) {
	var getCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/rest/api/3/search/jql":
			_, _ = w.Write([]byte(`{"isLast":true,"maxResults":50,"total":1,"issues":[{"key":"PROJ-1","fields":{"summary":"Summary only"}}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/rest/api/3/issue/PROJ-1":
			getCount++
			_, _ = w.Write([]byte(`{"id":"10001","key":"PROJ-1","fields":{"summary":"Full issue","comment":{"comments":[{"id":"c1"}]},"worklog":{"worklogs":[{"id":"w1","timeSpentSeconds":300}]}}}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()

	cmd := exec.Command("go", "run", "../../cmd/jira", "--config", jiraConfig(t, srv.URL), "--json", "issue", "list", "--detail")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("issue list --detail error = %v\n%s", err, out)
	}
	if getCount != 1 {
		t.Fatalf("issue list --detail made %d detail requests, want 1", getCount)
	}
	var env struct {
		Data struct {
			Issues []struct {
				ID       string `json:"id"`
				Key      string `json:"key"`
				Comments []struct {
					ID string `json:"id"`
				} `json:"comments"`
				Worklogs []struct {
					ID string `json:"id"`
				} `json:"worklogs"`
			} `json:"issues"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("decode envelope: %v\n%s", err, out)
	}
	if len(env.Data.Issues) != 1 || env.Data.Issues[0].ID != "10001" || len(env.Data.Issues[0].Comments) != 1 || len(env.Data.Issues[0].Worklogs) != 1 {
		t.Fatalf("detail output did not include fetched full record: %+v\n%s", env.Data.Issues, out)
	}
}

func TestRawOutputUsesCapturedJiraResponseBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/rest/api/3/search/jql" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
		_, _ = w.Write([]byte(`{"isLast":true,"maxResults":50,"total":1,"warningMessages":["native"],"issues":[{"id":"10001","key":"PROJ-1","fields":{"summary":"Native"}}]}`))
	}))
	defer srv.Close()

	cmd := exec.Command("go", "run", "../../cmd/jira", "--config", jiraConfig(t, srv.URL), "--raw", "issue", "list")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("issue list --raw error = %v\n%s", err, out)
	}
	var raw map[string]any
	if err := json.Unmarshal(out, &raw); err != nil {
		t.Fatalf("raw output is not JSON: %v\n%s", err, out)
	}
	if _, ok := raw["warningMessages"]; !ok {
		t.Fatalf("raw output lost Jira-native fields: %+v\n%s", raw, out)
	}
	if _, ok := raw["pagination"]; ok {
		t.Fatalf("raw output included CLI abstraction fields: %+v\n%s", raw, out)
	}
}
