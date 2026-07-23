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
	"sync/atomic"
	"testing"
)

func TestDestructiveIssueCommandsRequireForceOrDryRun(t *testing.T) {
	for _, sub := range []string{"clone", "move", "delete"} {
		cmd := exec.Command(buildJiraBinary(t), "issue", sub, "PROJ-1")
		if err := cmd.Run(); err == nil {
			t.Fatalf("issue %s without --force/--dry-run succeeded", sub)
		}
		cmd = exec.Command(buildJiraBinary(t), "issue", sub, "PROJ-1", "--dry-run", "--no-input")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("issue %s dry-run error = %v\n%s", sub, err, out)
		}
	}
}

func TestDeleteSubtasksFlagOnlyAppearsOnDelete(t *testing.T) {
	for _, sub := range []string{"clone", "move"} {
		cmd := exec.Command(buildJiraBinary(t), "issue", sub, "--help")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("issue %s --help error = %v\n%s", sub, err, out)
		}
		if strings.Contains(string(out), "--delete-subtasks") {
			t.Fatalf("issue %s help lists delete-only flag:\n%s", sub, out)
		}
	}

	cmd := exec.Command(buildJiraBinary(t), "issue", "delete", "--help")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("issue delete --help error = %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "--delete-subtasks") {
		t.Fatalf("issue delete help should keep --delete-subtasks:\n%s", out)
	}
}

func TestIssueMoveCanonicalProjectIssueTypePayloadBypassesSourceEditScreen(t *testing.T) {
	var updateCalled atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/rest/api/3/issue/PROJ-1/editmeta" {
			t.Errorf("issue move should not validate project/issuetype against the source edit screen")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if r.Method != http.MethodPut || r.URL.Path != "/rest/api/3/issue/PROJ-1" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		updateCalled.Store(true)
		var body struct {
			Fields map[string]any `json:"fields"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode move body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		project, _ := body.Fields["project"].(map[string]any)
		issueType, _ := body.Fields["issuetype"].(map[string]any)
		if project["key"] != "JCT" || issueType["name"] != "Task" {
			t.Errorf("move body lost canonical project/issuetype fields: %+v", body.Fields)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"key":"PROJ-1"}`))
	}))
	defer srv.Close()

	input := filepath.Join(t.TempDir(), "move.json")
	if err := os.WriteFile(input, []byte(`{"fields":{"project":{"key":"JCT"},"issuetype":{"name":"Task"}}}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cmd := exec.Command(buildJiraBinary(t), "--config", jiraConfig(t, srv.URL), "issue", "move", "PROJ-1", "--force", "--no-input", "--json-input", input, "--output=json")
	cmd.Env = append(os.Environ(), "JIRA_TOKEN_DEFAULT=test-token")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		t.Fatalf("issue move canonical payload error = %v\nstdout:\n%s\nstderr:\n%s", err, stdout.Bytes(), stderr.Bytes())
	}
	if !updateCalled.Load() {
		t.Fatal("issue move did not submit the canonical payload")
	}
	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode issue move envelope: %v\n%s", err, stdout.Bytes())
	}
	payload, ok := env.Data["payload"].(map[string]any)
	if !ok {
		t.Fatalf("live issue move omitted validated payload: %s", stdout.Bytes())
	}
	fields, _ := payload["fields"].(map[string]any)
	project, _ := fields["project"].(map[string]any)
	issueType, _ := fields["issuetype"].(map[string]any)
	if project["key"] != "JCT" || issueType["name"] != "Task" {
		t.Fatalf("live issue move payload lost submitted fields: %#v", payload)
	}
}
