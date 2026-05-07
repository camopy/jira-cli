package contract

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// `auth login --no-input` MUST NOT prompt and MUST NOT read implicit
// stdin. Metadata via flags or --json-input. Secrets via the configured
// backend, env, or --secret-stdin.
//
// We pipe garbage to stdin and assert the command does not consume it
// (deterministic exit, no hang, no error mentioning the piped data).
func TestAuthLoginNoInputDoesNotReadImplicitStdin(t *testing.T) {
	bin := buildJiraBinary(t)

	cmd := exec.Command(bin, "auth", "login", "--no-input", "--json")
	cmd.Stdin = strings.NewReader("garbage that should not be consumed\n")
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr

	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected non-zero exit (validation: no profile metadata supplied), got success")
		}
		body := stderr.String()
		if strings.Contains(strings.ToLower(body), "garbage") {
			t.Errorf("error mentions piped data — implicit stdin read: %s", body)
		}
		if strings.Contains(strings.ToLower(body), "stdin error") {
			t.Errorf("error references stdin — implicit read suspected: %s", body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("command hung — implicit stdin read suspected")
	}
}
