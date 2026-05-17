// boardScopeEnvelopeData translates a resolved BoardScope into the
// envelope's `data.board_scope` map. The `applied` flag tracks whether
// a JQL clause was actually emitted — not just whether a board was
// identified — so an envelope reader can correlate the envelope's
// scope with the JQL string in `data.jql`.
package main

import (
	"testing"

	"github.com/matcra587/jira-cli/pkg/jira"
)

func TestBoardScopeEnvelopeDataAppliedFalseOnZeroValueScope(t *testing.T) {
	t.Parallel()

	out := boardScopeEnvelopeData(jira.BoardScope{})
	if got, ok := out["applied"]; !ok || got != false {
		t.Errorf("zero-value scope: applied = %v (ok=%v); want false", got, ok)
	}
}

func TestBoardScopeEnvelopeDataAppliedFalseWhenProjectKeysEmpty(t *testing.T) {
	t.Parallel()

	id := 42
	name := "Empty Project Board"
	typ := "scrum"
	scope := jira.BoardScope{
		Board: jira.Board{
			ID:          &id,
			Name:        &name,
			Type:        &typ,
			ProjectKeys: []string{},
		},
		Precedence: "flag",
	}
	out := boardScopeEnvelopeData(scope)

	// applied=false because no JQL clause is emitted (JQLClause returns
	// empty for zero project keys).
	if got, ok := out["applied"]; !ok || got != false {
		t.Errorf("applied = %v; want false (no JQL clause emitted)", got)
	}
	// Board metadata should still surface so the user can see what
	// resolved — the resolved board exists, just couldn't constrain the
	// query.
	if out["id"] != 42 {
		t.Errorf("id = %v; want 42", out["id"])
	}
	if out["name"] != "Empty Project Board" {
		t.Errorf("name = %v; want %q", out["name"], "Empty Project Board")
	}
	if out["type"] != "scrum" {
		t.Errorf("type = %v; want %q", out["type"], "scrum")
	}
	keys, ok := out["project_keys"].([]string)
	if !ok || len(keys) != 0 {
		t.Errorf("project_keys = %v; want []string{}", out["project_keys"])
	}
}

func TestBoardScopeEnvelopeDataAppliedTrueWhenProjectKeysPresent(t *testing.T) {
	t.Parallel()

	id := 7
	name := "Engineering Sprint"
	typ := "scrum"
	scope := jira.BoardScope{
		Board: jira.Board{
			ID:          &id,
			Name:        &name,
			Type:        &typ,
			ProjectKeys: []string{"ENG", "PLAT"},
		},
		Precedence: "flag",
	}
	out := boardScopeEnvelopeData(scope)
	if out["applied"] != true {
		t.Errorf("applied = %v; want true", out["applied"])
	}
	if out["id"] != 7 {
		t.Errorf("id = %v; want 7", out["id"])
	}
	keys, ok := out["project_keys"].([]string)
	if !ok || len(keys) != 2 || keys[0] != "ENG" || keys[1] != "PLAT" {
		t.Errorf("project_keys = %v; want [ENG PLAT]", out["project_keys"])
	}
}

func TestApplyBoardClauseToJQLParenthesizesTopLevelORBeforeOrderBy(t *testing.T) {
	t.Parallel()

	scope := jira.BoardScope{
		Board: jira.Board{ProjectKeys: []string{"ENG", "PLAT"}},
	}
	got := applyBoardClauseToJQL("status = Open OR priority = High ORDER BY created DESC", scope)
	want := `project in (ENG, PLAT) AND (status = Open OR priority = High) ORDER BY created DESC`
	if got != want {
		t.Fatalf("applyBoardClauseToJQL() = %q, want %q", got, want)
	}
}

func TestApplyBoardClauseToJQLDoesNotDoubleWrapSimpleClause(t *testing.T) {
	t.Parallel()

	scope := jira.BoardScope{
		Board: jira.Board{ProjectKeys: []string{"ENG"}},
	}
	got := applyBoardClauseToJQL("status = Open ORDER BY updated DESC", scope)
	want := `project in (ENG) AND status = Open ORDER BY updated DESC`
	if got != want {
		t.Fatalf("applyBoardClauseToJQL() = %q, want %q", got, want)
	}
}
