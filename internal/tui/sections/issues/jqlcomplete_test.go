package issues

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/tui/core"
)

func testRef() jira.JQLReference {
	return jira.JQLReference{
		Fields: []jira.JQLField{
			{Value: "assignee", Operators: []string{"=", "!=", "in", "not in"}, Auto: true},
			{Value: "status", Operators: []string{"=", "!=", "in"}, Auto: true},
			{Value: "summary", Operators: []string{"~", "!~"}},
		},
		Functions: []jira.JQLFunction{
			{Value: "currentUser()"},
			{Value: "openSprints()"},
		},
	}
}

func TestJQLCompleteClassifiesPositions(t *testing.T) {
	cases := []struct {
		input string
		kind  tokenKind
		field string
		pref  string
	}{
		{"", wantField, "", ""},
		{"sta", wantField, "", "sta"},
		{"status ", wantOperator, "status", ""},
		{"status !", wantOperator, "status", "!"},
		{"status = ", wantValue, "status", ""},
		{"status = Op", wantValue, "status", "Op"},
		{"status not ", wantOperator, "status", ""},
		{"status not in ", wantValue, "status", ""},
		{"status = Open ", wantConnective, "status", ""},
		{"status = Open AND ", wantField, "", ""},
		{"status = Open AND assig", wantField, "", "assig"},
		{"status = Open ORDER BY ", wantField, "", ""},
		{"status = Open ORDER BY updated ", wantDirection, "updated", ""},
		{"status = Open ORDER BY updated DE", wantDirection, "updated", "DE"},
		{"assignee is EMPTY ", wantConnective, "assignee", ""},
	}
	for _, c := range cases {
		got := jqlComplete(c.input)
		if got.kind != c.kind || got.prefix != c.pref || (c.field != "" && !strings.EqualFold(got.field, c.field)) {
			t.Errorf("jqlComplete(%q) = kind=%d field=%q prefix=%q, want kind=%d field=%q prefix=%q",
				c.input, got.kind, got.field, got.prefix, c.kind, c.field, c.pref)
		}
	}
}

func TestJQLCompleteQuotedValueInProgress(t *testing.T) {
	got := jqlComplete(`status = "In Pr`)
	if got.kind != wantValue || got.prefix != "In Pr" || !got.quoted {
		t.Fatalf("quoted value context = %+v", got)
	}
}

func TestCandidatesPerPosition(t *testing.T) {
	ref := testRef()
	// Operator position offers the active field's own operators.
	ops := candidatesFor(ref, jqlContext{kind: wantOperator, field: "summary"})
	if !reflect.DeepEqual(ops, []string{"!~ ", "~ "}) {
		t.Errorf("summary operators = %v", ops)
	}
	// Value position offers the functions (live values merge in separately).
	vals := candidatesFor(ref, jqlContext{kind: wantValue, field: "assignee"})
	if len(vals) != 2 || vals[0] != "currentUser()" {
		t.Errorf("value candidates = %v", vals)
	}
	// Connective position offers the structural keywords.
	conn := candidatesFor(ref, jqlContext{kind: wantConnective})
	if len(conn) != 3 {
		t.Errorf("connectives = %v", conn)
	}
}

func TestCompletionLinesCompleteTheCurrentTokenOnly(t *testing.T) {
	input := "status = Open AND assig"
	c := jqlComplete(input)
	lines := completionLines(input, c, candidatesFor(testRef(), c))
	if len(lines) != 1 || lines[0] != "status = Open AND assignee " {
		t.Fatalf("lines = %v, want the assignee completion", lines)
	}
}

func TestCompletionLinesQuoteMultiWordValuesAtEmptyPrefix(t *testing.T) {
	// Just after the operator, multi-word values complete fully quoted.
	input := "status = "
	c := jqlComplete(input)
	lines := completionLines(input, c, []string{"In Progress", "Open"})
	want := []string{`status = "In Progress"`, `status = Open`}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("lines = %v, want %v", lines, want)
	}
}

