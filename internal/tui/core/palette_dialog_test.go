package core

import (
	"reflect"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/matcra587/jira-cli/internal/tui/components/dialog"
	"github.com/matcra587/jira-cli/internal/tui/keys"
)

// TestReplayKeyRoundTrips is the load-bearing guarantee behind the palette:
// every key string a binding can carry must rebuild into a KeyPressMsg whose
// String() is byte-for-byte that string, or key.Matches would never fire and a
// palette selection would silently do nothing. Iterating the whole default map
// keeps the namedKeys set honest — a new binding with an unlisted named key
// fails here rather than in the field.
func TestReplayKeyRoundTrips(t *testing.T) {
	assertRoundTrip := func(t *testing.T, label, ks string) {
		t.Helper()
		if got := replayKey(ks).String(); got != ks {
			t.Errorf("%s: replayKey(%q).String() = %q, want %q", label, ks, got, ks)
		}
		// The palette matches through key.Matches, so pin that path too.
		if !key.Matches(replayKey(ks), key.NewBinding(key.WithKeys(ks))) {
			t.Errorf("%s: replayKey(%q) does not match a binding on %q", label, ks, ks)
		}
	}

	m := keys.Default()
	v := reflect.ValueOf(m)
	for i := range v.NumField() {
		b, ok := v.Field(i).Interface().(key.Binding)
		if !ok {
			continue
		}
		name := v.Type().Field(i).Name
		for _, ks := range b.Keys() {
			assertRoundTrip(t, name, ks)
		}
	}

	// [tui.keys] rebinding is free-form, so the palette must replay keys the
	// default map never uses — modifier + named/function keys especially. A
	// naive first-rune fallback silently turned "ctrl+f5" into "ctrl+f".
	for _, ks := range []string{
		"f1", "f5", "f12", "f63",
		"ctrl+f5", "shift+f1", "alt+f12",
		"ctrl+home", "alt+pgdown", "shift+tab",
		"ctrl+insert", "alt+delete", "ctrl+end",
	} {
		assertRoundTrip(t, "rebind", ks)
	}

	// An unknown modified key must not collide with any other binding. It may
	// fail to match its own (unsupported) key, but it must never drop the
	// modifier: a bare "f99" or a truncated "ctrl+f" could fire a command bound
	// to that literal.
	for _, bad := range []string{"f99", "ctrl+f", "f"} {
		if got := replayKey("ctrl+f99").String(); got == bad {
			t.Errorf(`replayKey("ctrl+f99").String() = %q, collides with %q`, got, bad)
		}
	}
	// The modifier must survive into the replayed key's String().
	if got := replayKey("ctrl+f99").String(); !strings.HasPrefix(got, "ctrl+") {
		t.Errorf(`replayKey("ctrl+f99").String() = %q, want a ctrl+ prefix (modifier dropped)`, got)
	}
}

// TestPaletteScrollsAndPinsFooter pins the overflow fix: the palette dialog
// pins its accept/cancel row (dialog.Footered) and reports the selected row so
// the Shell scrolls it into view (dialog.ScrollHint), rather than letting a
// list taller than the box push the foot row and lower commands off-screen.
func TestPaletteScrollsAndPinsFooter(t *testing.T) {
	ctx := NewProgramContext(nil, nil)
	reg := NewRegistry()
	cs := &countingSection{id: "issues", bindings: []key.Binding{
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "transition")),
	}}
	reg.Register("issues", func(*ProgramContext) Section { return cs })
	app := NewApp(ctx, reg, []SectionID{"issues"})
	app.Init()

	d := app.newPaletteDialog()

	f, ok := any(d).(dialog.Footered)
	if !ok {
		t.Fatal("paletteDialog must implement dialog.Footered to pin its foot row")
	}
	if f.Footer() == "" {
		t.Error("Footer() is empty; the accept/cancel row would vanish when the body scrolls")
	}

	sh, ok := any(d).(dialog.ScrollHint)
	if !ok {
		t.Fatal("paletteDialog must implement dialog.ScrollHint so the viewport follows the cursor")
	}
	top, height, ok := sh.ScrollTo()
	if !ok || height != 1 {
		t.Fatalf("ScrollTo = (%d, %d, %v); want a single-row region", top, height, ok)
	}
	// The first entry sits below the title and query lines.
	if top != 2 {
		t.Errorf("ScrollTo top on the first row = %d, want 2", top)
	}
	// Moving the cursor down advances the reported region so it stays in view.
	d.palette.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if top, _, _ := sh.ScrollTo(); top != 3 {
		t.Errorf("ScrollTo top after one down = %d, want 3", top)
	}
}

// TestPalettePopThenReplay pins the pop-then-replay ordering: opening the
// palette, filtering to a section's contextual command, and accepting it pops
// the palette first, then replays the command's key to the underlying section —
// so the section acts on the key and the just-closed overlay cannot re-swallow
// it.
func TestPalettePopThenReplay(t *testing.T) {
	ctx := NewProgramContext(nil, nil)
	ctx.SetSize(100, 40)
	reg := NewRegistry()
	cs := &countingSection{id: "issues", bindings: []key.Binding{
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "transition")),
	}}
	reg.Register("issues", func(*ProgramContext) Section { return cs })

	app := NewApp(ctx, reg, []SectionID{"issues"})
	app.Init()

	// ctrl+k opens the palette.
	m, _ := app.Update(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'k'})
	app = m.(App)
	if !app.dialogs.Active() {
		t.Fatal("ctrl+k did not open the palette")
	}

	// Filter down to the "transition" command by typing it.
	for _, r := range "trans" {
		m, _ = app.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		app = m.(App)
	}

	// Accept: the palette pops and the chosen key replays to the section.
	m, _ = app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	app = m.(App)

	if app.dialogs.Active() {
		t.Error("accepting a palette entry must pop the dialog before the replay")
	}
	if cs.lastKey != "t" {
		t.Errorf("replayed key reached the section as %q, want t", cs.lastKey)
	}
}
