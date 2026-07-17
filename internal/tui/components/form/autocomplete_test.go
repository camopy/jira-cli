package form

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// sug builds a suggestion whose value and label are equal — the common case for
// plain value lists.
func sug(s string) Suggestion { return Suggestion{Value: s, Label: s} }

// mentionForm is a one-area form completing @-mentions from a fixed set.
func mentionForm(fetched *[]string) Model {
	return New(Config{
		Fields: []FieldSpec{{
			Multiline: true,
			Autocomplete: &Autocomplete{
				Trigger: '@',
				Fetch: func(query string) []Suggestion {
					if fetched != nil {
						*fetched = append(*fetched, query)
					}
					var out []Suggestion
					for _, name := range []string{"alice", "alan", "bob"} {
						if strings.HasPrefix(name, query) {
							out = append(out, sug(name))
						}
					}
					return out
				},
			},
		}},
		Width: 40,
	})
}

// typeAndFetch types s and runs any fetch command the keystroke produced,
// feeding the resulting SuggestionsMsg back in — the owner's Update loop in
// miniature.
func typeAndFetch(t *testing.T, m *Model, s string) {
	t.Helper()
	cmd, ev, _ := m.Update(tea.KeyPressMsg{Text: s})
	if ev != EventNone {
		t.Fatalf("typing %q emitted %v", s, ev)
	}
	drain(m, cmd)
}

// drain executes a command tree (Update wraps the field cmd and the fetch cmd
// in a batch) and routes any SuggestionsMsg back into the form.
func drain(m *Model, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			drain(m, c)
		}
	case SuggestionsMsg:
		m.Update(msg)
	}
}

func TestTriggerToken(t *testing.T) {
	cases := []struct {
		name   string
		before string
		query  string
		width  int
		ok     bool
	}{
		{"bare text", "hello", "", 0, false},
		{"trigger at start", "@al", "al", 3, true},
		{"trigger mid-sentence", "ping @bo", "bo", 3, true},
		{"trigger inside word", "mail@ex", "", 0, false},
		{"bare trigger", "@", "", 1, true},
		{"space ends token", "@al done", "", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			query, width, ok := triggerToken(tc.before, '@', nil)
			if query != tc.query || width != tc.width || ok != tc.ok {
				t.Fatalf("triggerToken(%q) = (%q, %d, %v), want (%q, %d, %v)",
					tc.before, query, width, ok, tc.query, tc.width, tc.ok)
			}
		})
	}
}

// TestBareTokenCompletion pins bare mode (no trigger rune) with a custom
// boundary: each comma-separated entry completes alone and acceptance swaps
// the token without a prefix.
func TestBareTokenCompletion(t *testing.T) {
	m := New(Config{
		Fields: []FieldSpec{{
			Autocomplete: &Autocomplete{
				IsBoundary: func(r rune) bool { return r == ',' || r == ' ' },
				Fetch: func(query string) []Suggestion {
					var out []Suggestion
					for _, name := range []string{"issues", "epics", "search"} {
						if strings.HasPrefix(name, query) {
							out = append(out, sug(name))
						}
					}
					return out
				},
			},
		}},
		Width: 40,
	})
	typeAndFetch(t, &m, "issues, ep")
	if !m.ac.visible() {
		t.Fatal("bare token after a comma did not arm suggestions")
	}
	press(t, &m, enter)
	if got := m.Value(0); got != "issues, epics" {
		t.Fatalf("bare acceptance produced %q, want the token swapped in place", got)
	}
}

func TestSuggestionsFetchRenderAccept(t *testing.T) {
	m := mentionForm(nil)
	typeAndFetch(t, &m, "ping ")
	typeAndFetch(t, &m, "@a")
	view := m.View()
	if !strings.Contains(view, "alice") || !strings.Contains(view, "alan") {
		t.Fatalf("suggestions not rendered: %q", view)
	}
	// Move to the second suggestion and accept it.
	press(t, &m, keyDn)
	if ev := press(t, &m, enter); ev != EventNone {
		t.Fatalf("accepting a suggestion emitted %v", ev)
	}
	if got := m.Value(0); got != "ping @alan" {
		t.Fatalf("acceptance produced %q", got)
	}
	if m.ac.visible() {
		t.Fatal("list still up after acceptance")
	}
}

// TestSuggestionValueDiffersFromLabel pins the {Value,Label} split: the list
// renders the human-readable Label while acceptance inserts the opaque Value.
func TestSuggestionValueDiffersFromLabel(t *testing.T) {
	m := New(Config{
		Fields: []FieldSpec{{
			Autocomplete: &Autocomplete{
				Trigger: '@',
				Fetch: func(string) []Suggestion {
					return []Suggestion{{Value: "acc-123", Label: "Ann Example"}}
				},
			},
		}},
		Width: 40,
	})
	typeAndFetch(t, &m, "@a")
	if v := m.View(); !strings.Contains(v, "Ann Example") || strings.Contains(v, "acc-123") {
		t.Fatalf("list must show the label, not the value: %q", v)
	}
	press(t, &m, enter)
	if got := m.Value(0); got != "@acc-123" {
		t.Fatalf("acceptance inserted %q, want the value with the trigger", got)
	}
}

