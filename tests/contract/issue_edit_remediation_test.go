package contract

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestIssueEditNoInputJSONInputDryRunShowsPreviewPayload(t *testing.T) {
	input := issueEditPayloadFile(t)
	cmd := exec.Command("go", "run", "../../cmd/jira", "issue", "edit", "PROJ-1", "--dry-run", "--no-input", "--json-input", input, "--output=json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("issue edit dry-run json-input error = %v\n%s", err, out)
	}

	var env struct {
		Data struct {
			Issue  string `json:"issue"`
			DryRun bool   `json:"dry_run"`
			Fields struct {
				Summary     string         `json:"summary"`
				Description map[string]any `json:"description"`
			} `json:"fields"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("issue edit dry-run output is not JSON: %v\n%s", err, out)
	}
	if env.Data.Issue != "PROJ-1" || !env.Data.DryRun {
		t.Fatalf("issue edit dry-run metadata = %+v", env.Data)
	}
	if env.Data.Fields.Summary != "Updated from JSON" {
		t.Fatalf("issue edit dry-run summary = %+v", env.Data.Fields)
	}
	if env.Data.Fields.Description["type"] != "doc" {
		t.Fatalf("issue edit dry-run description is not ADF: %+v", env.Data.Fields.Description)
	}
}

func TestIssueEditNoInputJSONInputCallsJiraUpdateWithFieldsAndADF(t *testing.T) {
	var called bool
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// issue edit resolves the issue's edit screen before validating
		// fields. The editmeta must declare the fields the payload sets,
		// otherwise strict-mode screen validation rejects them.
		if r.Method == http.MethodGet && r.URL.Path == "/rest/api/3/issue/PROJ-1/editmeta" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"fields":{"summary":{"name":"Summary","fieldId":"summary","required":true,"schema":{"type":"string"}},"description":{"name":"Description","fieldId":"description","required":false,"schema":{"type":"doc"}}}}`))
			return
		}
		if r.Method != http.MethodPut || r.URL.Path != "/rest/api/3/issue/PROJ-1" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		called = true
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"key":"PROJ-1"}`))
	}))
	defer srv.Close()

	cfg := jiraConfig(t, srv.URL)
	input := issueEditPayloadFile(t)
	cmd := exec.Command("go", "run", "../../cmd/jira", "--config", cfg, "issue", "edit", "PROJ-1", "--no-input", "--json-input", input, "--output=json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("issue edit json-input error = %v\n%s", err, out)
	}
	if !called {
		t.Fatal("issue edit did not call Jira update endpoint")
	}
	fields, ok := got["fields"].(map[string]any)
	if !ok {
		t.Fatalf("issue edit request missing fields object: %+v", got)
	}
	if fields["summary"] != "Updated from JSON" {
		t.Fatalf("issue edit request summary = %+v", fields["summary"])
	}
	description, ok := fields["description"].(map[string]any)
	if !ok || description["type"] != "doc" {
		t.Fatalf("issue edit request description is not ADF: %+v", fields["description"])
	}
}

func TestIssueEditJSONInputWithoutFieldsEnvelopeNamesPayloadShape(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "bad-edit.json")
	if err := os.WriteFile(input, []byte(`{"priority":{"name":"High"}}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd := exec.Command("go", "run", "../../cmd/jira", "issue", "edit", "PROJ-1", "--dry-run", "--no-input", "--json-input", input, "--output=json")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatalf("issue edit accepted json-input without fields envelope:\nstdout=%s\nstderr=%s", stdout.String(), stderr.String())
	}

	var env struct {
		Errors []struct {
			Message string `json:"message"`
			Hint    string `json:"hint"`
		} `json:"errors"`
	}
	decodeErrorEnvelopeFromStdout(t, stdout.Bytes(), stderr.Bytes(), cmd.Args, &env)
	if len(env.Errors) == 0 {
		t.Fatalf("issue edit error envelope has no errors:\nstdout=%s\nstderr=%s", stdout.String(), stderr.String())
	}
	msg := env.Errors[0].Message + " " + env.Errors[0].Hint
	if !strings.Contains(msg, `top-level "fields" object`) {
		t.Fatalf("error does not name fields-envelope requirement:\nstdout=%s\nstderr=%s", stdout.String(), stderr.String())
	}
	if strings.Contains(msg, "provide --json-input") {
		t.Fatalf("error still tells the user to provide --json-input they already provided:\nstdout=%s\nstderr=%s", stdout.String(), stderr.String())
	}
}

func TestIssueEditNoFieldInputKeepsDistinctMissingInputMessage(t *testing.T) {
	cmd := exec.Command("go", "run", "../../cmd/jira", "issue", "edit", "PROJ-1", "--dry-run", "--no-input", "--output=json")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("issue edit accepted empty no-input edit:\n%s", out)
	}
	if !strings.Contains(string(out), "provide --summary, --assignee, --markdown, or --json-input") {
		t.Fatalf("empty-input remediation changed or disappeared:\n%s", out)
	}
	if strings.Contains(string(out), `top-level "fields" object`) {
		t.Fatalf("empty-input path was conflated with malformed json-input:\n%s", out)
	}
}

func issueEditPayloadFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "edit.json")
	content := `{
  "fields": {
    "summary": "Updated from JSON",
    "description": {
      "type": "doc",
      "version": 1,
      "content": [
        {
          "type": "paragraph",
          "content": [
            {
              "type": "text",
              "text": "ADF body"
            }
          ]
        }
      ]
    }
  }
}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}
