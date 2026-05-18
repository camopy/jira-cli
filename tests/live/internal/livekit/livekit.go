//go:build live

// Package livekit is the shared harness for the live Jira end-to-end
// suites under tests/live/. It builds the jira binary, drives it as a
// subprocess against a real tenant, parses JSON envelopes, and tracks
// disposable issues for marker-gated cleanup. Each tests/live/<type>
// package imports this kit and supplies its own scenarios.
package livekit

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// liveProjectEnv names the environment variable that must hold the Jira
// project key the suites mutate. The harness refuses to run without it.
const liveProjectEnv = "JIRA_LIVETEST_PROJECT"

// Envelope is the JSON envelope every jira command emits under
// --output=json.
type Envelope struct {
	OK   bool `json:"ok"`
	Meta struct {
		Command  string `json:"command"`
		ExitCode *int   `json:"exit_code,omitempty"`
	} `json:"meta"`
	Data     map[string]any `json:"data"`
	Errors   []Error        `json:"errors"`
	Warnings []any          `json:"warnings"`
}

// Error is one entry in an envelope's errors array.
type Error struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
}

// Suite is one live-test run: a built binary, a target project, and a
// unique run marker used to gate destructive cleanup.
type Suite struct {
	Bin       string
	Project   string
	IssueType string
	RunID     string
	Marker    string
	BaseURL   string

	mu          sync.Mutex
	cleanupKeys map[string]bool
}

// NewSuite builds the binary, resolves the target project, and registers
// marker-gated cleanup of every tracked issue.
func NewSuite(t *testing.T) *Suite {
	t.Helper()
	project := RequireProject(t)
	bin := BuildBinary(t)
	runID := newRunID(t)
	issueType := strings.TrimSpace(os.Getenv("JIRA_LIVETEST_ISSUE_TYPE"))
	if issueType == "" {
		issueType = "Task"
		t.Logf("JIRA_LIVETEST_ISSUE_TYPE is not set; using Jira issue type %q", issueType)
	}
	s := &Suite{
		Bin:         bin,
		Project:     project,
		IssueType:   issueType,
		RunID:       runID,
		Marker:      "[jira-cli-livetest " + runID + "]",
		cleanupKeys: map[string]bool{},
	}
	t.Cleanup(func() { s.cleanupTrackedIssues(t) })
	s.BaseURL = s.activeBaseURL(t)
	return s
}

// RequireProject returns the configured live-test project key or fails
// the test rather than touching an unspecified tenant.
func RequireProject(t *testing.T) string {
	t.Helper()
	project := strings.TrimSpace(os.Getenv(liveProjectEnv))
	if project == "" {
		t.Fatalf("%s must be set to the Jira project key for live issue tests; refusing to run against an unspecified tenant/project", liveProjectEnv)
	}
	return project
}

var (
	jiraBinaryOnce sync.Once
	jiraBinaryPath string
	jiraBinaryDir  string
	jiraBinaryErr  error
)

// BuildBinary compiles ./cmd/jira once per run and returns its path.
func BuildBinary(t *testing.T) string {
	t.Helper()
	jiraBinaryOnce.Do(func() {
		if err := os.MkdirAll(goTmpDir(), 0o700); err != nil {
			jiraBinaryErr = err
			return
		}
		dir, err := os.MkdirTemp(goTmpDir(), "jira-cli-live-bin-*")
		if err != nil {
			jiraBinaryErr = err
			return
		}
		jiraBinaryDir = dir
		bin := filepath.Join(dir, "jira")
		cmd := exec.Command("go", "build", "-o", bin, "./cmd/jira")
		cmd.Dir = repoRoot()
		cmd.Env = liveEnv()
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			jiraBinaryErr = fmt.Errorf("go build jira binary: %w\n%s", err, out.String())
			return
		}
		jiraBinaryPath = bin
	})
	require.NoError(t, jiraBinaryErr)
	require.NotEmpty(t, jiraBinaryPath)
	return jiraBinaryPath
}

// repoRoot returns the module root, resolved relative to this source
// file (tests/live/internal/livekit/livekit.go).
func repoRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}

func goTmpDir() string {
	if v := strings.TrimSpace(os.Getenv("GOTMPDIR")); v != "" {
		return v
	}
	return os.TempDir()
}

// liveEnv is the environment for the built binary and subprocess runs:
// the inherited environment with color disabled so envelopes parse
// cleanly. GOTMPDIR/TMPDIR are passed through from the caller as-is.
func liveEnv() []string {
	return append(os.Environ(), "NO_COLOR=1")
}

func newRunID(t *testing.T) string {
	t.Helper()
	var b [4]byte
	_, err := rand.Read(b[:])
	require.NoError(t, err)
	return time.Now().UTC().Format("20060102T150405Z") + "-" + hex.EncodeToString(b[:])
}

