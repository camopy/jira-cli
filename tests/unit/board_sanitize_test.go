// SanitizeBoardsForCache enforces the data-model.md > Board > Validation
// invariants on records returned from /rest/agile/1.0/board: id must be
// > 0 and name must be non-empty after trim. Anything else gets dropped
// so the cache file never carries records that downstream consumers
// (resolver, plain-table renderer, JSON envelope) can't reason about.
package unit

import (
	"testing"

	"github.com/matcra587/jira-cli/pkg/jira"
)

func TestSanitizeBoardsForCacheDropsBadRecordsAndKeepsValid(t *testing.T) {
	t.Parallel()

	id := func(v int) *int { return &v }
	name := func(v string) *string { return &v }

	valid := jira.Board{ID: id(1), Name: name("OK")}
	nilID := jira.Board{Name: name("Nil ID")}
	zeroID := jira.Board{ID: id(0), Name: name("Zero ID")}
	negID := jira.Board{ID: id(-5), Name: name("Negative ID")}
	nilName := jira.Board{ID: id(2)}
	emptyName := jira.Board{ID: id(3), Name: name("")}
	whitespaceName := jira.Board{ID: id(4), Name: name("   \t\n")}
	anotherValid := jira.Board{ID: id(99), Name: name("Also OK")}

	in := []jira.Board{valid, nilID, zeroID, negID, nilName, emptyName, whitespaceName, anotherValid}
	kept, dropped := jira.SanitizeBoardsForCache(in)

	if dropped != 6 {
		t.Errorf("dropped=%d; want 6 (nil ID, zero ID, neg ID, nil Name, empty Name, whitespace Name)", dropped)
	}
	if len(kept) != 2 {
		t.Fatalf("kept len=%d; want 2 (valid, anotherValid)\nkept=%+v", len(kept), kept)
	}
	if kept[0].ID == nil || *kept[0].ID != 1 {
		t.Errorf("kept[0].ID = %v; want 1", kept[0].ID)
	}
	if kept[1].ID == nil || *kept[1].ID != 99 {
		t.Errorf("kept[1].ID = %v; want 99", kept[1].ID)
	}
}

func TestSanitizeBoardsForCachePreservesOrderAndProjectKeys(t *testing.T) {
	t.Parallel()

	id := func(v int) *int { return &v }
	name := func(v string) *string { return &v }

	in := []jira.Board{
		{ID: id(3), Name: name("Three"), ProjectKeys: []string{"C"}},
		{ID: id(1), Name: name("One"), ProjectKeys: []string{"A"}},
		{ID: id(2), Name: name("Two"), ProjectKeys: []string{"B"}},
	}
	kept, dropped := jira.SanitizeBoardsForCache(in)
	if dropped != 0 {
		t.Errorf("dropped=%d; want 0", dropped)
	}
	if len(kept) != 3 {
		t.Fatalf("kept len=%d; want 3", len(kept))
	}
	// Order preserved — sanitize is filter-only, never reorders.
	wantIDs := []int{3, 1, 2}
	for i, want := range wantIDs {
		if kept[i].ID == nil || *kept[i].ID != want {
			t.Errorf("kept[%d].ID = %v; want %d", i, kept[i].ID, want)
		}
	}
	// Project keys round-trip untouched (sort happens at the wire level
	// in ProjectsForBoard, not here).
	if got := kept[0].ProjectKeys; len(got) != 1 || got[0] != "C" {
		t.Errorf("ProjectKeys mangled: %v", got)
	}
}
