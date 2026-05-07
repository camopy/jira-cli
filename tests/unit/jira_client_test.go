package unit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/matcra587/jira-cli/pkg/jira"
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
