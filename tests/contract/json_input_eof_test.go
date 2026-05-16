package contract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A --json-input payload must be exactly one JSON document. Trailing
// non-whitespace bytes after the value indicate a malformed or
// concatenated file and must be rejected, not silently ignored by the
// streaming decoder.
func TestJSONInputRejectsTrailingBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "payload.json")
	// Valid object followed by junk. json.Decoder.Decode stops after the
	// first value and never sees the trailing bytes unless EOF is checked.
	if err := os.WriteFile(path, []byte(`{"summary":"ok"} this is junk`), 0o600); err != nil {
		t.Fatalf("write payload: %v", err)
	}

	stdout, stderr, code := runJira(t, "issue", "create", "--no-input", "--json-input", path, "--output=json")
	if code != 3 {
		t.Fatalf("exit = %d; want 3 (validation) for JSON with trailing bytes\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	body := strings.ToLower(string(stderr) + string(stdout))
	if !strings.Contains(body, "json") {
		t.Fatalf("trailing-bytes rejection does not mention the JSON parse failure:\n%s", body)
	}
}

// A --json-input payload must be exactly ONE JSON document. A second
// well-formed value, or a stray trailing structural byte (`}` or `]`),
// is a malformed file. json.Decoder.More() returns false for a stray
// trailing `}`/`]`, so these payloads slip past a More()-based check; a
// second Decode that must return io.EOF catches them.
func TestJSONInputRejectsTrailingStructuralBytes(t *testing.T) {
	cases := []struct {
		name    string
		payload string
	}{
		{"trailing brace", `{"summary":"ok"}}`},
		{"trailing bracket", `{"summary":"ok"}]`},
		{"second object", `{"a":1}{"b":2}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "payload.json")
			if err := os.WriteFile(path, []byte(tc.payload), 0o600); err != nil {
				t.Fatalf("write payload: %v", err)
			}
			stdout, stderr, code := runJira(t, "issue", "create", "--no-input", "--json-input", path, "--output=json")
			if code != 3 {
				t.Fatalf("exit = %d; want 3 (validation) for %q\nstdout=%s\nstderr=%s", code, tc.payload, stdout, stderr)
			}
			body := strings.ToLower(string(stderr) + string(stdout))
			if !strings.Contains(body, "json") {
				t.Fatalf("rejection does not mention the JSON parse failure:\n%s", body)
			}
		})
	}
}

// A clean single JSON document with only trailing whitespace must still
// be accepted — whitespace after the value is not junk.
func TestJSONInputAcceptsTrailingWhitespace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "payload.json")
	if err := os.WriteFile(path, []byte("{\"summary\":\"ok\"}\n\n  \t\n"), 0o600); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	// No base URL configured: the command fails later (credential/base-url),
	// but it must get PAST JSON parsing — the failure must not be a JSON
	// parse error.
	stdout, stderr, _ := runJira(t, "issue", "create", "--no-input", "--json-input", path,
		"--summary", "ok", "--output=json")
	body := strings.ToLower(string(stderr) + string(stdout))
	if strings.Contains(body, "trailing") || strings.Contains(body, "invalid json") {
		t.Fatalf("trailing whitespace was wrongly rejected as junk:\n%s", body)
	}
}
