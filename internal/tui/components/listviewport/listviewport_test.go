package listviewport

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func rows(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("row-%d", i)
	}
	return out
}

// invariant asserts the core property: the cursor is always within the visible
// window and the window never scrolls past the end.
func invariant(t *testing.T, m *Model) {
	t.Helper()
	if len(m.rows) == 0 {
		return
	}
	start, end := m.VisibleRange()
	if m.cursor < start || m.cursor >= end {
		t.Fatalf("cursor %d not visible in window [%d,%d)", m.cursor, start, end)
	}
	if m.offset < 0 {
		t.Fatalf("negative offset %d", m.offset)
	}
	if maxOff := len(m.rows) - m.height; m.height > 0 && maxOff >= 0 && m.offset > maxOff {
		t.Fatalf("offset %d past max %d", m.offset, maxOff)
	}
}

func TestEmptyListCursorIsNegative(t *testing.T) {
	m := New()
	m.SetSize(10, 5)
	if m.Cursor() != -1 {
		t.Errorf("empty list cursor = %d, want -1", m.Cursor())
	}
	if m.View() != "" {
		t.Errorf("empty list View should be empty, got %q", m.View())
	}
}

func TestCursorMovesWindowToStayVisible(t *testing.T) {
	m := New()
	m.SetSize(20, 5)
	m.SetRows(rows(100))

	m.Bottom()
	if m.Cursor() != 99 {
		t.Fatalf("cursor = %d, want 99", m.Cursor())
	}
	if start, end := m.VisibleRange(); start != 95 || end != 100 {
		t.Errorf("bottom window = [%d,%d), want [95,100)", start, end)
	}
	invariant(t, m)

	m.Top()
	if start, end := m.VisibleRange(); start != 0 || end != 5 {
		t.Errorf("top window = [%d,%d), want [0,5)", start, end)
	}
	invariant(t, m)
}

func TestScrollingHappensOnlyAtWindowEdge(t *testing.T) {
	m := New()
	m.SetSize(20, 5)
	m.SetRows(rows(100))

	// Moving within the window does not scroll.
	m.MoveDown(4) // cursor 4, still in [0,5)
	if start, _ := m.VisibleRange(); start != 0 {
		t.Errorf("offset moved early: start=%d, want 0", start)
	}
	// One more step pushes the window by one.
	m.MoveDown(1) // cursor 5 → window [1,6)
	if start, _ := m.VisibleRange(); start != 1 {
		t.Errorf("offset = %d, want 1 after crossing edge", start)
	}
	invariant(t, m)
}

func TestSetRowsShrinkClampsCursor(t *testing.T) {
	m := New()
	m.SetSize(20, 5)
	m.SetRows(rows(50))
	m.Bottom() // cursor 49
	m.SetRows(rows(3))
	if m.Cursor() != 2 {
		t.Errorf("cursor after shrink = %d, want 2", m.Cursor())
	}
	if m.offset != 0 {
		t.Errorf("offset after shrink = %d, want 0", m.offset)
	}
	invariant(t, m)
}

func TestShortListNeverScrolls(t *testing.T) {
	m := New()
	m.SetSize(20, 10)
	m.SetRows(rows(3))
	m.Bottom()
	if m.offset != 0 {
		t.Errorf("short list offset = %d, want 0", m.offset)
	}
	if start, end := m.VisibleRange(); start != 0 || end != 3 {
		t.Errorf("short list window = [%d,%d), want [0,3)", start, end)
	}
}

func TestMoveClampsAtBounds(t *testing.T) {
	m := New()
	m.SetSize(20, 5)
	m.SetRows(rows(10))
	m.MoveUp(100)
	if m.Cursor() != 0 {
		t.Errorf("cursor underflow = %d, want 0", m.Cursor())
	}
	m.MoveDown(100)
	if m.Cursor() != 9 {
		t.Errorf("cursor overflow = %d, want 9", m.Cursor())
	}
	invariant(t, m)
}

func TestPageMovesByHeightLessOne(t *testing.T) {
	m := New()
	m.SetSize(20, 5)
	m.SetRows(rows(100))
	m.PageDown()
	if m.Cursor() != 4 {
		t.Errorf("page down cursor = %d, want 4 (height-1)", m.Cursor())
	}
	invariant(t, m)
}

func TestScrollbarThumbTracksPosition(t *testing.T) {
	m := New()
	m.SetSize(20, 10)
	m.SetRows(rows(100))

	m.Top()
	col := m.scrollbarColumn()
	if col == nil {
		t.Fatal("expected a scrollbar for an overflowing list")
	}
	if col[0] != "█" {
		t.Errorf("thumb not at top when scrolled to top: %v", col)
	}

	m.Bottom()
	col = m.scrollbarColumn()
	if col[len(col)-1] != "█" {
		t.Errorf("thumb not at bottom when scrolled to bottom: %v", col)
	}
}

func TestRowStyleAppliesAfterPaddingAndSurvivesCursor(t *testing.T) {
	m := New()
	m.Scrollbar = false
	m.SetSize(10, 2)
	m.SetRows(rows(2))
	style := lipgloss.NewStyle().Underline(true)
	m.SetRowStyle(0, style)

	line := strings.Split(m.View(), "\n")[0]
	want := m.CursorStyle.Render(style.Render("row-0     "))
	if line != want {
		t.Fatalf("styled cursor row = %q, want %q", line, want)
	}

	m.SetRows(rows(2))
	if strings.Contains(m.View(), "\x1b[4m") {
		t.Error("SetRows did not clear stale row styles")
	}
}

func TestResizeKeepsSelection(t *testing.T) {
	m := New()
	m.SetSize(20, 10)
	m.SetRows(rows(100))
	m.SetCursor(50)
	m.SetSize(20, 4) // shrink viewport
	if m.Cursor() != 50 {
		t.Errorf("resize lost selection: cursor = %d, want 50", m.Cursor())
	}
	invariant(t, m)
}
