package contract

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestIssueViewMultipleKeysPreservesOrderAndUsesParallelism(t *testing.T) {
	var current atomic.Int32
	var peak atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/rest/api/3/issue/PROJ-") {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		now := current.Add(1)
		for {
			old := peak.Load()
			if now <= old || peak.CompareAndSwap(old, now) {
				break
			}
		}
		time.Sleep(25 * time.Millisecond)
		current.Add(-1)
		key := strings.TrimPrefix(r.URL.Path, "/rest/api/3/issue/")
		_, _ = w.Write([]byte(`{"id":"` + key + `","key":"` + key + `","fields":{"summary":"` + key + ` summary"}}`))
	}))
	defer srv.Close()
	t.Setenv("JIRA_TOKEN_DEFAULT", "test-token")

	bin := buildJiraBinary(t)
	cmd := exec.Command(bin, "--config", jiraConfig(t, srv.URL), "--output=json",
		"issue", "view", "-p", "2", "PROJ-1..2")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("issue view multi-key error = %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}

	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Succeeded int `json:"succeeded"`
			Failed    int `json:"failed"`
			Results   []struct {
				Key   string `json:"key"`
				OK    bool   `json:"ok"`
				Issue struct {
					Key string `json:"key"`
				} `json:"issue"`
			} `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout envelope is not JSON: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	if !env.OK || env.Data.Succeeded != 2 || env.Data.Failed != 0 {
		t.Fatalf("multi-key summary = ok %v succeeded %d failed %d\n%s", env.OK, env.Data.Succeeded, env.Data.Failed, stdout.String())
	}
	if got := []string{env.Data.Results[0].Key, env.Data.Results[1].Key}; strings.Join(got, ",") != "PROJ-1,PROJ-2" {
		t.Fatalf("result order = %#v, want PROJ-1, PROJ-2", got)
	}
	if env.Data.Results[0].Issue.Key != "PROJ-1" || env.Data.Results[1].Issue.Key != "PROJ-2" {
		t.Fatalf("result issues = %+v", env.Data.Results)
	}
	if peak.Load() < 2 {
		t.Fatalf("peak concurrency = %d, want local -p 2 to allow two in-flight requests", peak.Load())
	}
}

func TestIssueViewMultipleKeysReportsPerKeyErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/api/3/issue/PROJ-1":
			_, _ = w.Write([]byte(`{"id":"10001","key":"PROJ-1","fields":{"summary":"ok"}}`))
		case "/rest/api/3/issue/PROJ-2":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"errorMessages":["issue does not exist"],"errors":{}}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	bin := buildJiraBinary(t)
	cmd := exec.Command(bin, "--config", jiraConfig(t, srv.URL), "--output=json",
		"issue", "view", "PROJ-1", "PROJ-2", "-p", "2")
	cmd.Env = append(os.Environ(), "JIRA_TOKEN_DEFAULT=test-token")
	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Succeeded int `json:"succeeded"`
			Failed    int `json:"failed"`
			Results   []struct {
				Key   string          `json:"key"`
				OK    bool            `json:"ok"`
				Error json.RawMessage `json:"error"`
			} `json:"results"`
		} `json:"data"`
		Errors []struct {
			Code string `json:"code"`
		} `json:"errors"`
	}
	_, _, _ = runCommandExpectErrorEnvelope(t, cmd, &env)

	if env.OK || env.Data.Succeeded != 1 || env.Data.Failed != 1 {
		t.Fatalf("partial summary = ok %v succeeded %d failed %d", env.OK, env.Data.Succeeded, env.Data.Failed)
	}
	if len(env.Data.Results) != 2 {
		t.Fatalf("results length = %d, want 2", len(env.Data.Results))
	}
	if !env.Data.Results[0].OK || env.Data.Results[1].OK {
		t.Fatalf("per-key ok flags = %+v", env.Data.Results)
	}
	if len(env.Data.Results[1].Error) == 0 || string(env.Data.Results[1].Error) == "null" {
		t.Fatalf("missing per-key error: %+v", env.Data.Results[1])
	}
	if len(env.Errors) == 0 || env.Errors[0].Code == "" {
		t.Fatalf("top-level error missing stable code: %+v", env.Errors)
	}
}

func TestIssueViewMultipleKeysHumanOutputSummarizesPartialFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/api/3/issue/PROJ-1":
			_, _ = w.Write([]byte(`{"id":"10001","key":"PROJ-1","fields":{"summary":"ok","status":{"name":"Done"},"priority":{"name":"Medium"}}}`))
		case "/rest/api/3/issue/PROJ-2":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"errorMessages":["issue does not exist"],"errors":{}}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	bin := buildJiraBinary(t)
	cmd := exec.Command(bin, "--config", jiraConfig(t, srv.URL), "--output=human",
		"issue", "view", "PROJ-1", "PROJ-2", "-p", "2")
	cmd.Env = append(os.Environ(), "JIRA_TOKEN_DEFAULT=test-token")
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err == nil {
		t.Fatalf("command succeeded; want partial-failure exit\nstdout=%s\nstderr=%s", stdout.String(), stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "PROJ-1") || !strings.Contains(got, "threads=2") {
		t.Fatalf("human stdout missing success table:\nstdout=%s\nstderr=%s", got, stderr.String())
	}
	if strings.Contains(stdout.String(), "failed keys:") {
		t.Fatalf("human stdout should not include failed-key diagnostics:\nstdout=%s\nstderr=%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "ERR") ||
		!strings.Contains(stderr.String(), "failed keys") ||
		!strings.Contains(stderr.String(), "keys=PROJ-2") ||
		!strings.Contains(stderr.String(), "total=2") ||
		!strings.Contains(stderr.String(), "succeeded=1") ||
		!strings.Contains(stderr.String(), "failed=1") ||
		!strings.Contains(stderr.String(), `reason="jira not found"`) {
		t.Fatalf("human stderr missing failed-key diagnostic:\nstdout=%s\nstderr=%s", stdout.String(), stderr.String())
	}
	if strings.Contains(stderr.String(), "issue view completed with") {
		t.Fatalf("human stderr should not duplicate the partial-failure summary:\n%s", stderr.String())
	}
	if strings.Contains(stderr.String(), `{"errorMessages"`) {
		t.Fatalf("human stderr leaked raw upstream JSON instead of batch summary:\n%s", stderr.String())
	}
}

func TestIssueViewMultipleKeysHumanOutputBoundsFailedKeyDiagnostics(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, "/rest/api/3/issue/")
		if key == "PROJ-1" {
			_, _ = w.Write([]byte(`{"id":"10001","key":"PROJ-1","fields":{"summary":"ok","status":{"name":"Done"},"priority":{"name":"Medium"}}}`))
			return
		}
		if strings.HasPrefix(key, "PROJ-") {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"errorMessages":["issue does not exist"],"errors":{}}`))
			return
		}
		t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	bin := buildJiraBinary(t)
	cmd := exec.Command(bin, "--config", jiraConfig(t, srv.URL), "--output=human",
		"issue", "view", "PROJ-1:31", "-p", "4")
	cmd.Env = append(os.Environ(), "JIRA_TOKEN_DEFAULT=test-token")
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err == nil {
		t.Fatalf("command succeeded; want partial-failure exit\nstdout=%s\nstderr=%s", stdout.String(), stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "PROJ-1") || strings.Contains(got, "failed keys:") || strings.Contains(got, "PROJ-31") {
		t.Fatalf("human stdout should show successes only, not failed-key wall:\nstdout=%s\nstderr=%s", got, stderr.String())
	}
	gotErr := stderr.String()
	for _, want := range []string{
		"failed keys",
		"ERR",
		"PROJ-2",
		"shown=5",
		"PROJ-6",
		"omitted=25",
		"total=31",
		"succeeded=1",
		"failed=30",
		`reason="jira not found"`,
		`hint="use --output=json for full per-key errors"`,
	} {
		if !strings.Contains(gotErr, want) {
			t.Fatalf("human stderr missing %q:\nstdout=%s\nstderr=%s", want, stdout.String(), gotErr)
		}
	}
	if strings.Contains(gotErr, "issue view completed with") {
		t.Fatalf("human stderr should not duplicate the partial-failure summary:\n%s", gotErr)
	}
	if strings.Contains(gotErr, "PROJ-7") || strings.Contains(gotErr, "PROJ-31") {
		t.Fatalf("human stderr should truncate failed-key list by default:\n%s", gotErr)
	}
}

