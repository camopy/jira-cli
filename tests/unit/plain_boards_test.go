// Unit tests for `WriteBoardListPlain` — verifies the four-column
// layout (id / name / type / projects) with the multi-project
// descriptor and the empty-result affordance.
package unit

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gechr/x/ansi"
	"github.com/matcra587/jira-cli/internal/cli"
)

// Multi-project board renders the comma-joined project list verbatim
// when there are 1-2 keys, and collapses to "+N" overflow at 3+ keys
// ( / research.md descriptor table, applied to the  plain table).
func TestWriteBoardListPlainMultiProjectAndOverflow(t *testing.T) {
	t.Parallel()

	data := map[string]any{
		"from_cache": false,
		"fetched_at": "2026-05-06T18:30:00Z",
		"boards": []map[string]any{
			{"id": float64(1), "name": "Solo", "type": "scrum", "project_keys": []any{"ENG"}},
			{"id": float64(42), "name": "Engineering Sprint", "type": "scrum", "project_keys": []any{"ENG", "PLAT"}},
			{"id": float64(99), "name": "Wide Board", "type": "scrum", "project_keys": []any{"ENG", "PLAT", "OPS", "SRE"}},
		},
	}

	var buf bytes.Buffer
	if err := cli.WriteBoardListPlain(&buf, "boards.list", data, cli.WithPlainTTY(false)); err != nil {
		t.Fatalf("WriteBoardListPlain() error = %v", err)
	}
	got := ansi.Strip(buf.String())

	wants := []string{
		"Solo",
		"Engineering Sprint",
		"Wide Board",
		"scrum",
		"ENG",
		"PLAT",
	}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Fatalf("plain output missing %q:\n%s", w, got)
		}
	}
	// Single-project board renders just the one key (no trailing comma noise).
	if !strings.Contains(got, "ENG, PLAT") {
		t.Fatalf("expected `ENG, PLAT` in 2-project board row:\n%s", got)
	}
	// 4-project overflow collapses to "+N" form .
	if !strings.Contains(got, "+2") {
		t.Fatalf("expected `+2` overflow descriptor for 4-project board:\n%s", got)
	}
	// Single-project board's projects column does NOT contain ", " comma —
	// no other key follows.
	if strings.Contains(got, "ENG, ENG") {
		t.Fatalf("did not expect duplicated ENG entries: %s", got)
	}
}

// Empty boards list surfaces the the affordance line.
func TestWriteBoardListPlainEmptyShowsAffordance(t *testing.T) {
	t.Parallel()

	data := map[string]any{
		"from_cache": true,
		"fetched_at": "2026-05-06T18:30:00Z",
		"boards":     []map[string]any{},
	}

	var buf bytes.Buffer
	if err := cli.WriteBoardListPlain(&buf, "boards.list", data, cli.WithPlainTTY(false)); err != nil {
		t.Fatalf("WriteBoardListPlain() error = %v", err)
	}
	got := ansi.Strip(buf.String())

	// the affordance message — should advertise the cache primer remediation.
	if !strings.Contains(got, "No boards visible to this profile") {
		t.Fatalf("empty list missing the affordance line:\n%s", got)
	}
	if !strings.Contains(got, "jira cache boards --refresh") {
		t.Fatalf("empty list missing remediation hint:\n%s", got)
	}
}

// Zero-board profile (any cache state) shows the same affordance
// regardless of `from_cache`.
func TestWriteBoardListPlainZeroBoardsAffordanceWhenFreshCache(t *testing.T) {
	t.Parallel()

	for _, fromCache := range []bool{true, false} {
		data := map[string]any{
			"from_cache": fromCache,
			"fetched_at": "2026-05-06T18:30:00Z",
			"boards":     []map[string]any{},
		}
		var buf bytes.Buffer
		if err := cli.WriteBoardListPlain(&buf, "boards.list", data, cli.WithPlainTTY(false)); err != nil {
			t.Fatalf("WriteBoardListPlain() error = %v", err)
		}
		got := ansi.Strip(buf.String())
		if !strings.Contains(got, "No boards visible to this profile") {
			t.Fatalf("from_cache=%v: empty list missing affordance:\n%s", fromCache, got)
		}
	}
}
