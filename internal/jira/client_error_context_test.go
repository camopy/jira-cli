package jira

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type errReader struct {
	err error
}

func (r errReader) Read([]byte) (int, error) {
	return 0, r.err
}

func (r errReader) Close() error {
	return nil
}

func TestClientDoWrapsTransportErrorWithRequestContext(t *testing.T) {
	sentinel := errors.New("dial failed")
	client := NewClient(
		WithBaseURL("https://jira.example.com/"),
		WithHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, sentinel
		})}),
	)
	req, err := client.NewRequest(context.Background(), http.MethodGet, RESTPath("myself"), nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	_, err = client.Do(req, nil)
	if !errors.Is(err, sentinel) {
		t.Fatalf("Do() error = %v, want wrapping sentinel", err)
	}
	if !strings.Contains(err.Error(), "jira request GET /rest/api/3/myself") {
		t.Fatalf("Do() error lacks operation context: %v", err)
	}
}

func TestClientDoWrapsBodyReadError(t *testing.T) {
	sentinel := errors.New("read failed")
	client := NewClient(
		WithBaseURL("https://jira.example.com/"),
		WithHTTPClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       errReader{err: sentinel},
				Request:    req,
			}, nil
		})}),
	)
	req, err := client.NewRequest(context.Background(), http.MethodGet, RESTPath("myself"), nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	_, err = client.Do(req, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("Do() error = %T %[1]v, want *APIError", err)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("Do() error = %v, want wrapping sentinel", err)
	}
	if !strings.Contains(apiErr.Message, "read response body") {
		t.Fatalf("APIError message = %q, want read context", apiErr.Message)
	}
}

func TestClientDoRejectsOversizedSuccessBody(t *testing.T) {
	client := NewClient(
		WithBaseURL("https://jira.example.com/"),
		WithHTTPClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       io.NopCloser(io.LimitReader(zeroReader{}, maxResponseBodyBytes+1)),
				Request:    req,
			}, nil
		})}),
	)
	req, err := client.NewRequest(context.Background(), http.MethodGet, RESTPath("myself"), nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	_, err = client.Do(req, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("Do() error = %T %[1]v, want *APIError", err)
	}
	if !strings.Contains(apiErr.Message, "response body exceeded") {
		t.Fatalf("APIError message = %q, want body limit", apiErr.Message)
	}
}

func TestClientDoBoundsErrorBodyRead(t *testing.T) {
	body := &countingReadCloser{reader: io.LimitReader(zeroReader{}, maxErrorBodyBytes+8192)}
	client := NewClient(
		WithBaseURL("https://jira.example.com/"),
		WithHTTPClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Header:     http.Header{},
				Body:       body,
				Request:    req,
			}, nil
		})}),
	)
	req, err := client.NewRequest(context.Background(), http.MethodGet, RESTPath("myself"), nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	resp, err := client.Do(req, nil)
	if err == nil {
		t.Fatal("Do() error = nil, want API error")
	}
	if body.read > maxErrorBodyBytes+1 {
		t.Fatalf("read %d error-body bytes, want <= %d", body.read, maxErrorBodyBytes+1)
	}
	if len(resp.RawBody) > maxErrorBodyBytes {
		t.Fatalf("RawBody length = %d, want <= %d", len(resp.RawBody), maxErrorBodyBytes)
	}
}

func TestAPIErrorRedactsSensitiveBodyFields(t *testing.T) {
	client := newHTTPHandlerClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errorMessages":["bad"],"access_token":"secret-token","errors":{"password":"secret-password"}}`))
	}))

	req, err := client.NewRequest(context.Background(), http.MethodGet, RESTPath("myself"), nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	_, err = client.Do(req, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("Do() error = %T %[1]v, want *APIError", err)
	}
	if strings.Contains(apiErr.Message, "secret-token") || strings.Contains(apiErr.Message, "secret-password") {
		t.Fatalf("APIError leaked sensitive body fields: %q", apiErr.Message)
	}
	if !strings.Contains(apiErr.Message, "REDACTED") {
		t.Fatalf("APIError message = %q, want redaction marker", apiErr.Message)
	}
}

func TestClientDoRedactsInvalidJSONBodyPrefix(t *testing.T) {
	client := newHTTPHandlerClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`token=server-secret`))
	}))

	req, err := client.NewRequest(context.Background(), http.MethodGet, RESTPath("myself"), nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	var out map[string]any
	_, err = client.Do(req, &out)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("Do() error = %T %[1]v, want *APIError", err)
	}
	if strings.Contains(apiErr.Message, "server-secret") {
		t.Fatalf("APIError leaked invalid JSON body prefix: %q", apiErr.Message)
	}
	if !strings.Contains(apiErr.Message, "REDACTED") {
		t.Fatalf("APIError message = %q, want redaction marker", apiErr.Message)
	}
}

func TestClientDebugRedactsSensitiveRequestAndResponseBodies(t *testing.T) {
	ctx, debugLogs := newDebugLogContext(t)

	client := NewClient(
		WithBaseURL("https://jira.example.com/"),
		WithHTTPClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"access_token":"server-secret"}`)),
				Request:    req,
			}, nil
		})}),
		WithDebug(true),
	)
	req, err := client.NewRequest(ctx, http.MethodPost, RESTPath("issue"), map[string]any{"token": "client-secret"})
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	if _, err := client.Do(req, nil); err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	logText := debugLogs.String()
	if strings.Contains(logText, "client-secret") || strings.Contains(logText, "server-secret") {
		t.Fatalf("debug log leaked sensitive body fields:\n%s", logText)
	}
	if !strings.Contains(logText, "REDACTED") {
		t.Fatalf("debug log missing redaction marker:\n%s", logText)
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	return len(p), nil
}

type countingReadCloser struct {
	reader io.Reader
	read   int
}

func (c *countingReadCloser) Read(p []byte) (int, error) {
	n, err := c.reader.Read(p)
	c.read += n
	return n, err
}

func (c *countingReadCloser) Close() error {
	return nil
}
