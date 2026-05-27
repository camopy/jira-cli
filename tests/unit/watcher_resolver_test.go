package unit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/jira"
)

// TestResolveUserMeUsesCachedMyself asserts that --user me does NOT issue a
// /user/search request — it returns the authenticated user's accountId via
// /myself per .
func TestResolveUserMeUsesCachedMyself(t *testing.T) {
	var sawSearch bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/rest/api/3/myself":
			_, _ = w.Write([]byte(`{"accountId":"712020:test-user","emailAddress":"user@example.com","displayName":"Test User","active":true}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/rest/api/3/user/search"):
			sawSearch = true
			_, _ = w.Write([]byte(`[]`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := jira.NewClient(jira.WithBaseURL(srv.URL + "/"))
	user := jira.NewUserService(client)
	id, err := user.ResolveUser(context.Background(), "me")
	if err != nil {
		t.Fatalf("ResolveUser(me) error = %v", err)
	}
	if id != "712020:test-user" {
		t.Fatalf("ResolveUser(me) = %q, want %q", id, "712020:test-user")
	}
	if sawSearch {
		t.Fatal("ResolveUser(me) issued /user/search; expected /myself only")
	}
}

// TestResolveUserAccountIDPrefixSkipsSearch asserts the accountId:<id> prefix
// is parsed locally and bypasses /user/search per .
func TestResolveUserAccountIDPrefixSkipsSearch(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := jira.NewClient(jira.WithBaseURL(srv.URL + "/"))
	user := jira.NewUserService(client)
	id, err := user.ResolveUser(context.Background(), "accountId:712020:abc")
	if err != nil {
		t.Fatalf("ResolveUser error = %v", err)
	}
	if id != "712020:abc" {
		t.Fatalf("ResolveUser = %q, want %q", id, "712020:abc")
	}
	if hits != 0 {
		t.Fatalf("accountId: prefix issued %d HTTP requests, want 0", hits)
	}
}

// TestResolveUserSingleMatchReturnsAccountID covers the happy path: /user/search
// returns exactly one User → return its accountId.
func TestResolveUserSingleMatchReturnsAccountID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/rest/api/3/user/search") {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("query"); got != "alice@example.com" {
			t.Errorf("query = %q, want alice@example.com", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"accountId":"acc-alice","displayName":"Alice","emailAddress":"alice@example.com","active":true}]`))
	}))
	defer srv.Close()

	client := jira.NewClient(jira.WithBaseURL(srv.URL + "/"))
	user := jira.NewUserService(client)
	id, err := user.ResolveUser(context.Background(), "alice@example.com")
	if err != nil {
		t.Fatalf("ResolveUser error = %v", err)
	}
	if id != "acc-alice" {
		t.Fatalf("ResolveUser = %q, want acc-alice", id)
	}
}

func TestResolveUserIgnoresInactiveMatches(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"accountId":"inactive","displayName":"Inactive","emailAddress":"old@example.com","active":false},
			{"accountId":"active","displayName":"Active","emailAddress":"new@example.com","active":true}
		]`))
	}))
	defer srv.Close()

	user := jira.NewUserService(jira.NewClient(jira.WithBaseURL(srv.URL + "/")))
	id, err := user.ResolveUser(context.Background(), "person@example.com")
	if err != nil {
		t.Fatalf("ResolveUser error = %v", err)
	}
	if id != "active" {
		t.Fatalf("ResolveUser = %q, want active", id)
	}
}

func TestResolveUserInactiveOnlyReturnsNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"accountId":"inactive","displayName":"Inactive","active":false}]`))
	}))
	defer srv.Close()

	user := jira.NewUserService(jira.NewClient(jira.WithBaseURL(srv.URL + "/")))
	_, err := user.ResolveUser(context.Background(), "inactive@example.com")
	if !errors.Is(err, jira.ErrUserNotFound) {
		t.Fatalf("ResolveUser err = %v, want ErrUserNotFound", err)
	}
}

