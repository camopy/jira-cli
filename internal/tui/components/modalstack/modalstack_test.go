package modalstack

import "testing"

// layer returns a constant-content Layer for assertions.
func layer(s string) Layer {
	return func(int, int) string { return s }
}

func TestStackIsLIFO(t *testing.T) {
	var m Model
	m.Push(layer("first"))
	m.Push(layer("second"))

	if m.Len() != 2 || !m.Active() {
		t.Fatalf("len=%d active=%v, want 2/true", m.Len(), m.Active())
	}
	if top, _ := m.Top(); top(0, 0) != "second" {
		t.Errorf("top = %q, want second", top(0, 0))
	}
	if got, ok := m.Pop(); !ok || got(0, 0) != "second" {
		t.Errorf("pop = %v, want second,true", ok)
	}
	if got, ok := m.Pop(); !ok || got(0, 0) != "first" {
		t.Errorf("pop = %v, want first,true", ok)
	}
	if _, ok := m.Pop(); ok {
		t.Error("pop on empty stack returned ok=true")
	}
}

func TestRenderWithoutLayersReturnsBackground(t *testing.T) {
	var m Model
	bg := "background content"
	if got := m.Render(bg, 40, 10); got != bg {
		t.Errorf("Render with no layers = %q, want background unchanged", got)
	}
}

func TestRenderWithLayerOverlaysBackground(t *testing.T) {
	var m Model
	m.Push(layer("MODAL"))
	bg := "....................\n....................\n...................."
	got := m.Render(bg, 20, 3)
	if got == bg {
		t.Error("Render with a layer should differ from the background")
	}
}

func TestClearRemovesAll(t *testing.T) {
	var m Model
	m.Push(layer("a"))
	m.Push(layer("b"))
	m.Clear()
	if m.Active() || m.Len() != 0 {
		t.Errorf("after Clear active=%v len=%d, want false/0", m.Active(), m.Len())
	}
}
