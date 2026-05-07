package unit

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli"
)

func TestStructuredErrorsAndExitCodes(t *testing.T) {
	err := cli.NewError(cli.ErrorTypeRateLimit, "slow down")
	if code := cli.ExitCode(err); code != 4 {
		t.Fatalf("ExitCode() = %d", code)
	}
	var buf bytes.Buffer
	if werr := cli.WriteErrors(&buf, []cli.Error{err}); werr != nil {
		t.Fatalf("WriteErrors() error = %v", werr)
	}
	var decoded []cli.Error
	if jerr := json.Unmarshal(buf.Bytes(), &decoded); jerr != nil {
		t.Fatalf("stderr JSON invalid: %v", jerr)
	}
	if decoded[0].Type != string(cli.ErrorTypeRateLimit) {
		t.Fatalf("decoded error = %+v", decoded[0])
	}
}
