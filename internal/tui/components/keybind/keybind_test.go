package keybind

import (
	"testing"

	"charm.land/bubbles/v2/key"
)

// ownerMap is a minimal owner key map: the registry never owns bindings, it
// only indexes the owner's fields — Rebind must mutate them in place.
type ownerMap struct {
	Open key.Binding
	Quit key.Binding
}

func newOwner() (*ownerMap, Registry) {
	o := &ownerMap{
		Open: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		Quit: key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
	return o, Registry{"open": &o.Open, "quit": &o.Quit}
}

func TestRebindOverridesKeysAndKeepsDescription(t *testing.T) {
	o, r := newOwner()
	if err := r.Rebind(map[string][]string{"open": {"o", "O"}}); err != nil {
		t.Fatalf("Rebind returned error: %v", err)
	}
	if got := o.Open.Keys(); len(got) != 2 || got[0] != "o" {
		t.Errorf("keys = %v, want [o O]", got)
	}
	if h := o.Open.Help(); h.Key != "o" || h.Desc != "open" {
		t.Errorf("help = %+v, want first key as label with description kept", h)
	}
}

func TestRebindEmptyOverrideIgnored(t *testing.T) {
	o, r := newOwner()
	if err := r.Rebind(map[string][]string{"quit": {}}); err != nil {
		t.Fatalf("Rebind returned error: %v", err)
	}
	if got := o.Quit.Keys(); len(got) != 2 || got[0] != "q" {
		t.Errorf("empty override changed binding: %v", got)
	}
}

func TestRebindUnknownNameIsError(t *testing.T) {
	_, r := newOwner()
	if err := r.Rebind(map[string][]string{"nope": {"z"}}); err == nil {
		t.Error("expected error for unknown binding name, got nil")
	}
}

func TestRebindUnknownNameMutatesNothing(t *testing.T) {
	o, r := newOwner()
	err := r.Rebind(map[string][]string{"open": {"o"}, "nope": {"z"}})
	if err == nil {
		t.Fatal("unknown name did not error")
	}
	// The valid override in the same call must not have landed: a failed
	// Rebind is atomic, whatever the map iteration order.
	if got := o.Open.Keys(); got[0] != "enter" {
		t.Errorf("failed Rebind partially applied: open keys = %v", got)
	}
}

func TestNamesSorted(t *testing.T) {
	_, r := newOwner()
	names := r.Names()
	if len(names) != 2 || names[0] != "open" || names[1] != "quit" {
		t.Errorf("Names = %v, want sorted [open quit]", names)
	}
}
