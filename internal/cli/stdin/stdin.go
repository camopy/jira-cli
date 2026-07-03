// Package stdin is the ONLY place in the source tree allowed to read
// os.Stdin. Three opt-in surfaces exist:
//
//   - JSONInput(path) is the only way command-payload JSON enters the
//     CLI; pass "-" to mean stdin.
//   - TextInput(path) is the only way non-JSON text payloads (Markdown
//     for `adf convert`) enter the CLI; pass "-" to mean stdin.
//   - SecretStdin() is the only way credential material enters the CLI
//     via stdin; kept separate so a JSON parser never accidentally eats
//     a secret.
//
// Tests/contract/no_input_no_stdin_test.go and
// tests/guardrails/stdin_discipline_test.go enforce that no other
// package directly references os.Stdin.
package stdin

import (
	"errors"
	"io"
	"os"
)

// ErrEmptySecret is returned by SecretStdin when stdin yields no bytes
// — usually because the user forgot to pipe the secret in.
var ErrEmptySecret = errors.New("stdin: --secret-stdin requested but no input received")

// JSONInput returns a reader for the command-payload JSON. Path "-"
// means stdin; any other value is a regular file. The caller is
// responsible for closing the returned io.ReadCloser.
//
// This is the ONLY function the CLI uses to satisfy a
// `--json-input <path>` flag. Adding implicit stdin reads anywhere else
// is forbidden by the guardrail test.
func JSONInput(path string) (io.ReadCloser, error) {
	if path == "-" {
		return io.NopCloser(os.Stdin), nil
	}
	return os.Open(path)
}

// TextInput returns a reader for a non-JSON text payload, such as the
// Markdown input of `adf convert`. Path "-" means stdin; any other value
// is a regular file. The caller closes the returned io.ReadCloser.
func TextInput(path string) (io.ReadCloser, error) {
	if path == "-" {
		return io.NopCloser(os.Stdin), nil
	}
	return os.Open(path)
}

// SecretStdin reads the entire stdin and returns the trailing-newline-
// trimmed secret. Used only when --secret-stdin was passed; never
// invoked implicitly under --no-input or any other flag.
//
// This is the only secret-bearing stdin path.
func SecretStdin() (string, error) {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", ErrEmptySecret
	}
	// Trim a single trailing newline that pipes typically add. Don't
	// trim all whitespace — secrets can legitimately contain trailing
	// spaces, however unwise.
	if data[len(data)-1] == '\n' {
		data = data[:len(data)-1]
	}
	if len(data) > 0 && data[len(data)-1] == '\r' {
		data = data[:len(data)-1]
	}
	return string(data), nil
}
