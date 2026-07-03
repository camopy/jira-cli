package cli

import (
	"bytes"
	"strings"
	"testing"
)

// Multi-key plain output identifies each entry exactly once, and the
// identification is owned by the entry's own renderer — never by a
// duplicated heading above it.

func keyedData(command string, entries ...map[string]any) map[string]any {
	results := make([]any, 0, len(entries))
	for _, e := range entries {
		results = append(results, e)
	}
	_ = command
	return map[string]any{
		"results":   results,
		"succeeded": len(entries),
		"failed":    0,
	}
}

func TestKeyedResultsPlainSelfIdentifyingEntryPrintsKeyOnce(t *testing.T) {
	var buf bytes.Buffer
	data := keyedData(
		"issue.transition",
		map[string]any{"key": "JCT-9", "ok": true, "data": map[string]any{
			"issue": "JCT-9", "transition": "31", "dry_run": true,
		}},
	)
	if err := WriteKeyedResultsPlain(&buf, "issue.transition", data); err != nil {
		t.Fatalf("WriteKeyedResultsPlain: %v", err)
	}
	got := buf.String()
	if n := strings.Count(got, "JCT-9"); n != 1 {
		t.Fatalf("key must appear exactly once (in the entry's own line), got %d:\n%s", n, got)
	}
	if !strings.Contains(got, "issue=JCT-9") {
		t.Fatalf("the entry line must self-identify: %s", got)
	}
}

func TestKeyedResultsPlainAnonymousEntryGetsKeyInjected(t *testing.T) {
	var buf bytes.Buffer
	data := keyedData(
		"cache.refresh",
		map[string]any{"key": "boards", "ok": true, "data": map[string]any{
			"status": "refreshed", "count": 3,
		}},
	)
	if err := WriteKeyedResultsPlain(&buf, "cache.refresh", data); err != nil {
		t.Fatalf("WriteKeyedResultsPlain: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "key=boards") {
		t.Fatalf("anonymous entry data must gain the key field: %s", got)
	}
	if n := strings.Count(got, "boards"); n != 1 {
		t.Fatalf("key must appear exactly once, got %d:\n%s", n, got)
	}
}

func TestKeyedResultsPlainEntryIdentifiedByAnyFieldValue(t *testing.T) {
	var buf bytes.Buffer
	// The data names the entry under its own field (resource=boards);
	// injecting key=boards on top would duplicate it.
	data := keyedData(
		"cache.refresh",
		map[string]any{"key": "boards", "ok": true, "data": map[string]any{
			"resource": "boards", "status": "refreshed",
		}},
	)
	if err := WriteKeyedResultsPlain(&buf, "cache.refresh", data); err != nil {
		t.Fatalf("WriteKeyedResultsPlain: %v", err)
	}
	got := buf.String()
	if strings.Contains(got, "key=boards") {
		t.Fatalf("self-identified data must not gain an extra key field: %s", got)
	}
	if !strings.Contains(got, "resource=boards") {
		t.Fatalf("the data's own field identifies the entry: %s", got)
	}
}

func TestKeyedResultsPlainBlockRendererFoldsKeyIntoHeader(t *testing.T) {
	var buf bytes.Buffer
	data := keyedData(
		"issue.comment.list",
		map[string]any{"key": "JCT-7", "ok": true, "data": map[string]any{
			"comments": []any{map[string]any{
				"id":      "100",
				"body":    "hello",
				"author":  map[string]any{"display_name": "Alice"},
				"created": "2026-04-01T10:00:00.000+0000",
				"updated": "2026-04-01T10:00:00.000+0000",
			}},
		}},
	)
	if err := WriteKeyedResultsPlain(&buf, "issue.comment.list", data); err != nil {
		t.Fatalf("WriteKeyedResultsPlain: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "Comments on JCT-7") {
		t.Fatalf("block renderer must fold the key into its header: %s", got)
	}
	if n := strings.Count(got, "JCT-7"); n != 1 {
		t.Fatalf("key must appear exactly once, got %d:\n%s", n, got)
	}
}

func TestKeyedResultsPlainSingleKeyBlockKeepsPlainHeader(t *testing.T) {
	var buf bytes.Buffer
	// Single-target renders never set a result key; the header stays bare.
	err := WriteCommentListPlain(&buf, "issue.comment.list", map[string]any{
		"comments": []any{},
	})
	if err != nil {
		t.Fatalf("WriteCommentListPlain: %v", err)
	}
	if !strings.Contains(buf.String(), "Comments") || strings.Contains(buf.String(), " on ") {
		t.Fatalf("single-target header must stay bare: %s", buf.String())
	}
}