func TestCompletionLinesDropMultiWordValuesAfterBarePrefix(t *testing.T) {
	// A quote injected mid-token could never prefix-match the typed text,
	// so bare prefixes only complete space-free values.
	input := "status = In"
	c := jqlComplete(input)
	lines := completionLines(input, c, []string{"In Progress", "Indexed"})
	want := []string{"status = Indexed"}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("lines = %v, want %v", lines, want)
	}
}

func TestCompletionLinesKeepUserQuote(t *testing.T) {
	input := `status = "In`
	c := jqlComplete(input)
	lines := completionLines(input, c, []string{"In Progress"})
	if len(lines) != 1 || lines[0] != `status = "In Progress"` {
		t.Fatalf("quoted lines = %v", lines)
	}
}

func TestCompletionLinesInsertSpaceAfterClosedQuote(t *testing.T) {
	// A closed quote ends its token without a trailing space; the next
	// keyword must not glue onto it.
	input := `status = "In Progress"`
	c := jqlComplete(input)
	if c.kind != wantConnective {
		t.Fatalf("after a closed quote kind = %d, want wantConnective", c.kind)
	}
	lines := completionLines(input, c, jqlConnectives)
	if len(lines) == 0 || lines[0] != `status = "In Progress" AND ` {
		t.Fatalf("lines = %v, want a space before AND", lines)
	}
}

func TestCompletionLinesSkipFunctionsInsideQuotes(t *testing.T) {
	// Inside quotes a function call would be a literal string, not a call.
	input := `assignee = "cur`
	c := jqlComplete(input)
	lines := completionLines(input, c, []string{"currentUser()", "curt"})
	want := []string{`assignee = "curt"`}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("lines = %v, want %v", lines, want)
	}
}

func TestCompletionLinesWrapListValuesInParens(t *testing.T) {
	// IN/NOT IN takes a parenthesised list.
	input := "status in "
	c := jqlComplete(input)
	if !c.list {
		t.Fatalf("context after in = %+v, want list", c)
	}
	lines := completionLines(input, c, []string{"In Progress", "Open"})
	want := []string{`status in ("In Progress")`, `status in (Open)`}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("lines = %v, want %v", lines, want)
	}
}

func TestCompletionLinesDropListValuesAfterBarePrefix(t *testing.T) {
	// A typed prefix can't grow an opening paren in front of itself.
	input := "status in Op"
	c := jqlComplete(input)
	if lines := completionLines(input, c, []string{"Open"}); lines != nil {
		t.Fatalf("lines = %v, want none", lines)
	}
}

func TestOrderByDirectionIsTerminal(t *testing.T) {
	// After ORDER BY <field> <direction> nothing may follow — no AND/OR.
	c := jqlComplete("status = Open ORDER BY updated DESC ")
	if c.kind != wantSortMore {
		t.Fatalf("kind = %d, want wantSortMore", c.kind)
	}
	if cands := candidatesFor(testRef(), c); len(cands) != 0 {
		t.Fatalf("candidates after direction = %v, want none", cands)
	}
}

func TestCompletionLinesAreCaseInsensitiveOnThePrefix(t *testing.T) {
	input := "STATUS = open AND sta"
	c := jqlComplete(input)
	lines := completionLines(input, c, candidatesFor(testRef(), c))
	if len(lines) != 1 || !strings.HasSuffix(lines[0], "status ") {
		t.Fatalf("case-insensitive completion = %v", lines)
	}
}

// --- search wiring ---

func completeSearchModel(t *testing.T) *SearchModel {
	t.Helper()
	svc := fakeServices{
		issue: fakeIssueSvc{},
		jql: fakeJQLSvc{
			ref:    testRef(),
			values: []jira.JQLSuggestion{{Value: "In Progress"}, {Value: "In Review"}},
		},
	}
	ctx := newTestCtx(svc)
	s := NewSearch(ctx).(*SearchModel)
	s.Init(ctx)
	return s
}

func TestOpenEditFetchesReferenceDataOnce(t *testing.T) {
	s := completeSearchModel(t)
	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("first edit must fetch the JQL reference data")
	}
	msg := taskMsg(t, cmd)
	s.Update(msg)
	if !s.jqlRefLoaded || len(s.jqlRef.Fields) != 3 {
		t.Fatalf("reference data not stored: loaded=%v fields=%d", s.jqlRefLoaded, len(s.jqlRef.Fields))
	}
}

