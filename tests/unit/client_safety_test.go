package unit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/pkg/jira"
)

func TestProfileValidationRejectsUnsafeBaseURLs(t *testing.T) {
	for _, baseURL := range []string{"http://company.atlassian.net", "://bad"} {
		cfg := config.Defaults()
		cfg.Profiles[0].BaseURL = baseURL
		if err := cfg.Validate(); err == nil {
			t.Fatalf("Validate() accepted unsafe base URL %q", baseURL)
		}
	}

	cfg := config.Defaults()
	cfg.Profiles[0].BaseURL = "http://127.0.0.1:2990"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() rejected local test URL: %v", err)
	}
}

func TestClientHTTPTimeoutOptionCancelsSlowRequests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client := jira.NewClient(jira.WithBaseURL(srv.URL), jira.WithHTTPTimeout(20*time.Millisecond))
	req, err := client.NewRequest(context.Background(), http.MethodGet, "/", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	_, err = client.Do(req, nil)
	if err == nil {
		t.Fatal("Do() succeeded despite timeout")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "Client.Timeout") {
		t.Fatalf("Do() error = %v, want timeout", err)
	}
}

func TestBaseURLReturnsCopy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client := jira.NewClient(jira.WithBaseURL(srv.URL))
	base := client.BaseURL()
	base.Host = "evil.example"
	base.Scheme = "https"

	req, err := client.NewRequest(context.Background(), http.MethodGet, "rest/api/3/myself", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	if req.URL.Host != strings.TrimPrefix(srv.URL, "http://") {
		t.Fatalf("request host = %q, want original server host", req.URL.Host)
	}
}

func TestNilClientAndNilRequestReturnErrors(t *testing.T) {
	var nilClient *jira.Client
	if _, err := nilClient.NewRequest(context.Background(), http.MethodGet, "rest/api/3/myself", nil); err == nil {
		t.Fatal("nil client NewRequest() error = nil")
	}

	var zero jira.Client
	if _, err := zero.NewRequest(context.Background(), http.MethodGet, "rest/api/3/myself", nil); err == nil {
		t.Fatal("zero-value client NewRequest() error = nil")
	}

	client := jira.NewClient(jira.WithBaseURL("https://jira.example.com/"))
	if _, err := client.Do(nil, nil); err == nil {
		t.Fatal("Do(nil) error = nil")
	}
	if _, err := client.Do(&http.Request{Method: http.MethodPost}, nil); err == nil {
		t.Fatal("Do(request with nil URL) error = nil")
	}
}

func TestClientDoRejectsMutatedRequestTarget(t *testing.T) {
	client := jira.NewClient(jira.WithBaseURL("https://jira.example.com/base/"))
	req, err := client.NewRequest(context.Background(), http.MethodGet, "rest/api/3/myself", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	req.URL.Host = "evil.example"
	if _, err := client.Do(req, nil); err == nil {
		t.Fatal("Do(mutated host) error = nil")
	}

	req, err = client.NewRequest(context.Background(), http.MethodGet, "rest/api/3/myself", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.URL.Path = "/other/rest/api/3/myself"
	if _, err := client.Do(req, nil); err == nil {
		t.Fatal("Do(mutated path) error = nil")
	}
}

func TestAPIErrorBodyIsBoundedAndSanitized(t *testing.T) {
	body := strings.Repeat("x", 16*1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, body, http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := jira.NewClient(jira.WithBaseURL(srv.URL))
	req, err := client.NewRequest(context.Background(), http.MethodGet, "/", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	_, err = client.Do(req, nil)
	var apiErr *jira.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("Do() error = %T %[1]v, want APIError", err)
	}
	if len(apiErr.Message) > 4096 {
		t.Fatalf("API error body length = %d, want bounded", len(apiErr.Message))
	}
}

func TestProjectSchemaCacheReturnsCopies(t *testing.T) {
	cache := jira.NewProjectSchemaCache(time.Minute)
	original := &jira.ProjectFieldSchema{
		ProjectKey: "PROJ",
		IssueType:  "Task",
		Fields:     []jira.FieldSchema{{ID: "summary", Name: "Summary"}},
	}
	cache.Set("default", "PROJ", "Task", original)

	first, ok := cache.Get("default", "PROJ", "Task")
	if !ok {
		t.Fatal("cache miss")
	}
	first.Fields[0].Name = "Mutated"

	second, ok := cache.Get("default", "PROJ", "Task")
	if !ok {
		t.Fatal("cache miss after mutation")
	}
	if second.Fields[0].Name != "Summary" {
		t.Fatalf("cache returned mutable schema pointer: %+v", second.Fields[0])
	}
}
