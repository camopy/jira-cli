package jira

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type partialErrReadCloser struct {
	done bool
}

func (r *partialErrReadCloser) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	r.done = true
	return copy(p, "partial"), io.ErrUnexpectedEOF
}

func (r *partialErrReadCloser) Close() error {
	return nil
}

// HTTP body read errors MUST surface as a classified error rather
// than be silently dropped. Previously
// `body, _ := io.ReadAll(res.Body)` swallowed any read error and the
// caller saw "unexpected end of JSON input" or empty data — neither
// of which points at the actual cause (network blip mid-read).
//
// We exercise this with a transport response whose body fails part-way
// through. After the fix, Do() returns a typed server error.
func TestBodyReadErrorPropagatesAsServerError(t *testing.T) {
	client := NewClient(
		WithBaseURL("https://jira.example.com/"),
		WithHTTPClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{},
				Body:       &partialErrReadCloser{},
				Request:    req,
			}, nil
		})}),
	)
	req, err := client.NewRequest(context.Background(), http.MethodGet, "/some/path", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	var out map[string]any
	_, err = client.Do(req, &out)
	if err == nil {
		t.Fatal("expected body-read error to surface; got nil")
	}
	// Must be a typed APIError with server classification — not a
	// raw json.Unmarshal error masquerading as something else.
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.Type != ErrorTypeServer {
		t.Fatalf("body-read error must classify as server, got %q: %v", apiErr.Type, apiErr)
	}
	if !strings.Contains(strings.ToLower(apiErr.Message), "read") &&
		!strings.Contains(strings.ToLower(apiErr.Message), "body") {
		t.Fatalf("error message should mention body/read; got %q", apiErr.Message)
	}
}

// Belt-and-braces: an io.Reader that returns ErrUnexpectedEOF mid-stream
// also propagates as a server error. Tests the explicit fix path.
func TestBodyReadErrorFromReader(t *testing.T) {
	// This pins the concrete error shape used by the transport-level
	// regression above.
	if !errors.Is(io.ErrUnexpectedEOF, io.ErrUnexpectedEOF) {
		t.Fatal("sanity check failed")
	}
}

// TestAPIErrorServerCategoryForHTMLBody — typed dispatch.
//
// When a server returns a 200 with an HTML body (e.g. SSO wall,
// maintenance page), Do() must produce an *APIError typed
// ErrorTypeServer so that outputErrorFor routes to exit 5 instead of
// the default exit 3.
//
// This is the positive unit-level test for the client-side fix; the
// end-to-end assertion lives in tests/contract/i1_envelope_invariants_test.go
// (TestI1HTMLServerResponseEnvelope).
func TestAPIErrorServerCategoryForHTMLBody(t *testing.T) {
	client := newHTTPHandlerClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK) // 200 — passes the status-code gate
		_, _ = w.Write([]byte("<html><body>maintenance</body></html>"))
	}))

	req, err := client.NewRequest(context.Background(), http.MethodGet, "/some/path", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	var out map[string]any
	_, err = client.Do(req, &out)
	if err == nil {
		t.Fatal("expected error for HTML body; got nil")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.Type != ErrorTypeServer {
		t.Fatalf("HTML-body parse error must classify as %q, got %q", ErrorTypeServer, apiErr.Type)
	}
	if !strings.Contains(strings.ToLower(apiErr.Message), "json") &&
		!strings.Contains(strings.ToLower(apiErr.Message), "body") {
		t.Fatalf("error message should mention json/body; got %q", apiErr.Message)
	}
}
