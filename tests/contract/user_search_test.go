package contract

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// `jira user search` is the deterministic path from a person's name or
// email to the accountId an ADF mention node requires. The envelope
// carries every active match so callers disambiguate on email or display
// name; inactive and deleted accounts never appear.
func TestUserSearchReturnsAccountIdentities(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/user/search", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("query"); got != "sam@example.com" {
			t.Errorf("query = %q, want the raw search term", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"accountId":"712020:aaaa","displayName":"Sam Active","emailAddress":"sam@example.com","active":true},
			{"accountId":"712020:bbbb","displayName":"Sam Inactive","emailAddress":"sam.old@example.com","active":false},
			{"accountId":"712020:cccc","displayName":"Sam Deleted","emailAddress":"","active":true,"deleted":true}
		]`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cfg := jiraConfig(t, srv.URL)
	stdout, stderr, code := runJira(t, "--config", cfg, "user", "search", "sam@example.com", "--output=json")
	if code != 0 {
		t.Fatalf("user search exit = %d\nstderr=%s", code, stderr)
	}
	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Query string `json:"query"`
			Count int    `json:"count"`
			Users []struct {
				AccountID    string `json:"account_id"`
				DisplayName  string `json:"display_name"`
				EmailAddress string `json:"email_address"`
			} `json:"users"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, stdout)
	}
	if !env.OK || env.Data.Count != 1 || len(env.Data.Users) != 1 {
		t.Fatalf("want exactly the active, non-deleted match: %s", stdout)
	}
	u := env.Data.Users[0]
	if u.AccountID != "712020:aaaa" || u.DisplayName != "Sam Active" || u.EmailAddress != "sam@example.com" {
		t.Fatalf("match fields wrong: %+v", u)
	}
}

// Zero matches is a successful empty list, not an error — agents branch on
// count without unwrapping a failure envelope.
func TestUserSearchZeroMatchesIsEmptyOK(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/user/search", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cfg := jiraConfig(t, srv.URL)
	stdout, stderr, code := runJira(t, "--config", cfg, "user", "search", "nobody", "--output=json")
	if code != 0 {
		t.Fatalf("zero matches must still exit 0, got %d\nstderr=%s", code, stderr)
	}
	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Count int   `json:"count"`
			Users []any `json:"users"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, stdout)
	}
	if !env.OK || env.Data.Count != 0 || env.Data.Users == nil {
		t.Fatalf("want ok envelope with empty (non-null) users array: %s", stdout)
	}
}
