package contract

// Contract tests for `jira issue attachment ….
//
// These tests drive the CLI binary end-to-end against an httptest server
// faking Atlassian's REST surface. Each test pins one wire-level
// invariant from contracts/{command-schemas,envelope-shapes,http-contract}.md
// across the issue lifecycle commands.
//
// All tests will fail until the lead's integration pass registers
// IssueAttachmentCommand under issueCommand in internal/cli/root/commands.go.

import (
	"bytes"
	"encoding/json"
	stdlibErrors "errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// `attachment list` envelope shape: oldest-first ordering, pagination
// metadata under data.pagination, attachment fields per envelope-shapes.md.
func TestAttachmentListEnvelopeShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/rest/api/3/issue/PROJ-1") {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		// Atlassian returns attachments in chronological (oldest-first) order; the CLI
		// preserves this ordering.
		_, _ = w.Write([]byte(`{
			"key": "PROJ-1",
			"fields": {
				"attachment": [
					{"id":"10042","filename":"old.png","mimeType":"image/png","size":1024,
					 "author":{"accountId":"a1","displayName":"Alice"},"created":"2026-04-01T10:00:00.000+0000",
					 "content":"https://example.atlassian.net/secure/attachment/10042/old.png","self":"http://x/10042"},
					{"id":"10043","filename":"new.log","mimeType":"text/plain","size":4096,
					 "author":{"accountId":"a2","displayName":"Bob"},"created":"2026-05-01T10:00:00.000+0000",
					 "content":"https://example.atlassian.net/secure/attachment/10043/new.log","self":"http://x/10043"}
				]
			}
		}`))
	}))
	defer srv.Close()

	cfg := jiraConfig(t, srv.URL)
	t.Setenv("JIRA_TOKEN_DEFAULT", "test-token")

	cmd := exec.Command(buildJiraBinary(t), "--config", cfg, "--output=json",
		"issue", "attachment", "list", "PROJ-1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("attachment list error = %v\n%s", err, out)
	}
	var env map[string]any
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("attachment list output not JSON: %v\n%s", err, out)
	}
	meta, _ := env["meta"].(map[string]any)
	if cmdName, _ := meta["command"].(string); cmdName != "issue.attachment.list" {
		t.Fatalf("envelope.meta.command = %q, want issue.attachment.list", cmdName)
	}
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("envelope.data not object: %s", out)
	}
	issue, _ := data["issue"].(map[string]any)
	if issue["key"] != "PROJ-1" {
		t.Fatalf("data.issue = %v, want key PROJ-1: %s", issue, out)
	}
	atts, _ := data["attachments"].([]any)
	if len(atts) != 2 {
		t.Fatalf("data.attachments length = %d, want 2", len(atts))
	}
	first, _ := atts[0].(map[string]any)
	if got, _ := first["id"].(string); got != "10042" {
		t.Fatalf("first attachment id = %q, want 10042 (oldest-first ordering)", got)
	}
	for _, key := range []string{"id", "filename", "mime_type", "size", "author", "created"} {
		if _, exists := first[key]; !exists {
			t.Fatalf("attachment missing required key %q: %v", key, first)
		}
	}
	pagination, ok := meta["pagination"].(map[string]any)
	if !ok {
		t.Fatalf("meta.pagination missing or wrong type (the canonical location): %s", out)
	}
	if _, hasOld := data["pagination"]; hasOld {
		t.Fatalf("pagination must live in meta, not data: %s", out)
	}
	for _, key := range []string{"total", "startAt", "maxResults", "isLast"} {
		if _, exists := pagination[key]; !exists {
			t.Fatalf("pagination missing %q: %v", key, pagination)
		}
	}
	if pagination["isLast"] != true || pagination["total"] != float64(2) {
		t.Fatalf("complete set must report isLast:true with the known total: %v", pagination)
	}
}

func TestAttachmentListEmptyKeepsIssueAndNonNullAttachments(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"key":"EMPTY-1","fields":{"attachment":[]}}`))
	}))
	defer srv.Close()

	cfg := jiraConfig(t, srv.URL)
	t.Setenv("JIRA_TOKEN_DEFAULT", "test-token")
	cmd := exec.Command(
		buildJiraBinary(t),
		"--config", cfg,
		"--output=json",
		"issue", "attachment", "list", "EMPTY-1",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("attachment list error = %v\n%s", err, out)
	}
	var env struct {
		Data struct {
			Issue struct {
				Key string `json:"key"`
			} `json:"issue"`
			Attachments []json.RawMessage `json:"attachments"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("decode envelope: %v\n%s", err, out)
	}
	if env.Data.Issue.Key != "EMPTY-1" {
		t.Fatalf("data.issue.key = %q, want EMPTY-1\n%s", env.Data.Issue.Key, out)
	}
	if env.Data.Attachments == nil || len(env.Data.Attachments) != 0 {
		t.Fatalf("data.attachments = %#v, want non-null empty array\n%s", env.Data.Attachments, out)
	}
}

