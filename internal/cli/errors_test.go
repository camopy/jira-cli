package cli_test

import (
	"errors"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli"
)

func TestMapErrorClassifiesOutputFailure(t *testing.T) {
	cause := errors.New("write sentinel")
	err := cli.NewOutputError(cause)

	if !errors.Is(err, cause) {
		t.Fatal("output error does not retain its write cause")
	}
	got := cli.MapError(err)
	if got.Code != "output_write_failed" {
		t.Fatalf("Code = %q, want output_write_failed", got.Code)
	}
	if got.Type != "io" {
		t.Fatalf("Type = %q, want io", got.Type)
	}
	if got.Retryable {
		t.Fatal("Retryable = true, want false")
	}
	if exit := cli.ExitCode(got); exit != 8 {
		t.Fatalf("ExitCode() = %d, want 8", exit)
	}
}
