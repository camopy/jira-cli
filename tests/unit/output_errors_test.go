package unit

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli"
)

func TestStructuredErrorsAndExitCodes(t *testing.T) {
	err := cli.Error{Type: string(cli.ErrorTypeRateLimit), Code: "rate_limited", Message: "slow down"}
	if code := cli.ExitCode(err); code != 4 {
		t.Fatalf("ExitCode() = %d", code)
	}

	// A failure envelope round-trips through JSON and preserves the
	// structured error type for a machine consumer.
	var buf bytes.Buffer
	env := cli.Envelope{Errors: []cli.Error{err}, Warnings: []cli.Warning{}}
	if werr := cli.WriteEnvelope(&buf, env); werr != nil {
		t.Fatalf("WriteEnvelope() error = %v", werr)
	}
	var decoded cli.Envelope
	if jerr := json.Unmarshal(buf.Bytes(), &decoded); jerr != nil {
		t.Fatalf("envelope JSON invalid: %v", jerr)
	}
	if len(decoded.Errors) != 1 || decoded.Errors[0].Type != string(cli.ErrorTypeRateLimit) {
		t.Fatalf("decoded errors = %+v", decoded.Errors)
	}
}
