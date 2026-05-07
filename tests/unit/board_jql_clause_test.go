package unit

// BoardScope.JQLClause emits `project in (P1, P2)` from the board's
// cached project keys.
//
// Edge cases pinned by data-model.md:
//   - empty project list → empty string (caller treats as "no scope")
//   - project key order is preserved verbatim (cache writes them sorted)
//   - single-project boards still emit the full `project in (...)` shape

import (
	"testing"

	"github.com/matcra587/jira-cli/pkg/jira"
)

func TestBoardScopeJQLClauseSingleProject(t *testing.T) {
	scope := jira.BoardScope{
		Board: jira.Board{ProjectKeys: []string{"ENG"}},
	}
	got := scope.JQLClause()
	want := "project in (ENG)"
	if got != want {
		t.Fatalf("JQLClause = %q; want %q", got, want)
	}
}

func TestBoardScopeJQLClauseMultiProject(t *testing.T) {
	scope := jira.BoardScope{
		Board: jira.Board{ProjectKeys: []string{"ENG", "PLAT"}},
	}
	got := scope.JQLClause()
	want := "project in (ENG, PLAT)"
	if got != want {
		t.Fatalf("JQLClause = %q; want %q", got, want)
	}
}

func TestBoardScopeJQLClauseEmptyProjectList(t *testing.T) {
	// Board with no associated projects → empty string.
	// Caller treats empty string as "no scope to inject".
	scope := jira.BoardScope{
		Board: jira.Board{ProjectKeys: []string{}},
	}
	got := scope.JQLClause()
	if got != "" {
		t.Fatalf("JQLClause on empty project list = %q; want empty string", got)
	}
}

func TestBoardScopeJQLClauseNilProjectKeys(t *testing.T) {
	// Defensive: nil slice should behave like empty slice, not panic.
	scope := jira.BoardScope{
		Board: jira.Board{ProjectKeys: nil},
	}
	got := scope.JQLClause()
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
	got := scope.JQLClause()
	want := "project in (AAA, BBB, CCC)"
	if got != want {
		t.Fatalf("JQLClause = %q; want %q", got, want)
	}
}