func TestResolveUserAmbiguousCandidatesExcludeInactive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"accountId":"inactive","displayName":"Inactive","active":false},
			{"accountId":"a-1","displayName":"Alice Smith","active":true},
			{"accountId":"a-2","displayName":"Alice Jones","active":true}
		]`))
	}))
	defer srv.Close()

	user := jira.NewUserService(jira.NewClient(jira.WithBaseURL(srv.URL + "/")))
	_, err := user.ResolveUser(context.Background(), "alice")
	var ambig *jira.AmbiguousUserError
	if !errors.As(err, &ambig) {
		t.Fatalf("ResolveUser err = %v (%T), want *AmbiguousUserError", err, err)
	}
	if len(ambig.Candidates) != 2 {
		t.Fatalf("Candidates = %d, want 2 active candidates", len(ambig.Candidates))
	}
	for _, c := range ambig.Candidates {
		if c.AccountID != nil && *c.AccountID == "inactive" {
			t.Fatalf("inactive user leaked into candidates: %+v", ambig.Candidates)
		}
	}
}

// TestResolveUserZeroMatchesReturnsErrUserNotFound covers the not-found path
// (exit 2): /user/search returns []. ErrUserNotFound is the sentinel.
func TestResolveUserZeroMatchesReturnsErrUserNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	client := jira.NewClient(jira.WithBaseURL(srv.URL + "/"))
	user := jira.NewUserService(client)
	_, err := user.ResolveUser(context.Background(), "ghost@example.com")
	if !errors.Is(err, jira.ErrUserNotFound) {
		t.Fatalf("ResolveUser err = %v, want ErrUserNotFound", err)
	}
}

// TestResolveUserMultiMatchReturnsAmbiguousError covers the ambiguity path
// (exit 3): /user/search returns 2+ results → AmbiguousUserError carrying
// every candidate so callers can render the disambiguation envelope.
func TestResolveUserMultiMatchReturnsAmbiguousError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"accountId":"a-1","displayName":"Alice Smith","emailAddress":"alice.smith@example.com","active":true},
			{"accountId":"a-2","displayName":"Alice Jones","emailAddress":"alice.jones@example.com","active":true},
			{"accountId":"a-3","displayName":"Alice Brown","emailAddress":"","active":true}
		]`))
	}))
	defer srv.Close()

	client := jira.NewClient(jira.WithBaseURL(srv.URL + "/"))
	user := jira.NewUserService(client)
	_, err := user.ResolveUser(context.Background(), "alice")
	var ambig *jira.AmbiguousUserError
	if !errors.As(err, &ambig) {
		t.Fatalf("ResolveUser err = %v (%T), want *AmbiguousUserError", err, err)
	}
	if len(ambig.Candidates) != 3 {
		t.Fatalf("Candidates = %d, want 3", len(ambig.Candidates))
	}
	wantIDs := []string{"a-1", "a-2", "a-3"}
	for i, want := range wantIDs {
		got := ""
		if id := ambig.Candidates[i].AccountID; id != nil {
			got = *id
		}
		if got != want {
			t.Errorf("Candidates[%d].AccountID = %q, want %q", i, got, want)
		}
	}
}

// TestUserSearchReturnsCandidates covers the bare UserService.Search method
// — the resolver builds on top of this; tests assert it walks /user/search
// and decodes the User slice.
func TestUserSearchReturnsCandidates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/rest/api/3/user/search") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("query"); got != "matt" {
			t.Errorf("query = %q, want matt", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"accountId":"m1","displayName":"Test","active":true}]`))
	}))
	defer srv.Close()

	client := jira.NewClient(jira.WithBaseURL(srv.URL + "/"))
	user := jira.NewUserService(client)
	users, _, err := user.Search(context.Background(), "matt")
	if err != nil {
		t.Fatalf("Search error = %v", err)
	}
	if len(users) != 1 || users[0].AccountID == nil || *users[0].AccountID != "m1" {
		t.Fatalf("Search() = %+v, want one user with accountId m1", users)
	}
}
