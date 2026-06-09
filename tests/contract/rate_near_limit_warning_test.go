package contract

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
)

// A successful read whose response carries Jira's X-RateLimit-NearLimit header
// must surface a non-fatal rate_limit_near warning in the envelope, while the
// command still succeeds (ok:true, exit 0).
func TestNearLimitSuccessSurfacesWarning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/myself" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("X-RateLimit-NearLimit", "true")
		w.Header().Set("RateLimit-Reason", "jira-burst-based")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accountId":"abc","displayName":"Demo","emailAddress":"demo@example.com","active":true}`))
	}))
	defer srv.Close()

	bin := buildJiraBinary(t)
	cfg := jiraConfig(t, srv.URL)
	cmd := exec.Command(bin, "--config", cfg, "--output=json", "auth", "whoami")
	cmd.Env = append(os.Environ(), "JIRA_TOKEN_DEFAULT=tok")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("auth whoami should succeed (near-limit is non-fatal): %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}

	var env struct {
		OK       bool `json:"ok"`
		Warnings []struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"warnings"`
	}
	decodeErrorEnvelopeFromStdout(t, stdout.Bytes(), stderr.Bytes(), cmd.Args, &env)
	if !env.OK {
		t.Fatalf("near-limit must not fail the command:\n%s", stdout.String())
	}
	found := false
	for _, w := range env.Warnings {
		if w.Type == "rate_limit_near" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a rate_limit_near warning, got %+v", env.Warnings)
	}
}