// Run executes a jira subcommand, requires ok=true, and returns the
// parsed envelope.
func (s *Suite) Run(t *testing.T, args ...string) Envelope {
	t.Helper()
	stdout, stderr, err := s.RunRaw(t, append([]string{"--output=json", "--no-input", "--timeout=90s"}, args...)...)
	require.NoError(t, err, "jira %s\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), stdout, stderr)
	var env Envelope
	require.NoError(t, json.Unmarshal([]byte(stdout), &env), "jira %s returned non-JSON stdout:\n%s\nstderr:\n%s", strings.Join(args, " "), stdout, stderr)
	require.True(t, env.OK, "jira %s returned ok=false: errors=%+v stdout=%s stderr=%s", strings.Join(args, " "), env.Errors, stdout, stderr)
	require.NotNil(t, env.Data, "jira %s returned nil data", strings.Join(args, " "))
	return env
}

// RunRaw executes the binary and returns stdout, stderr, and the process
// error without interpreting the result.
func (s *Suite) RunRaw(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.Bin, args...)
	cmd.Dir = repoRoot()
	cmd.Env = liveEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func (s *Suite) activeBaseURL(t *testing.T) string {
	t.Helper()
	profileEnv := s.Run(t, "config", "profile")
	active := StringField(t, profileEnv.Data, "active_profile")
	require.NotEmpty(t, active, "active Jira profile is required")
	baseEnv := s.Run(t, "config", "get", "profiles."+active+".base_url")
	baseURL := StringField(t, baseEnv.Data, "value")
	require.NotEmpty(t, baseURL, "active Jira profile %q must have base_url", active)
	return strings.TrimRight(baseURL, "/")
}

// SelfAccountID returns the authenticated user's account id.
func (s *Suite) SelfAccountID(t *testing.T) string {
	t.Helper()
	env := s.Run(t, "auth", "whoami")
	accountID := StringField(t, env.Data, "account_id")
	require.NotEmpty(t, accountID, "auth whoami must return the authenticated user's account_id")
	return accountID
}

// WriteJSON marshals value to a temp file and returns its path.
func (s *Suite) WriteJSON(t *testing.T, name string, value any) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	body, err := json.MarshalIndent(value, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, append(body, '\n'), 0o600))
	return path
}

// CreateIssue creates an issue in the target project and returns its key.
func (s *Suite) CreateIssue(t *testing.T, summary string, extra map[string]any) string {
	t.Helper()
	payload := map[string]any{
		"project_key": s.Project,
		"issue_type":  s.IssueType,
		"summary":     summary,
	}
	for k, v := range extra {
		payload[k] = v
	}
	env := s.Run(t, "issue", "create", "--json-input", s.WriteJSON(t, "issue-create.json", payload))
	issue := MapField(t, env.Data, "issue")
	key := StringField(t, issue, "key")
	require.NotEmpty(t, key, "issue.create returned no issue key")
	return key
}

// TrackCleanup marks an issue key for marker-gated deletion at run end.
func (s *Suite) TrackCleanup(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if key != "" {
		s.cleanupKeys[key] = true
	}
}

// UntrackCleanup drops a key that has already been deleted.
func (s *Suite) UntrackCleanup(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cleanupKeys, key)
}

func (s *Suite) cleanupTrackedIssues(t *testing.T) {
	t.Helper()
	s.mu.Lock()
	keys := make([]string, 0, len(s.cleanupKeys))
	for key := range s.cleanupKeys {
		keys = append(keys, key)
	}
	s.mu.Unlock()
	for _, key := range keys {
		if err := s.SafeDeleteIssue(t, key); err != nil {
			t.Logf("cleanup: %v", err)
		}
	}
}

// SafeDeleteIssue deletes an issue only after confirming its summary
// carries this run's marker — it refuses to delete anything else.
func (s *Suite) SafeDeleteIssue(t *testing.T, key string) error {
	t.Helper()
	if key == "" {
		return nil
	}
	view, err := s.runAllowFailure(t, "issue", "view", key)
	if err != nil {
		return fmt.Errorf("view %s before cleanup: %w", key, err)
	}
	summary := IssueSummary(view)
	if !strings.Contains(summary, s.Marker) {
		return fmt.Errorf("refusing to delete %s: summary %q does not contain live-test marker %q", key, summary, s.Marker)
	}
	if _, err := s.runAllowFailure(t, "issue", "delete", key, "--force"); err != nil {
		return fmt.Errorf("delete %s: %w", key, err)
	}
	s.UntrackCleanup(key)
	return nil
}

