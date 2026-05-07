package contract

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestIssueEditDryRunEditNoInputContract(t *testing.T) {
	// --no-input must include at least one field to mutate; --summary is
	// the cheapest field to exercise the dry-run envelope path. An empty
	// edit under --no-input is now a validation error.
	cmd := exec.Command("go", "run", "../../cmd/jira",
		"issue", "edit", "PROJ-1", "--dry-run", "--no-input",
		"--summary", "renamed", "--json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("issue edit error = %v\n%s", err, out)
	}
	var env map[string]any
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("issue edit output is not JSON: %v\n%s", err, out)
	}
	if env["data"] == nil {
		t.Fatalf("issue edit missing data: %+v", env)
	}
}

// JIRA_EDITOR env var MUST override $EDITOR. Test boots two editor
// scripts that write distinct sentinels into the temp file and asserts
// the JIRA_EDITOR-set sentinel reaches Jira.
func TestIssueEditJiraEditorEnvOverridesEditor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/rest/api/3/issue/PROJ-1":
			_, _ = w.Write([]byte(`{"key":"PROJ-1","fields":{"description":{"type":"doc","version":1,"content":[]}}}`))
		case r.Method == http.MethodPut && r.URL.Path == "/rest/api/3/issue/PROJ-1":
			body, _ := io.ReadAll(r.Body)
			// Assert the body content came from the JIRA_EDITOR script
			// (sentinel: "from-jira-editor"), not the EDITOR script
			// (sentinel: "from-editor-env").
			if !strings.Contains(string(body), "from-jira-editor") {
				t.Errorf("update body missing JIRA_EDITOR sentinel: %s", body)
			}
			if strings.Contains(string(body), "from-editor-env") {
				t.Errorf("update body picked up $EDITOR sentinel — JIRA_EDITOR didn't override: %s", body)
			}
			_, _ = w.Write([]byte(`{"key":"PROJ-1"}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	jiraEditorScript := filepath.Join(t.TempDir(), "jira-editor.sh")
	editorEnvScript := filepath.Join(t.TempDir(), "editor-env.sh")
	for _, sp := range []struct {
		path, sentinel string
	}{
		{jiraEditorScript, "from-jira-editor"},
		{editorEnvScript, "from-editor-env"},
	} {
		body := "#!/bin/sh\ncat > \"$1\" <<'EOF'\n---\nissue: PROJ-1\nfield: description\n---\n\n" + sp.sentinel + "\nEOF\n"
		if err := os.WriteFile(sp.path, []byte(body), 0o700); err != nil {
			t.Fatalf("write editor script %s: %v", sp.path, err)
		}
	}

	cmd := exec.Command("go", "run", "../../cmd/jira", "--config", jiraConfig(t, srv.URL), "issue", "edit", "PROJ-1", "--json")
	cmd.Env = append(cmd.Environ(),
		"JIRA_EDITOR="+jiraEditorScript,
		"EDITOR="+editorEnvScript,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("issue edit error = %v\n%s", err, out)
	}
}

// kubectl-style: bare `jira issue edit KEY` (no field flags, no
// --json-input) opens the configured external editor on the description.
// The previous --edit flag is gone.
func TestIssueEditBareCommandOpensEditorAndUpdatesADF(t *testing.T) {
	var gotGet, gotPut bool
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/rest/api/3/issue/PROJ-1":
			gotGet = true
			_, _ = w.Write([]byte(`{
				"key": "PROJ-1",
				"fields": {
					"description": {
						"type": "doc",
						"version": 1,
						"content": [
							{"type": "paragraph", "content": [{"type": "text", "text": "old body"}]}
						]
					}
				}
			}`))
		case r.Method == http.MethodPut && r.URL.Path == "/rest/api/3/issue/PROJ-1":
			gotPut = true
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("decode update body: %v", err)
			}
			_, _ = w.Write([]byte(`{"key":"PROJ-1"}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	editorPath := filepath.Join(t.TempDir(), "editor.sh")
	editorScript := `#!/bin/sh
cat > "$1" <<'EOF'
---
issue: PROJ-1
field: description
---

new **body**
EOF
`
	if err := os.WriteFile(editorPath, []byte(editorScript), 0o700); err != nil {
		t.Fatalf("write editor script: %v", err)
	}

	cmd := exec.Command("go", "run", "../../cmd/jira", "--config", jiraConfig(t, srv.URL), "issue", "edit", "PROJ-1", "--json")
	cmd.Env = append(cmd.Environ(), "EDITOR="+editorPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("issue edit error = %v\n%s", err, out)
	}
	if !gotGet {
		t.Fatal("bare issue edit did not fetch current issue description")
	}
	if !gotPut {
		t.Fatal("bare issue edit did not update Jira issue")
	}
	fields, ok := got["fields"].(map[string]any)
	if !ok {
		t.Fatalf("update body missing fields: %+v", got)
	}
	description, ok := fields["description"].(map[string]any)
	if !ok || description["type"] != "doc" {
		t.Fatalf("description update is not ADF: %+v", fields["description"])
	}
	raw, err := json.Marshal(description)
	if err != nil {
		t.Fatalf("marshal updated description: %v", err)
	}
	if !strings.Contains(string(raw), "new") || !strings.Contains(string(raw), "body") {
		t.Fatalf("updated ADF does not contain edited markdown text: %s", raw)
	}
}
