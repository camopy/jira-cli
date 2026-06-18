package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// discoverCloudID reads the Atlassian cloudId from a site's unauthenticated
// _edge/tenant_info probe so `auth login --scoped` can route a granular token
// through the gateway without the user hunting down the id by hand.
func TestDiscoverCloudIDParsesTenantInfo(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"cloudId":"a1b2c3d4-7777","activationId":"x"}`))
	}))
	defer srv.Close()

	id, err := discoverCloudID(context.Background(), srv.URL, 0)
	if err != nil {
		t.Fatalf("discoverCloudID() error = %v", err)
	}
	if id != "a1b2c3d4-7777" {
		t.Fatalf("discoverCloudID() = %q, want a1b2c3d4-7777", id)
	}
	if gotPath != tenantInfoPath {
		t.Fatalf("probed path = %q, want %q", gotPath, tenantInfoPath)
	}
}

// A non-200 from the probe (or a body without a cloudId) must surface as an
// error so login can fall back to telling the user to pass --cloud-id rather
// than storing an empty, unroutable id.
func TestDiscoverCloudIDErrors(t *testing.T) {
	t.Run("non-200", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()
		if _, err := discoverCloudID(context.Background(), srv.URL, 0); err == nil {
			t.Fatal("discoverCloudID() = nil error on HTTP 404")
		}
	})

	t.Run("no cloudId in body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"activationId":"x"}`))
		}))
		defer srv.Close()
		if _, err := discoverCloudID(context.Background(), srv.URL, 0); err == nil {
			t.Fatal("discoverCloudID() = nil error when the body carried no cloudId")
		}
	})

	t.Run("blank base URL", func(t *testing.T) {
		if _, err := discoverCloudID(context.Background(), "   ", 0); err == nil {
			t.Fatal("discoverCloudID() = nil error for a blank base URL")
		}
	})
}
