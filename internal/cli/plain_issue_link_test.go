package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gechr/clog"
)

// Mutation success lines make issue keys clickable: a field whose value is
// a bare issue key renders as an OSC 8 link to its browse URL when the
// output supports color, and degrades to plain text off a TTY — clog owns
// the fallback, not the renderer.

func linkLogger(buf *bytes.Buffer, mode clog.ColorMode) *clog.Logger {
	logger := clog.New(clog.NewOutput(buf, mode))
	logger.SetOmitEmpty(true)
	return logger
}

func TestGenericPlainLinksIssueKeys(t *testing.T) {
	var buf bytes.Buffer
	cfg := defaultPlainConfig()
	cfg.baseURL = "https://example.atlassian.net"
	err := writeGenericPlain(linkLogger(&buf, clog.ColorAlways), cfg, "Edited issue", map[string]any{
		"issue": "PROJ-123",
	})
	if err != nil {
		t.Fatalf("writeGenericPlain: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "\x1b]8;;https://example.atlassian.net/browse/PROJ-123\x1b\\") {
		t.Fatalf("issue key must link to its browse URL, got %q", got)
	}
	if !strings.Contains(got, "PROJ-123") {
		t.Fatalf("display text must remain the key, got %q", got)
	}
}

func TestGenericPlainKeyLinkDegradesWithoutColor(t *testing.T) {
	var buf bytes.Buffer
	cfg := defaultPlainConfig()
	cfg.baseURL = "https://example.atlassian.net"
	err := writeGenericPlain(linkLogger(&buf, clog.ColorNever), cfg, "Edited issue", map[string]any{
		"issue": "PROJ-123",
	})
	if err != nil {
		t.Fatalf("writeGenericPlain: %v", err)
	}
	got := buf.String()
	if strings.Contains(got, "\x1b]8;;") {
		t.Fatalf("no OSC 8 escapes without color support, got %q", got)
	}
	if !strings.Contains(got, "PROJ-123") {
		t.Fatalf("key must survive as plain text, got %q", got)
	}
}

func TestGenericPlainLeavesNonKeysAndNoBaseURLAlone(t *testing.T) {
	var buf bytes.Buffer
	// No base URL: even a key-shaped value stays a plain field.
	err := writeGenericPlain(linkLogger(&buf, clog.ColorAlways), defaultPlainConfig(), "Edited issue", map[string]any{
		"issue":   "PROJ-123",
		"summary": "not A-1 key",
	})
	if err != nil {
		t.Fatalf("writeGenericPlain: %v", err)
	}
	if strings.Contains(buf.String(), "\x1b]8;;") {
		t.Fatalf("no browse URL known — nothing to link, got %q", buf.String())
	}
}
