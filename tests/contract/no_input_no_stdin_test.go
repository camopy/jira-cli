package contract

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// --no-input on a mutation command MUST NOT read stdin implicitly.
// The only opt-ins are --json-input and --secret-stdin.
//
// We exercise this by piping garbage to a --no-input command: if the
// CLI reads stdin implicitly, it'll either hang waiting for input,
// consume the garbage and try to parse it, or otherwise behave
// non-deterministically. The CLI must ignore the pipe and fail with a
// deterministic validation error from the missing required-flag set,
// NOT from a failed stdin read.
func TestNoInputDoesNotReadStdinImplicitly(t *testing.T) {
	bin := buildJiraBinary(t)

	cases := []struct {
		name string
		args []string
	}{
		{"issue create", []string{"issue", "create", "--no-input", "--json"}},
		{"issue edit", []string{"issue", "edit", "KAN-1", "--no-input", "--json"}},
		{"comment add", []string{"issue", "comment", "KAN-1", "--no-input", "--json"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(bin, tc.args...)
			cmd.Stdin = strings.NewReader(`{"this":"should never be read"}`)
			stderr := &bytes.Buffer{}
			cmd.Stderr = stderr

			done := make(chan error, 1)
			go func() { done <- cmd.Run() }()

			select {
			case err := <-done:
				// We expect a non-zero exit (missing required flags), but
				// NOT because of the piped JSON. The pipe should be ignored.
				if err == nil {
					t.Fatalf("expected non-zero exit (validation error from missing required flags); got success")
				}
				body := stderr.String()
				if strings.Contains(strings.ToLower(body), "stdin") || strings.Contains(body, "this") {
					t.Errorf("error mentions stdin or piped payload — implicit stdin read suspected: %s", body)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("command hung — implicit stdin read suspected")
			}
		})
	}
}
