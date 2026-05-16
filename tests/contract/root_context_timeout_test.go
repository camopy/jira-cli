package contract

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// A root --timeout flag must bound the whole invocation: a request to a
// server that never responds has to be abandoned once the deadline
// elapses, and the CLI must exit with a deterministic error rather than
// hang until the OS or the per-profile HTTP timeout intervenes.
func TestRootTimeoutAbortsHungRequest(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release // hold the connection open until the test ends
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	defer close(release)

	cfg := jiraConfig(t, srv.URL)

	done := make(chan struct{})
	var stdout, stderr []byte
	var code int
	go func() {
		stdout, stderr, code = runJira(t, "--config", cfg, "--timeout", "1s", "--output=json",
			"search", "jql", "project = PROJ")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("command hung well past --timeout=1s — root deadline not propagated")
	}

	if code == 0 {
		t.Fatalf("hung request under --timeout=1s exited 0; want a non-zero failure\nstdout=%s", stdout)
	}
	body := strings.ToLower(string(stderr) + string(stdout))
	if strings.Contains(body, "unknown flag") {
		t.Fatalf("--timeout is not a recognized root flag:\n%s", body)
	}
	if !strings.Contains(body, "deadline") && !strings.Contains(body, "canceled") {
		t.Fatalf("error does not mention a deadline/cancellation cause:\n%s", body)
	}
}
