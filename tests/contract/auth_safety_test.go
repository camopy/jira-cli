package contract

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAuthLoginRefusesPromptsInHeadlessJSONMode(t *testing.T) {
	bin := buildJiraBinary(t)
	cmd := exec.Command(bin, "--json", "auth", "login")
	cmd.Stdin = strings.NewReader("work\nhttps://company.atlassian.net\ntoken\ndev@example.com\nkeyring\n\n\n")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatalf("headless auth login prompted and succeeded:\nstdout=%s", stdout.String())
	}
	// Prompts must not appear on stdout (the JSON path).
	if strings.Contains(stdout.String(), "Profile:") || strings.Contains(stdout.String(), "Secret backend:") {
		t.Fatalf("headless auth login wrote prompts to stdout:\nstdout=%s", stdout.String())
	}
	// clog diagnostic on stderr must mention "--no-input".
	stderrLow := strings.ToLower(stderr.String())
	if !strings.Contains(stderrLow, "err") || !strings.Contains(stderrLow, "no-input") {
		t.Fatalf("headless auth login did not emit clog diagnostic on stderr:\nstderr=%s", stderr.String())
	}
	// --json path must deliver a JSON envelope on stdout.
	var env map[string]any
	if jsonErr := json.Unmarshal(stdout.Bytes(), &env); jsonErr != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout=%s", jsonErr, stdout.String())
	}
	errs, _ := env["errors"].([]any)
	if len(errs) == 0 {
		t.Fatalf("envelope.errors is empty:\nstdout=%s", stdout.String())
	}
}

func TestAuthLoginDoesNotExposeRawTokenFlag(t *testing.T) {
	bin := buildJiraBinary(t)
	cmd := exec.Command(bin, "auth", "login", "--help")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("auth login --help error = %v\n%s", err, out)
	}
	if strings.Contains(string(out), "--token") {
		t.Fatalf("auth login exposes raw token flag:\n%s", out)
	}
}

func TestOnePasswordAuthLoginDoesNotPassSecretInProcessArgs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake op fixture is Unix-specific")
	}

	path := filepath.Join(t.TempDir(), "config.toml")
	binDir := t.TempDir()
	argsFile := filepath.Join(t.TempDir(), "op-args")
	stdinFile := filepath.Join(t.TempDir(), "op-stdin")
	op := filepath.Join(binDir, "op")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" > " + shellQuote(argsFile) + "\ncat > " + shellQuote(stdinFile) + "\n"
	if err := os.WriteFile(op, []byte(script), 0o700); err != nil {
		t.Fatalf("WriteFile(fake op) error = %v", err)
	}

	secret := "super-secret-token"
	cmd := exec.Command(
		buildJiraBinary(t),
		"--config", path,
		"auth", "login",
		"--no-input",
		"--profile-name", "work",
		"--base-url", "https://company.atlassian.net",
		"--auth-type", "token",
		"--email", "dev@example.com",
		"--backend", "1password",
		"--vault", "Engineering",
		"--item", "jira-cli-work",
		"--secret-stdin",
	)
	cmd.Stdin = strings.NewReader(secret)
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"OP_SERVICE_ACCOUNT_TOKEN=",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("auth login with fake op error = %v\n%s", err, out)
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("ReadFile(args) error = %v", err)
	}
	if strings.Contains(string(args), secret) {
		t.Fatalf("1Password command line leaked secret in args: %s", args)
	}
	stdin, err := os.ReadFile(stdinFile)
	if err != nil {
		t.Fatalf("ReadFile(stdin) error = %v", err)
	}
	if !strings.Contains(string(stdin), secret) {
		t.Fatalf("1Password command did not receive secret through stdin/template: %s", stdin)
	}
}

func shellQuote(path string) string {
	return "'" + strings.ReplaceAll(path, "'", "'\\''") + "'"
}
