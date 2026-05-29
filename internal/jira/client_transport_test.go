package jira

import (
	"net/http"
	"testing"
	"time"
)

func TestDefaultHTTPClientUsesParallelFriendlyTransport(t *testing.T) {
	client := NewClient()

	if client.client.Timeout != defaultHTTPTimeout {
		t.Fatalf("default timeout = %s, want %s", client.client.Timeout, defaultHTTPTimeout)
	}
	transport, ok := client.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("default transport = %T, want *http.Transport", client.client.Transport)
	}
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
	transport, ok := client.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %T, want *http.Transport", client.client.Transport)
	}
	if transport.MaxIdleConnsPerHost < maxIdleConnsPerHost {
		t.Fatalf("MaxIdleConnsPerHost = %d, want at least %d", transport.MaxIdleConnsPerHost, maxIdleConnsPerHost)
	}
}