// runAllowFailure runs a jira subcommand and returns the envelope and an
// error instead of failing the test, for cleanup-path resilience.
func (s *Suite) runAllowFailure(t *testing.T, args ...string) (Envelope, error) {
	t.Helper()
	stdout, stderr, err := s.RunRaw(t, append([]string{"--output=json", "--no-input", "--timeout=90s"}, args...)...)
	if err != nil {
		return Envelope{}, fmt.Errorf("jira %s: %w\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), err, stdout, stderr)
	}
	var env Envelope
	if jerr := json.Unmarshal([]byte(stdout), &env); jerr != nil {
		return Envelope{}, fmt.Errorf("jira %s returned non-JSON stdout: %w\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), jerr, stdout, stderr)
	}
	if !env.OK {
		return env, fmt.Errorf("jira %s returned ok=false: errors=%+v\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), env.Errors, stdout, stderr)
	}
	return env, nil
}

// IssueSummary extracts issue.fields.summary from a view envelope.
func IssueSummary(env Envelope) string {
	issue, ok := env.Data["issue"].(map[string]any)
	if !ok {
		return ""
	}
	fields, ok := issue["fields"].(map[string]any)
	if !ok {
		return ""
	}
	summary, _ := fields["summary"].(string)
	return summary
}

// IssueStatus extracts issue.fields.status.name from a view envelope.
func IssueStatus(env Envelope) string {
	issue, ok := env.Data["issue"].(map[string]any)
	if !ok {
		return ""
	}
	fields, ok := issue["fields"].(map[string]any)
	if !ok {
		return ""
	}
	status, ok := fields["status"].(map[string]any)
	if !ok {
		return ""
	}
	name, _ := status["name"].(string)
	return name
}

// IssueLabels extracts issue.fields.labels from a view envelope.
func IssueLabels(t *testing.T, env Envelope) []string {
	t.Helper()
	issue := MapField(t, env.Data, "issue")
	fields := MapField(t, issue, "fields")
	raw, ok := fields["labels"].([]any)
	require.True(t, ok, "issue fields.labels missing or not an array: %+v", fields["labels"])
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		out = append(out, fmt.Sprint(v))
	}
	return out
}

// SurvivorSummary is the summary of the retained kitchen-sink issue.
func SurvivorSummary(runID string) string {
	return "[jira-cli-livetest SURVIVOR " + runID + "] kitchen sink issue"
}

// ADFDoc builds a minimal single-paragraph ADF document.
func ADFDoc(text string) map[string]any {
	return map[string]any{
		"type":    "doc",
		"version": 1,
		"content": []map[string]any{
			{
				"type": "paragraph",
				"content": []map[string]any{
					{"type": "text", "text": text},
				},
			},
		},
	}
}

// MapField asserts m[key] is an object and returns it.
func MapField(t *testing.T, m map[string]any, key string) map[string]any {
	t.Helper()
	raw, ok := m[key].(map[string]any)
	require.True(t, ok, "field %q missing or not an object: %+v", key, m[key])
	return raw
}

// StringField asserts m[key] is a string and returns it.
func StringField(t *testing.T, m map[string]any, key string) string {
	t.Helper()
	raw, ok := m[key].(string)
	require.True(t, ok, "field %q missing or not a string: %+v", key, m[key])
	return raw
}

// BoolField asserts m[key] is a bool and returns it.
func BoolField(t *testing.T, m map[string]any, key string) bool {
	t.Helper()
	raw, ok := m[key].(bool)
	require.True(t, ok, "field %q missing or not a bool: %+v", key, m[key])
	return raw
}

// SliceField asserts m[key] is an array and returns it.
func SliceField(t *testing.T, m map[string]any, key string) []any {
	t.Helper()
	raw, ok := m[key].([]any)
	require.True(t, ok, "field %q missing or not an array: %+v", key, m[key])
	return raw
}

var (
	survivorMu            sync.Mutex
	survivorAnnouncements []string
)

// RecordSurvivor queues a retained-issue announcement for PrintSurvivors.
func RecordSurvivor(key, browseURL string) {
	survivorMu.Lock()
	defer survivorMu.Unlock()
	survivorAnnouncements = append(survivorAnnouncements, fmt.Sprintf("\n*** LIVE JIRA SURVIVOR ISSUE ***\nkey: %s\nbrowse: %s\n********************************\n", key, browseURL))
}

// PrintSurvivors writes every recorded survivor announcement to stderr.
// A suite package calls this from TestMain after m.Run.
func PrintSurvivors() {
	survivorMu.Lock()
	defer survivorMu.Unlock()
	for _, msg := range survivorAnnouncements {
		_, _ = fmt.Fprint(os.Stderr, msg)
	}
}

// CleanupBinary removes the temporary directory holding the built jira
// binary. A suite package calls this from TestMain after m.Run, once no
// further test can need the binary.
func CleanupBinary() {
	if jiraBinaryDir != "" {
		_ = os.RemoveAll(jiraBinaryDir)
	}
}