func TestEnterAcceptsInsteadOfNewline(t *testing.T) {
	m := mentionForm(nil)
	typeAndFetch(t, &m, "@b")
	press(t, &m, enter)
	if got := m.Value(0); strings.Contains(got, "\n") {
		t.Fatalf("enter inserted a newline while suggesting: %q", got)
	}
}

func TestEscDismissesListWithoutGuard(t *testing.T) {
	m := mentionForm(nil)
	typeAndFetch(t, &m, "@a")
	if ev := press(t, &m, esc); ev != EventNone {
		t.Fatalf("esc with list up emitted %v", ev)
	}
	if m.ac.visible() {
		t.Fatal("list survived esc")
	}
	// The next esc reaches the form proper and asks about the dirty draft.
	press(t, &m, esc)
	if !strings.Contains(m.View(), "discard input?") {
		t.Fatal("guard skipped after list dismissal")
	}
}

func TestStaleSuggestionsDrop(t *testing.T) {
	m := mentionForm(nil)
	typeAndFetch(t, &m, "@a")
	// A slow response for an older query arrives after the token moved on.
	m.Update(SuggestionsMsg{Field: 0, Query: "zz", Items: []Suggestion{sug("zoe")}})
	if strings.Contains(m.View(), "zoe") {
		t.Fatal("stale suggestions installed")
	}
}

func TestQueryChangeRefetches(t *testing.T) {
	var queries []string
	m := mentionForm(&queries)
	typeAndFetch(t, &m, "@a")
	typeAndFetch(t, &m, "l")
	if len(queries) != 2 || queries[0] != "a" || queries[1] != "al" {
		t.Fatalf("fetch queries = %v", queries)
	}
	if got := m.View(); strings.Contains(got, "bob") {
		t.Fatalf("narrowed query still shows old items: %q", got)
	}
}

func TestNoFetchBelowMinQuery(t *testing.T) {
	var queries []string
	m := New(Config{
		Fields: []FieldSpec{{
			Autocomplete: &Autocomplete{
				Trigger:  '@',
				MinQuery: 2,
				Fetch: func(q string) []Suggestion {
					queries = append(queries, q)
					return nil
				},
			},
		}},
		Width: 40,
	})
	typeAndFetch(t, &m, "@a")
	if len(queries) != 0 {
		t.Fatalf("fetched below MinQuery: %v", queries)
	}
	typeAndFetch(t, &m, "l")
	if len(queries) != 1 || queries[0] != "al" {
		t.Fatalf("fetch at MinQuery: %v", queries)
	}
}

func TestSuggestionListCapped(t *testing.T) {
	m := New(Config{
		Fields: []FieldSpec{{
			Autocomplete: &Autocomplete{
				Trigger: '@',
				Fetch: func(string) []Suggestion {
					return []Suggestion{sug("a1"), sug("a2"), sug("a3"), sug("a4"), sug("a5"), sug("a6"), sug("a7")}
				},
			},
		}},
		Width: 40,
	})
	typeAndFetch(t, &m, "@x")
	if n := len(m.ac.items); n != maxSuggestions {
		t.Fatalf("list not capped: %d items", n)
	}
}

func TestLineFieldMidTextAcceptance(t *testing.T) {
	m := New(Config{
		Fields: []FieldSpec{{
			Autocomplete: &Autocomplete{
				Trigger: '@',
				Fetch:   func(string) []Suggestion { return []Suggestion{sug("alice")} },
			},
		}},
		Width: 40,
	})
	typeAndFetch(t, &m, "@a tail")
	// Walk the cursor back to sit right after "@a" so the token re-arms
	// mid-text; the last arrow's sync must fire the fetch again.
	for range len(" tail") {
		cmd, _, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
		drain(&m, cmd)
	}
	if !m.ac.visible() {
		t.Fatal("mid-text token did not re-arm suggestions")
	}
	press(t, &m, enter)
	if got := m.Value(0); got != "@alice tail" {
		t.Fatalf("mid-text acceptance produced %q, want the suffix preserved", got)
	}
	// The cursor sits at the end of the completion, not the end of the line.
	if _, _, ok := triggerToken(m.fields[0].beforeCursor(), '@', nil); !ok {
		t.Fatalf("cursor not after the completed token: before=%q", m.fields[0].beforeCursor())
	}
}

func TestPasteDuringConfirmIsSwallowed(t *testing.T) {
	m := New(Config{Fields: []FieldSpec{{}}, Width: 40})
	typeAndFetch(t, &m, "draft")
	press(t, &m, esc) // dirty → confirming
	if _, ev, consumed := m.Update(tea.PasteMsg{Content: "clipboard noise"}); ev != EventNone || !consumed {
		t.Fatalf("paste during confirm: ev=%v consumed=%v", ev, consumed)
	}
	if got := m.Value(0); got != "draft" {
		t.Fatalf("paste mutated the guarded draft: %q", got)
	}
}

func TestFocusChangeClearsSuggestions(t *testing.T) {
	m := New(Config{
		Fields: []FieldSpec{
			{Autocomplete: &Autocomplete{Trigger: '@', Fetch: func(string) []Suggestion { return []Suggestion{sug("alice")} }}},
			{Optional: true},
		},
		Width: 40,
	})
	typeAndFetch(t, &m, "@a")
	press(t, &m, tab)
	if m.ac.visible() {
		t.Fatal("suggestions leaked across a focus change")
	}
}
