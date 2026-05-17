package contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/pkg/jira"
)

func TestCommandsUseConfiguredJiraServices(t *testing.T) {
	var seenJQL []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/rest/api/3/search/jql":
			var body struct {
				JQL string `json:"jql"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode search body: %v", err)
			}
			seenJQL = append(seenJQL, body.JQL)
			switch body.JQL {
			case jira.DefaultIssueListJQL:
				_, _ = w.Write([]byte(`{"isLast":true,"issues":[{"key":"PROJ-1","fields":{"summary":"From server","status":{"name":"To Do"},"priority":{"name":"High"},"updated":"2026-05-03T10:00:00Z"}}]}`))
			case "project = CUSTOM":
				_, _ = w.Write([]byte(`{"isLast":true,"issues":[{"key":"CUSTOM-1","fields":{"summary":"Custom search"}}]}`))
			case "project = PROJ":
				_, _ = w.Write([]byte(`{"isLast":true,"issues":[{"key":"PROJ-2","fields":{"summary":"Search hit"}}]}`))
			default:
				t.Fatalf("unexpected JQL %q", body.JQL)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/rest/api/3/issue/PROJ-1/worklog":
			_, _ = w.Write([]byte(`{"worklogs":[{"id":"1","timeSpentSeconds":60}]}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	cfg := jiraConfig(t, srv.URL)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"issue", "list", "--output=json"}, `"key": "PROJ-1"`},
		{[]string{"issue", "list", "--jql", "project = CUSTOM", "--output=json"}, `"key": "CUSTOM-1"`},
		{[]string{"search", "jql", "project = PROJ", "--output=json"}, `"key": "PROJ-2"`},
		{[]string{"worklog", "list", "PROJ-1", "--output=json"}, `"timeSpentSeconds": 60`},
	} {
		cmd := exec.Command("go", append([]string{"run", "../../cmd/jira", "--config", cfg}, tc.args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("jira %v error = %v\n%s", tc.args, err, out)
		}
		if !strings.Contains(string(out), tc.want) {
			t.Fatalf("jira %v output missing %q:\n%s", tc.args, tc.want, out)
		}
	}
	for _, want := range []string{jira.DefaultIssueListJQL, "project = CUSTOM", "project = PROJ"} {
		if !slices.Contains(seenJQL, want) {
			t.Fatalf("missing JQL %q in %v", want, seenJQL)
		}
	}
}

func TestIssueCommentCommandConvertsMarkdownToADF(t *testing.T) {
	cmd := exec.Command("go", "run", "../../cmd/jira", "issue", "comment", "PROJ-1", "--body-markdown", "hello **world**", "--dry-run", "--no-input", "--output=json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("issue comment error = %v\n%s", err, out)
	}
	var env struct {
		Data struct {
			Comment struct {
				Body map[string]any `json:"body"`
			} `json:"comment"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("comment output is not JSON: %v\n%s", err, out)
	}
	if env.Data.Comment.Body["type"] != "doc" {
		t.Fatalf("comment body is not ADF: %+v", env.Data.Comment.Body)
	}
}

func TestJSONErrorsUseClogDiagnosticsAndExitCodes(t *testing.T) {
	bin := buildJiraBinary(t)
	cmd := exec.Command(bin, "worklog", "add", "PROJ-1", "--time-spent", "wat", "--output=json")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatalf("invalid duration succeeded:\nstdout=%s", stdout.String())
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 3 {
		t.Fatalf("exit = %v stdout=%s", err, stdout.String())
	}
	// clog diagnostic on stderr must mention "invalid duration".
	stderrLow := strings.ToLower(stderr.String())
	if !strings.Contains(stderrLow, "err") || !strings.Contains(stderrLow, "invalid duration") {
		t.Fatalf("stderr clog diagnostic missing:\nstderr=%s", stderr.String())
	}
	// --json path must deliver a JSON envelope on stdout with
	// the error in errors[].
	var env map[string]any
	if jsonErr := json.Unmarshal(stdout.Bytes(), &env); jsonErr != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout=%s", jsonErr, stdout.String())
	}
	errs, _ := env["errors"].([]any)
	if len(errs) == 0 {
		t.Fatalf("envelope.errors is empty:\nstdout=%s", stdout.String())
	}
	first, _ := errs[0].(map[string]any)
	if msg, _ := first["message"].(string); !strings.Contains(msg, "invalid duration") {
		t.Fatalf("envelope.errors[0].message = %q; want to contain %q", msg, "invalid duration")
	}
}

func TestSchemaIncludesFlagsAndOutputSchemas(t *testing.T) {
	cmd := exec.Command("go", "run", "../../cmd/jira", "agent", "schema")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("schema error = %v\n%s", err, out)
	}
	for _, want := range []string{`"flags"`, `"output_schemas"`, `"issue.list"`, `"issue.create"`, `"worklog.add"`, `"--dry-run"`} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("schema missing %q:\n%s", want, out)
		}
	}
}

func jiraConfig(t *testing.T, baseURL string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	content := `default_profile = "default"
queries_path = "` + filepath.ToSlash(t.TempDir()) + `/queries"

[[profiles]]
name = "default"
base_url = "` + baseURL + `"
auth_type = "token"
secret_backend = "keyring"
refresh_interval = 30
timeout = 30
workday_seconds = 28800
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}
