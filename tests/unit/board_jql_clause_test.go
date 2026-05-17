package unit

// BoardScope.JQLClause emits `project in (P1, P2)` from the board's
// cached project keys, returning a (clause, ok) pair so callers branch
// on the bool rather than re-deriving `len(ProjectKeys) > 0`.
//
// Edge cases pinned by data-model.md:
//   - empty project list → ("", false) (caller treats as "no scope")
//   - project key order is preserved verbatim (cache writes them sorted)
//   - single-project boards still emit the full `project in (...)` shape

import (
	"testing"

	"github.com/matcra587/jira-cli/internal/jira"
)

func TestBoardScopeJQLClauseSingleProject(t *testing.T) {
	scope := jira.BoardScope{
		Board: jira.Board{ProjectKeys: []string{"ENG"}},
	}
	got, ok := scope.JQLClause()
	if !ok {
		t.Fatalf("JQLClause ok = false; want true")
	}
	if want := "project in (ENG)"; got != want {
		t.Fatalf("JQLClause = %q; want %q", got, want)
	}
}

func TestBoardScopeJQLClauseMultiProject(t *testing.T) {
	scope := jira.BoardScope{
		Board: jira.Board{ProjectKeys: []string{"ENG", "PLAT"}},
	}
	got, ok := scope.JQLClause()
	if !ok {
		t.Fatalf("JQLClause ok = false; want true")
	}
	if want := "project in (ENG, PLAT)"; got != want {
		t.Fatalf("JQLClause = %q; want %q", got, want)
	}
}

func TestBoardScopeJQLClauseEmptyProjectList(t *testing.T) {
	// Board with no associated projects → ("", false).
	scope := jira.BoardScope{
		Board: jira.Board{ProjectKeys: []string{}},
	}
	got, ok := scope.JQLClause()
	if ok {
		t.Fatalf("JQLClause ok = true; want false (empty project list)")
	}
	if got != "" {
		t.Fatalf("JQLClause on empty project list = %q; want empty string", got)
	}
}

func TestBoardScopeJQLClauseNilProjectKeys(t *testing.T) {
	// Defensive: nil slice should behave like empty slice, not panic.
	scope := jira.BoardScope{
		Board: jira.Board{ProjectKeys: nil},
	}
	got, ok := scope.JQLClause()
	if ok {
		t.Fatalf("JQLClause ok = true; want false (nil project keys)")
	}
	if got != "" {
		t.Fatalf("JQLClause on nil project keys = %q; want empty string", got)
	}
}

func TestBoardScopeJQLClausePreservesProjectKeyOrder(t *testing.T) {
	// The cache writes project keys sorted ascending; JQLClause emits in
	// that order verbatim (deterministic for envelope round-trips).
	scope := jira.BoardScope{
		Board: jira.Board{ProjectKeys: []string{"AAA", "BBB", "CCC"}},
	}
	got, ok := scope.JQLClause()
	if !ok {
		t.Fatalf("JQLClause ok = false; want true")
	}
	if want := "project in (AAA, BBB, CCC)"; got != want {
		t.Fatalf("JQLClause = %q; want %q", got, want)
	}
}
