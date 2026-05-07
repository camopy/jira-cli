package contract

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
)

func TestJSONFlagForcesEnvelopeEvenForDetectedAgents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/rest/api/3/search/jql" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"isLast":true,"issues":[{"key":"PROJ-2","fields":{"summary":"Search hit"}}]}`))
	}))
	defer srv.Close()

	bin := buildJiraBinary(t)
	cmd := exec.Command(bin, "--config", jiraConfig(t, srv.URL), "--json", "search", "jql", "project = PROJ")
	cmd.Env = append(cmd.Environ(), "CLAUDE_CODE=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("agent --json command error = %v\n%s", err, out)
	}
	env := decodeEnvelope(t, out)
	if env.Meta.Command != "search.jql" || env.Data == nil {
		t.Fatalf("agent --json did not emit envelope: %+v\n%s", env, out)
	}
}

func TestJSONFlagConflictsWithAlternateOutputModes(t *testing.T) {
	bin := buildJiraBinary(t)
	for _, args := range [][]string{
		{"--json", "--compact", "schema"},
		{"--json", "--plain", "schema"},
		{"--json", "--raw", "schema"},
	} {
		args := args
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			cmd := exec.Command(bin, args...)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			err := cmd.Run()
			if err == nil {
				t.Fatalf("jira %v succeeded (want non-zero exit)", args)
			}
			// clog diagnostic on stderr must name the conflict.
			stderrLow := strings.ToLower(stderr.String())
			if !strings.Contains(stderrLow, "err") || !strings.Contains(stderrLow, "flags") {
				t.Fatalf("jira %v stderr did not emit clog diagnostic error:\n%s", args, stderr.String())
			}
			// --json was requested so stdout must carry a JSON envelope
			// with the error in errors[].
			var env map[string]any
			if jsonErr := json.Unmarshal(stdout.Bytes(), &env); jsonErr != nil {
				t.Fatalf("jira %v stdout is not valid JSON: %v\nstdout=%s", args, jsonErr, stdout.String())
			}
			errs, _ := env["errors"].([]any)
			if len(errs) == 0 {
				t.Fatalf("jira %v stdout envelope.errors is empty:\nstdout=%s", args, stdout.String())
			}
		})
	}
}

func TestConfigCommandsEmitEnvelopeInHeadlessMode(t *testing.T) {
	path := t.TempDir() + "/config.toml"
	bin := buildJiraBinary(t)
	cmd := exec.Command(bin, "--config", path, "--json", "config", "init", "--no-input", "--profile", "default", "--base-url", "https://company.atlassian.net", "--auth-type", "token", "--email", "dev@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("config init error = %v\n%s", err, out)
	} else if env := decodeEnvelope(t, out); env.Meta.Command != "config.init" {
		t.Fatalf("config init envelope = %+v", env)
	}

	cmd = exec.Command(bin, "--config", path, "--json", "config", "get", "default_profile")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("config get error = %v\n%s", err, out)
	}
	env := decodeEnvelope(t, out)
	if env.Meta.Command != "config.get" {
		t.Fatalf("config get command = %q", env.Meta.Command)
	}
	data, ok := env.Data.(map[string]any)
	if !ok || data["key"] != "default_profile" || data["value"] != "default" {
		t.Fatalf("config get data = %#v", env.Data)
	}
}

func TestErrorDiagnosticUsesFailingCommandMessage(t *testing.T) {
	bin := buildJiraBinary(t)
	cmd := exec.Command(bin, "--json", "issue", "delete", "PROJ-1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatalf("issue delete without --force succeeded:\nstdout=%s", stdout.String())
	}
	// clog diagnostic on stderr must include the failing message.
	if !strings.Contains(stderr.String(), "issue delete requires") {
		t.Fatalf("stderr does not include failing command message:\nstderr=%s", stderr.String())
	}
	// --json was requested so stdout must carry a JSON envelope
	// with the error message in errors[].
	var env map[string]any
	if jsonErr := json.Unmarshal(stdout.Bytes(), &env); jsonErr != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout=%s", jsonErr, stdout.String())
	}
	errs, _ := env["errors"].([]any)
	if len(errs) == 0 {
		t.Fatalf("envelope.errors is empty; want error entry containing command message:\nstdout=%s", stdout.String())
	}
	first, _ := errs[0].(map[string]any)
	msg, _ := first["message"].(string)
	if !strings.Contains(msg, "issue delete requires") {
		t.Fatalf("envelope.errors[0].message does not match; got %q", msg)
	}
}

type testEnvelope struct {
	Meta struct {
		Command string `json:"command"`
		Profile string `json:"profile"`
	} `json:"meta"`
	Data   any `json:"data"`
	Errors []struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"errors"`
}

func decodeEnvelope(t *testing.T, out []byte) testEnvelope {
	t.Helper()
	var env testEnvelope
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("output is not a JSON envelope: %v\n%s", err, out)
	}
	if env.Meta.Command == "" {
		t.Fatalf("output missing envelope metadata:\n%s", out)
	}
	return env
}
