package cache

import (
	"fmt"
	"slices"
	"testing"
)

func TestRecordIssueKeysMostRecentFirstDedupedCapped(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	profile := "test-issuekeys"

	if err := RecordIssueKeys(profile, []string{"proj-1", " PROJ-2 ", "PROJ-1", ""}); err != nil {
		t.Fatalf("RecordIssueKeys() error = %v", err)
	}
	if got, want := IssueKeys(profile), []string{"PROJ-1", "PROJ-2"}; !slices.Equal(got, want) {
		t.Fatalf("first record = %v, want %v (normalized, deduped, order kept)", got, want)
	}

	if err := RecordIssueKeys(profile, []string{"PROJ-3", "PROJ-2"}); err != nil {
		t.Fatalf("RecordIssueKeys(merge) error = %v", err)
	}
	if got, want := IssueKeys(profile), []string{"PROJ-3", "PROJ-2", "PROJ-1"}; !slices.Equal(got, want) {
		t.Fatalf("merge = %v, want %v (incoming first, survivor order kept)", got, want)
	}

	bulk := make([]string, 0, issueKeysCap+10)
	for i := range issueKeysCap + 10 {
		bulk = append(bulk, fmt.Sprintf("BULK-%d", i))
	}
	if err := RecordIssueKeys(profile, bulk); err != nil {
		t.Fatalf("RecordIssueKeys(bulk) error = %v", err)
	}
	got := IssueKeys(profile)
	if len(got) != issueKeysCap {
		t.Fatalf("cap not applied: got %d keys, want %d", len(got), issueKeysCap)
	}
	if got[0] != "BULK-0" {
		t.Fatalf("newest-first violated after bulk record: first = %s", got[0])
	}
}

func TestIssueKeysMissingOrBrokenCacheIsNil(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if keys := IssueKeys("never-recorded"); keys != nil {
		t.Fatalf("missing cache returned %v, want nil", keys)
	}
}
