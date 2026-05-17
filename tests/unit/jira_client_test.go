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

func TestClientInjectsAuthBaseURLAndRateMetadata(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		w.Header().Set("X-RateLimit-Remaining", "4")
		w.Header().Set("Retry-After", "2")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := jira.NewClient(jira.WithBaseURL(srv.URL+"/"), jira.WithBearerToken("abc"))
	req, err := c.NewRequest(context.Background(), http.MethodGet, "rest/api/3/myself", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	var out map[string]bool
	resp, err := c.Do(req, &out)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if auth != "Bearer abc" {
		t.Fatalf("Authorization = %q", auth)
	}
	if !out["ok"] {
		t.Fatalf("decoded body = %+v", out)
	}
	if resp.Rate.Remaining != 4 || resp.Rate.RetryAfterSeconds != 2 {
		t.Fatalf("rate metadata = %+v", resp.Rate)
	}
}

func TestClientReturnsTypedAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"errorMessages":["bad auth"]}`))
	}))
	defer srv.Close()

	c := jira.NewClient(jira.WithBaseURL(srv.URL + "/"))
	req, err := c.NewRequest(context.Background(), http.MethodGet, "bad", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	_, err = c.Do(req, nil)
	var apiErr *jira.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %T %v, want *jira.APIError", err, err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized || apiErr.Type != jira.ErrorTypeAuth {
		t.Fatalf("api error = %+v", apiErr)
	}
}

func TestNewClientERejectsInvalidOptions(t *testing.T) {
	tests := []struct {
		name string
		opt  jira.Option
	}{
		{name: "nil http client", opt: jira.WithHTTPClient(nil)},
		{name: "malformed base URL", opt: jira.WithBaseURL("://bad")},
		{name: "relative base URL", opt: jira.WithBaseURL("/jira")},
		{name: "unsupported scheme", opt: jira.WithBaseURL("ftp://jira.example.com")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := jira.NewClientE(tt.opt); err == nil {
				t.Fatal("NewClientE() error = nil, want validation error")
			}
		})
	}
}

func TestNewClientRetainsInvalidOption(t *testing.T) {
	client := jira.NewClient(jira.WithBaseURL("://bad"))
	if _, err := client.NewRequest(context.Background(), http.MethodGet, "rest/api/3/myself", nil); err == nil {
		t.Fatal("NewRequest() succeeded with invalid base URL option")
	}
}

func TestNewRequestRejectsUnsafeTargets(t *testing.T) {
	client := jira.NewClient(jira.WithBaseURL("https://jira.example.com/"))
	tests := []string{
		"https://evil.example/rest/api/3/myself",
		"//evil.example/rest/api/3/myself",
		"rest/api/3/issue/../myself",
		"rest/api/3/myself#fragment",
		"rest/api/3/myself\x01",
	}
	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			if _, err := client.NewRequest(context.Background(), http.MethodGet, path, nil); err == nil {
				t.Fatal("NewRequest() error = nil, want unsafe target rejection")
			}
		})
	}
}

func TestReadOnlyOnlyAllowsExactSearchEndpoint(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.Method != http.MethodPost || r.URL.Path != "/rest/api/3/search/jql" {
			t.Fatalf("unexpected request reached server: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"issues":[],"isLast":true}`))
	}))
	defer srv.Close()

	client := jira.NewClient(jira.WithBaseURL(srv.URL), jira.WithReadOnly(true))
	allowed, err := client.NewRequest(context.Background(), http.MethodPost, "rest/api/3/search/jql", map[string]any{"jql": "project=KAN"})
	if err != nil {
		t.Fatalf("NewRequest(allowed) error = %v", err)
	}
	if _, err := client.Do(allowed, nil); err != nil {
		t.Fatalf("Do(allowed search) error = %v", err)
	}

	refused, err := client.NewRequest(context.Background(), http.MethodPost, "rest/api/3/issue/ABC/search/jql", map[string]any{})
	if err != nil {
		t.Fatalf("NewRequest(refused) error = %v", err)
	}
	if _, err := client.Do(refused, nil); err == nil || !strings.Contains(strings.ToLower(err.Error()), "read-only") {
		t.Fatalf("Do(non-allowlisted search-ish path) error = %v, want read-only refusal", err)
	}
	encodedSlash, err := client.NewRequest(context.Background(), http.MethodPost, "rest/api/3/search%2Fjql", map[string]any{})
	if err != nil {
		t.Fatalf("NewRequest(encoded slash) error = %v", err)
	}
	if _, err := client.Do(encodedSlash, nil); err == nil || !strings.Contains(strings.ToLower(err.Error()), "read-only") {
		t.Fatalf("Do(encoded-slash path) error = %v, want read-only refusal", err)
	}
	if hits != 1 {
		t.Fatalf("server hits = %d, want only exact search endpoint to pass", hits)
	}
}

func TestReadOnlySearchAllowlistRespectsBasePath(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path != "/jira/rest/api/3/search/jql" {
			t.Fatalf("unexpected path reached server: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"issues":[],"isLast":true}`))
	}))
	defer srv.Close()

	client := jira.NewClient(jira.WithBaseURL(srv.URL+"/jira/"), jira.WithReadOnly(true))
	allowed, err := client.NewRequest(context.Background(), http.MethodPost, "rest/api/3/search/jql", map[string]any{"jql": "project=KAN"})
	if err != nil {
		t.Fatalf("NewRequest(allowed) error = %v", err)
	}
	if _, err := client.Do(allowed, nil); err != nil {
		t.Fatalf("Do(allowed base-path search) error = %v", err)
	}

	prefixed, err := client.NewRequest(context.Background(), http.MethodPost, "anything/rest/api/3/search/jql", map[string]any{})
	if err != nil {
		t.Fatalf("NewRequest(prefixed) error = %v", err)
	}
	if _, err := client.Do(prefixed, nil); err == nil || !strings.Contains(strings.ToLower(err.Error()), "read-only") {
		t.Fatalf("Do(prefixed search path) error = %v, want read-only refusal", err)
	}
	if hits != 1 {
		t.Fatalf("server hits = %d, want only configured base-path search to pass", hits)
	}
}
