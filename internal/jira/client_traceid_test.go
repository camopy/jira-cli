package jira

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

// A failed exchange must carry Jira's own trace id so the envelope can
// hand Atlassian support a correlatable value; the CLI's request_id is
// local and means nothing server-side. Atl-Traceid is the id support
// quotes, so it wins over the older X-ARequestId spelling.
func TestAPIErrorCapturesUpstreamRequestID(t *testing.T) {
	tests := map[string]struct {
		headers map[string]string
		want    string
	}{
		"Atl-Traceid": {
			headers: map[string]string{"Atl-Traceid": "a1b2c3d4e5"},
			want:    "a1b2c3d4e5",
		},
		"X-ARequestId fallback": {
			headers: map[string]string{"X-ARequestId": "node-77-req"},
			want:    "node-77-req",
		},
		"Atl-Traceid preferred over X-ARequestId": {
			headers: map[string]string{"Atl-Traceid": "trace-1", "X-ARequestId": "node-2"},
			want:    "trace-1",
		},
		"absent headers": {
			headers: nil,
			want:    "",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			client := newHTTPHandlerClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				for k, v := range tc.headers {
					w.Header().Set(k, v)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"errorMessages":["boom"]}`))
			}))
			req, err := client.NewRequest(context.Background(), http.MethodGet, "/some/path", nil)
			if err != nil {
				t.Fatalf("NewRequest: %v", err)
			}
			_, err = client.Do(req, nil)
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("expected *APIError, got %T: %v", err, err)
			}
			if apiErr.UpstreamRequestID != tc.want {
				t.Fatalf("UpstreamRequestID = %q, want %q", apiErr.UpstreamRequestID, tc.want)
			}
		})
	}
}

// Successful responses expose the trace id through the Response so
// envelope writers can put it in meta without re-reading headers.
func TestResponseUpstreamRequestID(t *testing.T) {
	client := newHTTPHandlerClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Atl-Traceid", "ok-trace-9")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	req, err := client.NewRequest(context.Background(), http.MethodGet, "/some/path", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	var out map[string]any
	resp, err := client.Do(req, &out)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if got := resp.UpstreamRequestID(); got != "ok-trace-9" {
		t.Fatalf("Response.UpstreamRequestID() = %q, want ok-trace-9", got)
	}
	if got := (Response{}).UpstreamRequestID(); got != "" {
		t.Fatalf("zero Response.UpstreamRequestID() = %q, want empty", got)
	}
}