// `attachment add` multipart upload contract.
// Asserts: X-Atlassian-Token: no-check header, multipart Content-Type,
// form field name `file`, response shape carries new attachment ids.
func TestAttachmentAddMultipartContract(t *testing.T) {
	var sawXATToken string
	var sawContentType string
	var formFiles []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/rest/api/3/issue/PROJ-1/attachments" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		sawXATToken = r.Header.Get("X-Atlassian-Token")
		sawContentType = r.Header.Get("Content-Type")
		mediaType, params, err := mime.ParseMediaType(sawContentType)
		if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
			t.Fatalf("Content-Type not multipart: %q (err=%v)", sawContentType, err)
		}
		mr := multipart.NewReader(r.Body, params["boundary"])
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("multipart NextPart: %v", err)
			}
			formFiles = append(formFiles, part.FormName())
			_, _ = io.Copy(io.Discard, part)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"10044","filename":"trace.log","mimeType":"text/plain","size":42,
			"author":{"accountId":"a1","displayName":"Alice"},"created":"2026-05-05T10:00:00.000+0000",
			"content":"http://x/10044","self":"http://x/10044"}]`))
	}))
	defer srv.Close()

	tmp := filepath.Join(t.TempDir(), "trace.log")
	if err := os.WriteFile(tmp, []byte("hello"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg := jiraConfig(t, srv.URL)
	t.Setenv("JIRA_TOKEN_DEFAULT", "test-token")

	cmd := exec.Command(buildJiraBinary(t), "--config", cfg, "--output=json",
		"issue", "attachment", "add", "PROJ-1", "--file", tmp)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("attachment add error = %v\n%s", err, out)
	}
	if sawXATToken != "no-check" {
		t.Fatalf("X-Atlassian-Token = %q, want no-check (: Atlassian CSRF guard)", sawXATToken)
	}
	if len(formFiles) != 1 || formFiles[0] != "file" {
		t.Fatalf("form field names = %v, want exactly [\"file\"] (: Atlassian requires field name 'file')", formFiles)
	}
	var env map[string]any
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("attachment add output not JSON: %v\n%s", err, out)
	}
	data, _ := env["data"].(map[string]any)
	atts, _ := data["attachments"].([]any)
	if len(atts) != 1 {
		t.Fatalf("response data.attachments length = %d, want 1", len(atts))
	}
	first, _ := atts[0].(map[string]any)
	if got, _ := first["id"].(string); got != "10044" {
		t.Fatalf("data.attachments[0].id = %q, want 10044", got)
	}
}

// 413 size-rejection bubbles up verbatim with exit 5. The CLI must
// surface the underlying Jira error verbatim if the upload exceeds the
// limit, rather than failing silently or producing a generic HTTP
// message.
func TestAttachmentAddSizeRejection413SurfacesUpstream(t *testing.T) {
	const upstreamMsg = "The file you are uploading is too large. The configured maximum upload size is 10485760 bytes."
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		_, _ = w.Write([]byte(`{"errorMessages":["` + upstreamMsg + `"],"errors":{}}`))
	}))
	defer srv.Close()

	tmp := filepath.Join(t.TempDir(), "big.bin")
	if err := os.WriteFile(tmp, []byte("ignored"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg := jiraConfig(t, srv.URL)
	t.Setenv("JIRA_TOKEN_DEFAULT", "test-token")

	bin := buildJiraBinary(t)
	cmd := exec.Command(bin, "--config", cfg, "--output=json",
		"issue", "attachment", "add", "PROJ-1", "--file", tmp)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatalf("attachment add succeeded on 413; want non-zero exit")
	}
	var exitErr *exec.ExitError
	if !stdlibErrors.As(err, &exitErr) {
		t.Fatalf("err is not *exec.ExitError: %v", err)
	}
	// Exit 5 = server error per Constitution Alignment +
	// http-contract.md error mapping (413 → 5).
	if exitErr.ExitCode() != 5 {
		t.Fatalf("exit code = %d, want 5 (server error per http-contract.md)", exitErr.ExitCode())
	}
	// Upstream message preserved verbatim, no generic "upload failed"
	// substitution.
	combined := stdout.String() + stderr.String()
	if !strings.Contains(combined, upstreamMsg) {
		t.Fatalf("output does not surface upstream error message verbatim:\nstdout=%s\nstderr=%s",
			stdout.String(), stderr.String())
	}
	// Negative assertion: no generic substitute.
	if strings.Contains(combined, "upload failed") {
		t.Fatalf("output contains generic substitute 'upload failed' instead of upstream message")
	}
}

// `attachment delete` contract: force-gate under --no-input,
// success envelope shape, DELETE wire request to /attachment/{id}.
func TestAttachmentDeleteForceGateAndWireContract(t *testing.T) {
	t.Run("no-input without --force exits 3", func(t *testing.T) {
		bin := buildJiraBinary(t)
		cmd := exec.Command(bin, "--output=json", "--no-input",
			"issue", "attachment", "delete", "PROJ-1", "10042")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		if err == nil {
			t.Fatalf("attachment delete --no-input without --force succeeded; want exit 3")
		}
		var exitErr *exec.ExitError
		if !stdlibErrors.As(err, &exitErr) {
			t.Fatalf("err is not *exec.ExitError: %v", err)
		}
		if exitErr.ExitCode() != 3 {
			t.Fatalf("exit code = %d, want 3 (validation)", exitErr.ExitCode())
		}
	})

	t.Run("with --force deletes by id", func(t *testing.T) {
		var sawMethod, sawPath string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sawMethod = r.Method
			sawPath = r.URL.Path
			w.WriteHeader(http.StatusNoContent)
		}))
		defer srv.Close()

		cfg := jiraConfig(t, srv.URL)
		t.Setenv("JIRA_TOKEN_DEFAULT", "test-token")
		cmd := exec.Command(buildJiraBinary(t), "--config", cfg, "--output=json", "--no-input",
			"issue", "attachment", "delete", "PROJ-1", "10042", "--force")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("attachment delete --force error = %v\n%s", err, out)
		}
		if sawMethod != http.MethodDelete {
			t.Fatalf("wire method = %q, want DELETE", sawMethod)
		}
		// data-model.md: "by attachment id, not issue key (Atlassian's
		// endpoint is DELETE /attachment/{id})".
		if sawPath != "/rest/api/3/attachment/10042" {
			t.Fatalf("wire path = %q, want /rest/api/3/attachment/10042", sawPath)
		}
		var env map[string]any
		if err := json.Unmarshal(out, &env); err != nil {
			t.Fatalf("attachment delete output not JSON: %v\n%s", err, out)
		}
		data, _ := env["data"].(map[string]any)
		if got, _ := data["attachment_id"].(string); got != "10042" {
			t.Fatalf("data.attachment_id = %q, want 10042", got)
		}
		if got, _ := data["deleted"].(bool); !got {
			t.Fatalf("data.deleted = %v, want true", got)
		}
	})
}

// `attachment download` modes:
// (1) --to PATH, (2) clobber-protect under --to (no --force) →
// exit 3, (3) clobber-protect cleared by --force.
//
// TTY-mode current-dir behavior is exercised separately (the TTY-gated
// dispatch is hard to fake in a test exec); here we cover the
// envelope shape for --to and the clobber-protect contract that
// must hold without entering the HTTP path.
func TestAttachmentDownloadModesAndClobberProtect(t *testing.T) {
	const payload = "binary-bytes-for-test"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/rest/api/3/attachment/content/") {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Disposition", `attachment; filename="server.png"`)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	cfg := jiraConfig(t, srv.URL)
	t.Setenv("JIRA_TOKEN_DEFAULT", "test-token")

	t.Run("--to writes file and emits envelope", func(t *testing.T) {
		// --to is confined to the working directory, so the download runs
		// with its cwd inside the temp dir and a relative target.
		workDir := t.TempDir()
		cmd := exec.Command(buildJiraBinary(t), "--config", cfg, "--output=json",
			"issue", "attachment", "download", "PROJ-1", "10042", "--to", "local.png")
		cmd.Dir = workDir
		stdout, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("attachment download error = %v\n%s", err, stdout)
		}
		got, err := os.ReadFile(filepath.Join(workDir, "local.png"))
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		if string(got) != payload {
			t.Fatalf("file bytes = %q, want %q", string(got), payload)
		}
		var env map[string]any
		if err := json.Unmarshal(stdout, &env); err != nil {
			t.Fatalf("download output not JSON: %v\n%s", err, stdout)
		}
		data, _ := env["data"].(map[string]any)
		if got, _ := data["mode"].(string); got != "output" {
			t.Fatalf("data.mode = %q, want output", got)
		}
		if got, _ := data["written_to"].(string); got != "local.png" {
			t.Fatalf("data.written_to = %q, want local.png", got)
		}
		if bytesField, _ := data["bytes"].(float64); int(bytesField) != len(payload) {
			t.Fatalf("data.bytes = %v, want %d", bytesField, len(payload))
		}
	})

	t.Run("--to existing file without --force exits 3", func(t *testing.T) {
		workDir := t.TempDir()
		existing := filepath.Join(workDir, "existing.png")
		if err := os.WriteFile(existing, []byte("KEEP"), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		bin := buildJiraBinary(t)
		cmd := exec.Command(bin, "--config", cfg, "--output=json", "--no-input",
			"issue", "attachment", "download", "PROJ-1", "10042", "--to", "existing.png")
		cmd.Dir = workDir
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		if err == nil {
			t.Fatalf("download to existing path without --force succeeded; want exit 3")
		}
		var exitErr *exec.ExitError
		if !stdlibErrors.As(err, &exitErr) {
			t.Fatalf("err is not *exec.ExitError: %v", err)
		}
		if exitErr.ExitCode() != 3 {
			t.Fatalf("exit code = %d, want 3 (clobber-protect)", exitErr.ExitCode())
		}
		// Existing file MUST NOT be touched without --force.
		got, err := os.ReadFile(existing)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		if string(got) != "KEEP" {
			t.Fatalf("existing file was clobbered: contents = %q", string(got))
		}
	})

	t.Run("--to existing file with --force overwrites", func(t *testing.T) {
		workDir := t.TempDir()
		existing := filepath.Join(workDir, "existing.png")
		if err := os.WriteFile(existing, []byte("OLD"), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		cmd := exec.Command(buildJiraBinary(t), "--config", cfg, "--output=json",
			"issue", "attachment", "download", "PROJ-1", "10042", "--to", "existing.png", "--force")
		cmd.Dir = workDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("download --force error = %v\n%s", err, out)
		}
		got, err := os.ReadFile(existing)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		if string(got) != payload {
			t.Fatalf("file bytes after --force = %q, want %q", string(got), payload)
		}
	})

	// An absolute --to that stays inside the working directory remains
	// allowed — confinement is about escapes, not path spelling.
	t.Run("--to absolute path inside working dir is allowed", func(t *testing.T) {
		workDir := t.TempDir()
		out := filepath.Join(workDir, "abs.png")
		cmd := exec.Command(buildJiraBinary(t), "--config", cfg, "--output=json",
			"issue", "attachment", "download", "PROJ-1", "10042", "--to", out)
		cmd.Dir = workDir
		stdout, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("attachment download error = %v\n%s", err, stdout)
		}
		if got, err := os.ReadFile(out); err != nil || string(got) != payload {
			t.Fatalf("ReadFile = %q, %v; want %q", string(got), err, payload)
		}
	})
}

// `attachment download --to` is confined to the working directory: a
// `..` traversal or an absolute path outside it is a validation error
// (exit 3) rejected BEFORE any HTTP call — the guard fires even with no
// server behind the profile, which is exactly what this test relies on.
func TestAttachmentDownloadRejectsWorkingDirEscape(t *testing.T) {
	cfg := jiraConfig(t, "http://127.0.0.1:1")
	t.Setenv("JIRA_TOKEN_DEFAULT", "test-token")
	bin := buildJiraBinary(t)

	for name, target := range map[string]string{
		"relative traversal":     filepath.Join("..", "escape.bin"),
		"nested traversal":       filepath.Join("sub", "..", "..", "escape.bin"),
		"absolute outside tree":  filepath.Join(os.TempDir(), "escape.bin"),
		"traversal with dry-run": filepath.Join("..", "escape.bin"),
	} {
		t.Run(name, func(t *testing.T) {
			args := []string{
				"--config", cfg, "--output=json", "--no-input",
				"issue", "attachment", "download", "PROJ-1", "10042", "--to", target,
			}
			if name == "traversal with dry-run" {
				args = append(args, "--dry-run")
			}
			cmd := exec.Command(bin, args...)
			cmd.Dir = t.TempDir()
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			err := cmd.Run()
			if err == nil {
				t.Fatalf("download --to %q succeeded; want exit 3\n%s", target, stdout.String())
			}
			var exitErr *exec.ExitError
			if !stdlibErrors.As(err, &exitErr) {
				t.Fatalf("err is not *exec.ExitError: %v", err)
			}
			if exitErr.ExitCode() != 3 {
				t.Fatalf("exit code = %d, want 3 (path confinement)\n%s%s", exitErr.ExitCode(), stdout.String(), stderr.String())
			}
			combined := stdout.String() + stderr.String()
			if !strings.Contains(combined, "outside the working directory") {
				t.Fatalf("rejection lacks the remediation message:\n%s", combined)
			}
			if _, err := os.Stat(filepath.Join(cmd.Dir, "..", "escape.bin")); err == nil {
				t.Fatalf("escape file was written outside the working directory")
			}
		})
	}
}
