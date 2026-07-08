package contract

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
)

func TestIssueListDetailUsesSingleSearchRequest(t *testing.T) {
	assertIssueDetailUsesSingleSearchRequest(t, []string{"issue", "list", "--detail"})
}

func TestIssueMineDetailUsesSingleSearchRequest(t *testing.T) {
	assertIssueDetailUsesSingleSearchRequest(t, []string{"issue", "mine", "--detail"})
}

func assertIssueDetailUsesSingleSearchRequest(t *testing.T, commandArgs []string) {
	t.Helper()
	var searchCount int
	var searchBody struct {
		Fields []string `json:"fields"`
		Expand string   `json:"expand"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/rest/api/3/search/jql":
			searchCount++
			if err := json.NewDecoder(r.Body).Decode(&searchBody); err != nil {
				t.Fatalf("decode search body: %v", err)
			}
			_, _ = w.Write([]byte(`{"isLast":true,"maxResults":50,"total":1,"issues":[{"id":"10001","key":"PROJ-1","fields":{"summary":"Full issue","comment":{"comments":[{"id":"c1"}]},"worklog":{"worklogs":[{"id":"w1","timeSpentSeconds":300}]}}}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/rest/api/3/issue/PROJ-1":
			t.Fatalf("issue list --detail made an avoidable per-issue detail request: %s", r.URL.String())
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()

	args := append([]string{"--config", jiraConfig(t, srv.URL), "--output=json"}, commandArgs...)
	cmd := exec.Command(buildJiraBinary(t), args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s error = %v\n%s", strings.Join(commandArgs, " "), err, out)
	}
	if searchCount != 1 {
		t.Fatalf("%s made %d search requests, want 1", strings.Join(commandArgs, " "), searchCount)
	}
	if len(searchBody.Fields) != 1 || searchBody.Fields[0] != "*all" {
		t.Fatalf("detail search fields = %#v, want [*all]", searchBody.Fields)
	}
	for _, want := range []string{"renderedFields", "names", "schema", "transitions", "operations", "changelog"} {
		if !strings.Contains(searchBody.Expand, want) {
			t.Fatalf("detail search expand = %q, missing %q", searchBody.Expand, want)
		}
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
