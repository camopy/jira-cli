package jira

import (
	"net/http"
	"testing"
	"time"
)

// poolTransport unwraps the retry layer (always installed, inert by default)
// to reach the tuned connection-pool *http.Transport underneath.
func poolTransport(t *testing.T, rt http.RoundTripper) *http.Transport {
	t.Helper()
	if retry, ok := rt.(*retryTransport); ok {
		rt = retry.base
	}
	transport, ok := rt.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %T, want *http.Transport", rt)
	}
	return transport
}

func TestDefaultHTTPClientUsesParallelFriendlyTransport(t *testing.T) {
	client := NewClient()

	if client.client.Timeout != defaultHTTPTimeout {
		t.Fatalf("default timeout = %s, want %s", client.client.Timeout, defaultHTTPTimeout)
	}
	transport := poolTransport(t, client.client.Transport)
	if transport.MaxIdleConns < defaultMaxIdleConns {
		t.Fatalf("MaxIdleConns = %d, want at least %d", transport.MaxIdleConns, defaultMaxIdleConns)
	}
	if transport.MaxIdleConnsPerHost < maxIdleConnsPerHost {
		t.Fatalf("MaxIdleConnsPerHost = %d, want at least %d", transport.MaxIdleConnsPerHost, maxIdleConnsPerHost)
	}
}

func TestWithHTTPTimeoutPreservesDefaultTransport(t *testing.T) {
	client := NewClient(WithHTTPTimeout(time.Second))

	if client.client.Timeout != time.Second {
		t.Fatalf("timeout = %s, want 1s", client.client.Timeout)
	}
	transport := poolTransport(t, client.client.Transport)
	if transport.MaxIdleConnsPerHost < maxIdleConnsPerHost {
		t.Fatalf("MaxIdleConnsPerHost = %d, want at least %d", transport.MaxIdleConnsPerHost, maxIdleConnsPerHost)
	}
}
