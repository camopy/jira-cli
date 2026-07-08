package contract

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
)

// issueViewADFServer serves a single issue whose description carries a
// placeholder node — a construct the ADF->Markdown renderer cannot fully
// express, so the HUMAN renderer drops detail rendering it.
func issueViewADFServer(t *testing.T) *httptest.Server {
	t.Helper()
	const issue = `{
		"id": "10001", "key": "PROJ-1",
		"fields": {
			"summary": "card issue",
			"customfield_10000": {"value": "Raw option"},
			"description": {
				"type": "doc", "version": 1,
				"content": [
					{"type": "paragraph", "content": [
						{"type": "text", "text": "see "},
						{"type": "placeholder", "attrs": {"text": "p"}}
					]}
				]
			}
		}
	}`
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/rest/api/3/issue/PROJ-1") {
			_, _ = w.Write([]byte(issue))
			return
		}
		t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
}

// The json envelope carries the full ADF in data.issue, so nothing is
// lost on the machine path — it MUST NOT carry an adf_lossy_render
// render-loss warning (the warning would be false there).
func TestIssueViewJSONDoesNotEmitFalseADFWarning(t *testing.T) {
	srv := issueViewADFServer(t)
	defer srv.Close()
	t.Setenv("JIRA_TOKEN_DEFAULT", "test-token")

	cmd := exec.Command(buildJiraBinary(t), "--config", jiraConfig(t, srv.URL),
		"--output=json", "issue", "view", "PROJ-1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("issue view --output=json error = %v\n%s", err, out)
	}
	var env struct {
		Warnings []struct {
			Type string `json:"type"`
		} `json:"warnings"`
		Data struct {
			Issue struct {
				Key    string `json:"key"`
				Fields struct {
					Summary     string          `json:"summary"`
					CustomField json.RawMessage `json:"customfield_10000"`
					Description json.RawMessage `json:"description"`
				} `json:"fields"`
			} `json:"issue"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("envelope not JSON: %v\n%s", err, out)
	}
	for _, w := range env.Warnings {
		if w.Type == "adf_lossy_render" {
			t.Fatalf("json issue view emitted a false adf_lossy_render warning:\n%s", out)
		}
	}
	// The full ADF must still be present in the data payload.
	if !strings.Contains(string(env.Data.Issue.Fields.Description), "placeholder") {
		t.Fatalf("json issue view did not carry the full ADF: %s", out)
	}
	if env.Data.Issue.Key != "PROJ-1" || env.Data.Issue.Fields.Summary != "card issue" {
		t.Fatalf("json issue view did not preserve common fields under data.issue.fields: %+v\n%s", env.Data.Issue, out)
	}
	if !strings.Contains(string(env.Data.Issue.Fields.CustomField), "Raw option") {
		t.Fatalf("json issue view did not preserve raw custom field IDs under data.issue.fields: %+v\n%s", env.Data.Issue.Fields.CustomField, out)
	}
}

// Human output flattens ADF to text and genuinely loses the placeholder
// detail, so it MUST surface the render-loss warning on stderr.
func TestIssueViewHumanEmitsRealADFWarning(t *testing.T) {
	srv := issueViewADFServer(t)
	defer srv.Close()
	t.Setenv("JIRA_TOKEN_DEFAULT", "test-token")

	cmd := exec.Command(buildJiraBinary(t), "--config", jiraConfig(t, srv.URL),
		"--output=human", "issue", "view", "PROJ-1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("issue view --output=human error = %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "adf_lossy_render") && !strings.Contains(string(out), "placeholder") {
		t.Fatalf("human issue view did not surface the real ADF render-loss warning:\n%s", out)
	}
}