func TestTypingGetsTokenAwareFieldCompletion(t *testing.T) {
	s := completeSearchModel(t)
	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	s.Update(taskMsg(t, cmd)) // reference data lands
	for _, r := range "sta" {
		s.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if v := ansi.Strip(s.jqlInput.View()); !strings.Contains(v, "status") {
		t.Errorf("editor should ghost-complete the field name:\n%q", v)
	}
	s.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if got := s.jqlInput.Value(); got != "status " {
		t.Errorf("tab accepted %q, want %q", got, "status ")
	}
}

func TestValuePositionFetchesAndCompletesLiveValues(t *testing.T) {
	s := completeSearchModel(t)
	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	s.Update(taskMsg(t, cmd))
	for _, r := range "status = " {
		key := tea.KeyPressMsg{Code: rune(r), Text: string(r)}
		if r == ' ' {
			key = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
		}
		_, c := s.Update(key)
		if c != nil {
			// Value position fired the live-values fetch; land it.
			s.Update(taskMsg(t, c))
		}
	}
	if s.valueSugsFor != "status" || len(s.valueSugs) != 2 {
		t.Fatalf("live values not cached: for=%q n=%d", s.valueSugsFor, len(s.valueSugs))
	}
	// At the empty value position multi-word values ghost in fully quoted.
	s.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if got := s.jqlInput.Value(); got != `status = "In Progress"` {
		t.Errorf("tab at empty prefix accepted %q, want quoted In Progress", got)
	}
}

func TestQuotedPrefixCompletesLiveValue(t *testing.T) {
	s := completeSearchModel(t)
	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	s.Update(taskMsg(t, cmd))
	s.valueSugs = []string{"In Progress", "In Review"}
	s.valueSugsFor = "status"
	s.jqlInput.SetValue(`status = "In R`)
	s.refreshSuggestions()
	s.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if got := s.jqlInput.Value(); got != `status = "In Review"` {
		t.Errorf("quoted completion accepted %q, want In Review", got)
	}
}

func TestFailedValueFetchRetriesOnNextKeystroke(t *testing.T) {
	s := completeSearchModel(t)
	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	s.Update(taskMsg(t, cmd))
	s.jqlInput.SetValue("status = ")
	s.refreshSuggestions() // arms sugField/sugPrefix and fires the fetch
	if s.sugField != "status" {
		t.Fatalf("fetch not armed: sugField=%q", s.sugField)
	}
	// The fetch fails: the attempted field/prefix must be forgotten so the
	// same position retries instead of staying silent.
	s.Update(core.TaskFinishedMsg{Scope: s.jqlSuggScope(), Err: errors.New("boom")})
	if s.sugField != "" || s.sugPrefix != "" {
		t.Fatalf("error must reset the armed fetch: field=%q prefix=%q", s.sugField, s.sugPrefix)
	}
	if cmd := s.refreshSuggestions(); cmd == nil {
		t.Fatal("same position must refetch after a failure")
	}
}

func TestNoReferenceDataFallsBackToSavedQueries(t *testing.T) {
	ctx := newTestCtx(fakeServices{issue: fakeIssueSvc{}})
	s := NewSearch(ctx).(*SearchModel)
	s.Init(ctx)
	s.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // ref fetch fires but never lands
	for _, r := range "assignee" {
		s.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if v := s.jqlInput.View(); !strings.Contains(v, "currentUser()") {
		t.Errorf("fallback whole-query suggestion missing:\n%q", v)
	}
}

func TestValueFieldOnlyForSuggestableFields(t *testing.T) {
	ref := testRef()
	if got := valueField(ref, jqlComplete("status = Op")); got != "status" {
		t.Errorf("status should be suggestable, got %q", got)
	}
	if got := valueField(ref, jqlComplete("summary ~ fix")); got != "" {
		t.Errorf("summary (no auto) must not fetch values, got %q", got)
	}
	if got := valueField(ref, jqlComplete("status ")); got != "" {
		t.Errorf("operator position must not fetch values, got %q", got)
	}
}
