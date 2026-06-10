package keys

import "testing"

func TestDefaultBindsCoreVerbs(t *testing.T) {
	m := Default()
	cases := map[string]string{
		"transition": "t",
		"comment":    "c",
		"assign":     "a",
		"quit":       "q",
	}
	for name, want := range cases {
		b, ok := m.index()[name]
		if !ok {
			t.Fatalf("binding %q missing from index", name)
		}
		if keys := b.Keys(); len(keys) == 0 || keys[0] != want {
			t.Errorf("binding %q first key = %v, want %q", name, keys, want)
		}
	}
}

func TestRebindOverridesKeysAndHelp(t *testing.T) {
	m := Default()
	if err := m.Rebind(map[string][]string{"transition": {"x", "X"}}); err != nil {
		t.Fatalf("Rebind returned error: %v", err)
	}
	if got := m.Transition.Keys(); len(got) != 2 || got[0] != "x" {
		t.Errorf("transition keys = %v, want [x X]", got)
	}
	if h := m.Transition.Help(); h.Key != "x" {
		t.Errorf("help key = %q, want x (first override key)", h.Key)
	}
}

func TestRebindUnknownNameIsError(t *testing.T) {
	m := Default()
	if err := m.Rebind(map[string][]string{"nope": {"z"}}); err == nil {
		t.Error("expected error for unknown binding name, got nil")
	}
}

func TestRebindEmptyKeysIgnored(t *testing.T) {
	m := Default()
	before := m.Comment.Keys()
	if err := m.Rebind(map[string][]string{"comment": {}}); err != nil {
		t.Fatalf("Rebind returned error: %v", err)
	}
	after := m.Comment.Keys()
	if len(before) != len(after) || before[0] != after[0] {
		t.Errorf("empty override changed binding: before=%v after=%v", before, after)
	}
}

func TestNamesAreSortedAndComplete(t *testing.T) {
	m := Default()
	names := m.Names()
	if len(names) != len(m.index()) {
		t.Errorf("Names returned %d entries, index has %d", len(names), len(m.index()))
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Errorf("Names not sorted at %d: %q > %q", i, names[i-1], names[i])
		}
	}
}