func TestIssueViewMultipleKeysJSONKeepsAllFailuresWhenHumanDiagnosticsTruncate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, "/rest/api/3/issue/")
		if key == "PROJ-1" {
			_, _ = w.Write([]byte(`{"id":"10001","key":"PROJ-1","fields":{"summary":"ok"}}`))
			return
		}
		if strings.HasPrefix(key, "PROJ-") {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"errorMessages":["issue does not exist"],"errors":{}}`))
			return
		}
		t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	bin := buildJiraBinary(t)
	cmd := exec.Command(bin, "--config", jiraConfig(t, srv.URL), "--output=json",
		"issue", "view", "PROJ-1:31", "-p", "4")
	cmd.Env = append(os.Environ(), "JIRA_TOKEN_DEFAULT=test-token")
	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Succeeded int `json:"succeeded"`
			Failed    int `json:"failed"`
			Results   []struct {
				Key string `json:"key"`
				OK  bool   `json:"ok"`
			} `json:"results"`
		} `json:"data"`
	}
	_, _, _ = runCommandExpectErrorEnvelope(t, cmd, &env)

	if env.OK || env.Data.Succeeded != 1 || env.Data.Failed != 30 || len(env.Data.Results) != 31 {
		t.Fatalf("multi-key JSON summary = ok %v succeeded %d failed %d results %d",
			env.OK, env.Data.Succeeded, env.Data.Failed, len(env.Data.Results))
	}
	if env.Data.Results[30].Key != "PROJ-31" || env.Data.Results[30].OK {
		t.Fatalf("last JSON result = %+v, want full untruncated PROJ-31 failure", env.Data.Results[30])
	}
}
