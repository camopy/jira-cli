package contract

// Contract tests for `jira issue attachment ….
//
// These tests drive the CLI binary end-to-end against an httptest server
// faking Atlassian's REST surface. Each test pins one wire-level
// invariant from contracts/{command-schemas,envelope-shapes,http-contract}.md
// across the issue lifecycle commands.
//
// All tests will fail until the lead's integration pass registers
// IssueAttachmentCommand under issueCommand in cmd/jira/commands.go.

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

	cmd := exec.Command("go", "run", "../../cmd/jira", "--config", cfg, "--output=json",
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
	pagination, ok := data["pagination"].(map[string]any)
	if !ok {
		t.Fatalf("data.pagination missing or wrong type: %s", out)
	}
	for _, key := range []string{"total", "start_at", "max_results", "is_last"} {
		if _, exists := pagination[key]; !exists {
			t.Fatalf("pagination missing %q: %v", key, pagination)
		}
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

	cmd := exec.Command("go", "run", "../../cmd/jira", "--config", cfg, "--output=json",
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
		cmd := exec.Command("go", "run", "../../cmd/jira", "--config", cfg, "--output=json", "--no-input",
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
		out := filepath.Join(t.TempDir(), "local.png")
		cmd := exec.Command("go", "run", "../../cmd/jira", "--config", cfg, "--output=json",
			"issue", "attachment", "download", "PROJ-1", "10042", "--to", out)
		stdout, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("attachment download error = %v\n%s", err, stdout)
		}
		got, err := os.ReadFile(out)
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
		if got, _ := data["written_to"].(string); got != out {
			t.Fatalf("data.written_to = %q, want %q", got, out)
		}
		if bytesField, _ := data["bytes"].(float64); int(bytesField) != len(payload) {
			t.Fatalf("data.bytes = %v, want %d", bytesField, len(payload))
		}
	})

	t.Run("--to existing file without --force exits 3", func(t *testing.T) {
		existing := filepath.Join(t.TempDir(), "existing.png")
		if err := os.WriteFile(existing, []byte("KEEP"), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		bin := buildJiraBinary(t)
		cmd := exec.Command(bin, "--config", cfg, "--output=json", "--no-input",
			"issue", "attachment", "download", "PROJ-1", "10042", "--to", existing)
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
		existing := filepath.Join(t.TempDir(), "existing.png")
		if err := os.WriteFile(existing, []byte("OLD"), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		cmd := exec.Command("go", "run", "../../cmd/jira", "--config", cfg, "--output=json",
			"issue", "attachment", "download", "PROJ-1", "10042", "--to", existing, "--force")
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
}
