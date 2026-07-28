package contract

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"strings"
	"testing"
)

// issue view --fields must reach Jira as the fields query parameter, drop the
// edit-support expansions, and keep the documented data.issue envelope shape.
func TestIssueViewFieldsNarrowsRequestAndKeepsEnvelopeShape(t *testing.T) {
	var query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/rest/api/3/issue/PROJ-1") {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		query = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"id":"1","key":"PROJ-1","fields":{"summary":"narrowed","status":{"name":"Backlog"}}}`))
	}))
	defer srv.Close()
	t.Setenv("JIRA_TOKEN_DEFAULT", "test-token")

	bin := buildJiraBinary(t)
	cmd := exec.Command(bin, "--config", jiraConfig(t, srv.URL), "--output=json",
		"issue", "view", "PROJ-1", "--fields", "summary,status")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("issue view --fields error = %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}

	values, err := url.ParseQuery(query)
	if err != nil {
		t.Fatalf("request query %q did not parse: %v", query, err)
	}
	if got := values.Get("fields"); got != "summary,status" {
		t.Fatalf("fields query parameter = %q, want %q (raw query %q)", got, "summary,status", query)
	}
	expand := values.Get("expand")
	for _, banned := range []string{"transitions", "operations", "editmeta"} {
		if strings.Contains(expand, banned) {
			t.Fatalf("expand = %q, a --fields read must not request %q", expand, banned)
		}
	}

	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Issue struct {
				Key    string `json:"key"`
				Fields struct {
					Summary string `json:"summary"`
				} `json:"fields"`
			} `json:"issue"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout envelope is not JSON: %v\nstdout=%s", err, stdout.String())
	}
	if !env.OK || env.Data.Issue.Key != "PROJ-1" || env.Data.Issue.Fields.Summary != "narrowed" {
		t.Fatalf("envelope = ok %v issue %+v\n%s", env.OK, env.Data.Issue, stdout.String())
	}
}
