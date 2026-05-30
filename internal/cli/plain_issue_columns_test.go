package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gechr/x/ansi"
)

func columnsSampleData() map[string]any {
	return map[string]any{
		"detail": false,
		"issues": []map[string]any{
			{
				"key":      "SAM1-7",
				"summary":  "Create wallet integration",
				"status":   "In Progress",
				"assignee": "Riley Chen",
				"priority": "High",
				"updated":  "2026-05-30T12:00:00.000+0000",
			},
		},
	}
}

func TestIssueListTSVDefaultColumns(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := WriteCommandPlain(&buf, "issue.list", columnsSampleData(), WithPlainTSV(true), WithPlainTTY(false)); err != nil {
		t.Fatalf("WriteCommandPlain() error = %v", err)
	}
	got := buf.String()

	if strings.Contains(got, "listed issues") {
		t.Fatalf("TSV output should omit the human status line:\n%s", got)
	}
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("TSV output should carry no ANSI styling:\n%q", got)
	}
	for _, want := range []string{
		"KEY\tSUMMARY\tSTATUS\tASSIGNEE\tPRIORITY",
		"SAM1-7\tCreate wallet integration\tIn Progress\tRiley Chen\tHigh",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("TSV output missing %q:\n%s", want, got)
		}
	}
}

func TestIssueListTSVSelectedColumns(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := WriteCommandPlain(&buf, "issue.list", columnsSampleData(),
		WithPlainTSV(true), WithPlainTTY(false), WithPlainColumns([]string{"key", "updated", "status"}))
	if err != nil {
		t.Fatalf("WriteCommandPlain() error = %v", err)
	}
	got := buf.String()

	for _, want := range []string{
		"KEY\tUPDATED\tSTATUS",
		"SAM1-7\t2026-05-30T12:00:00.000+0000\tIn Progress",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("selected-column TSV missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "SUMMARY") || strings.Contains(got, "ASSIGNEE") {
		t.Fatalf("selected-column TSV should drop unselected columns:\n%s", got)
	}
}

func TestIssueListHumanColumnsSubset(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := WriteCommandPlain(&buf, "issue.list", columnsSampleData(),
		WithPlainTTY(true), WithPlainTermWidth(80), WithPlainColumns([]string{"key", "priority"}))
	if err != nil {
		t.Fatalf("WriteCommandPlain() error = %v", err)
	}
	stripped := ansi.Strip(buf.String())

	for _, want := range []string{"KEY", "PRIORITY", "SAM1-7", "High"} {
		if !strings.Contains(stripped, want) {
			t.Fatalf("human column subset missing %q:\n%s", want, stripped)
		}
	}
	for _, notWant := range []string{"SUMMARY", "ASSIGNEE", "STATUS"} {
		if strings.Contains(stripped, notWant) {
			t.Fatalf("human column subset should drop %q:\n%s", notWant, stripped)
		}
	}
}

func TestIssueListTSVEmptyResultStillPrintsHeader(t *testing.T) {
	t.Parallel()

	data := map[string]any{"detail": false, "issues": []map[string]any{}}
	var buf bytes.Buffer
	if err := WriteCommandPlain(&buf, "issue.list", data, WithPlainTSV(true), WithPlainTTY(false)); err != nil {
		t.Fatalf("WriteCommandPlain() error = %v", err)
	}
	got := strings.TrimRight(buf.String(), "\n")
	if got != "KEY\tSUMMARY\tSTATUS\tASSIGNEE\tPRIORITY" {
		t.Fatalf("empty-result TSV = %q, want a lone header line", got)
	}
}

func TestIssueColumnsUnknownNameErrors(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := WriteCommandPlain(&buf, "issue.list", columnsSampleData(),
		WithPlainTTY(false), WithPlainColumns([]string{"bogus"}))
	if err == nil {
		t.Fatalf("expected an error for an unknown column, got none:\n%s", buf.String())
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("error should name the bad column: %v", err)
	}
	for _, valid := range []string{"key", "summary", "status", "assignee", "priority", "updated"} {
		if !strings.Contains(err.Error(), valid) {
			t.Fatalf("error should list valid column %q: %v", valid, err)
		}
	}
}

func TestValidateIssueColumns(t *testing.T) {
	t.Parallel()

	if err := ValidateIssueColumns(nil); err != nil {
		t.Fatalf("nil columns should be valid: %v", err)
	}
	// Names are case-insensitive and space-trimmed.
	if err := ValidateIssueColumns([]string{"KEY", " Updated "}); err != nil {
		t.Fatalf("known columns should be valid: %v", err)
	}
	if err := ValidateIssueColumns([]string{"key", "nope"}); err == nil {
		t.Fatalf("expected an error for an unknown column")
	}
}
