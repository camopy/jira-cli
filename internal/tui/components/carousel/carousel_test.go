package carousel

import "testing"

func TestNextWrapsAroundEnd(t *testing.T) {
	m := New("a", "b", "c")
	m.Next()
	if m.Active() != 1 {
		t.Fatalf("after Next active = %d, want 1", m.Active())
	}
	m.Next()
	m.Next() // from index 2 wraps to 0
	if m.Active() != 0 {
		t.Errorf("Next did not wrap: active = %d, want 0", m.Active())
	}
}

func TestPrevWrapsAroundStart(t *testing.T) {
	m := New("a", "b", "c")
	m.Prev() // from 0 wraps to last
	if m.Active() != 2 {
		t.Errorf("Prev did not wrap: active = %d, want 2", m.Active())
	}
}

func TestEmptyCarousel(t *testing.T) {
	m := New()
	if m.Active() != -1 {
		t.Errorf("empty active = %d, want -1", m.Active())
	}
	if m.ActiveItem() != "" {
		t.Errorf("empty active item = %q, want empty", m.ActiveItem())
	}
	m.Next() // must not panic
	m.Prev()
	if m.View() != "" {
		t.Errorf("empty View = %q, want empty", m.View())
	}
}

func TestSetItemsClampsActive(t *testing.T) {
	m := New("a", "b", "c", "d")
	m.SetActive(3)
	m.SetItems([]string{"x", "y"})
	if m.Active() != 1 {
		t.Errorf("active after shrink = %d, want 1", m.Active())
	}
	if m.ActiveItem() != "y" {
		t.Errorf("active item = %q, want y", m.ActiveItem())
	}
}
