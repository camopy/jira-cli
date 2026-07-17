package issues

import (
	"context"
	"sync"
	"testing"

	"github.com/matcra587/jira-cli/internal/jira"
)

// countingUserSvc records every AssignableSearch query so a test can prove the
// assignee source prefetches once and then filters locally. The empty query
// returns page; a named query returns byQuery[query].
type countingUserSvc struct {
	jira.UserService
	mu      sync.Mutex
	calls   []string
	page    []*jira.User
	byQuery map[string][]*jira.User
}

func (f *countingUserSvc) AssignableSearch(_ context.Context, query, _ string) ([]*jira.User, *jira.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, query)
	if query == "" {
		return f.page, nil, nil
	}
	return f.byQuery[query], nil, nil
}

func (f *countingUserSvc) queries() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func mkUser(name, id string) *jira.User {
	n, i := name, id
	return &jira.User{DisplayName: &n, AccountID: &i}
}

func TestAssigneePrefetchFiltersLocallyThenFallsBack(t *testing.T) {
	svc := &countingUserSvc{
		page:    []*jira.User{mkUser("Alice Ng", "acc-alice"), mkUser("Bob Roy", "acc-bob")},
		byQuery: map[string][]*jira.User{"zed": {mkUser("Zed Faraway", "acc-zed")}},
	}
	m := newVerbModel(t, fakeServices{issue: fakeIssueSvc{}, user: svc})
	m.ensureCreateCaches()
	fetch := m.assigneeFetch("JCT")

	// A query the prefetched page satisfies filters locally.
	if got := fetch("ali"); len(got) != 1 || got[0].Detail != "acc-alice" {
		t.Fatalf("local match = %+v, want just Alice", got)
	}
	// It cost exactly one server hit — the empty-query prefetch, not a search for
	// "ali".
	if qs := svc.queries(); len(qs) != 1 || qs[0] != "" {
		t.Fatalf("server calls = %v, want a single empty-query prefetch", qs)
	}
	// A second page-satisfiable query reuses the cache — no new server hit.
	fetch("bob")
	if qs := svc.queries(); len(qs) != 1 {
		t.Fatalf("a cached-page match must not hit the server again: %v", qs)
	}
	// A query the page cannot satisfy falls back to a server search.
	if got := fetch("zed"); len(got) != 1 || got[0].Detail != "acc-zed" {
		t.Fatalf("fallback = %+v, want Zed from the server", got)
	}
	if qs := svc.queries(); len(qs) != 2 || qs[1] != "zed" {
		t.Fatalf("server calls = %v, want the empty prefetch then a 'zed' fallback", qs)
	}
}
