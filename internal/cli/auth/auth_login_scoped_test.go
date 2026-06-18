package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/matcra587/jira-cli/internal/config"
)

// scopedDetectServer stands in for a Jira tenant during token auto-detection.
// It serves three endpoints off one httptest server: the site /myself, the
// unauthenticated tenant_info probe, and the gateway /myself (under an
// /ex/jira/<id>/ prefix). siteStatus controls whether the site accepts the
// token (classic) or rejects it (scoped/bad); gatewayOK controls whether the
// gateway accepts it.
func scopedDetectServer(t *testing.T, siteStatus int, gatewayOK bool) (*httptest.Server, *int32, *int32) {
	t.Helper()
	var tenantHits, gatewayHits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == tenantInfoPath:
			atomic.AddInt32(&tenantHits, 1)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"cloudId":"detected-cloud-id"}`))
		case strings.HasPrefix(r.URL.Path, "/ex/jira/"):
			atomic.AddInt32(&gatewayHits, 1)
			if !gatewayOK {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"errorMessages":["Client must be authenticated"]}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"accountId":"acc-scoped","displayName":"Scoped User"}`))
		case r.URL.Path == "/rest/api/3/myself":
			if siteStatus != http.StatusOK {
				w.WriteHeader(siteStatus)
				_, _ = w.Write([]byte(`{"errorMessages":["nope"]}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"accountId":"acc-classic","displayName":"Classic User"}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &tenantHits, &gatewayHits
}

func gatewayBuilderFor(srv *httptest.Server) func(string) string {
	return func(cloudID string) string { return srv.URL + "/ex/jira/" + cloudID + "/" }
}

// A token the site accepts is classic: detection returns the user and an empty
// cloudId, and never probes the gateway or tenant_info.
func TestVerifyAndDetectClassicToken(t *testing.T) {
	srv, tenantHits, gatewayHits := scopedDetectServer(t, http.StatusOK, false)
	profile := config.Profile{BaseURL: srv.URL, Email: "me@example.com"}

	user, cloudID, err := verifyAndDetectCredential(context.Background(), profile, "tok", 0, 0, gatewayBuilderFor(srv))
	if err != nil {
		t.Fatalf("verifyAndDetectCredential() error = %v", err)
	}
	if cloudID != "" {
		t.Fatalf("classic token returned cloudId %q, want empty", cloudID)
	}
	if user == nil || user.AccountID != "acc-classic" {
		t.Fatalf("classic detection returned wrong user: %+v", user)
	}
	if *tenantHits != 0 || *gatewayHits != 0 {
		t.Fatalf("classic detection should not probe tenant_info/gateway (tenant=%d gateway=%d)", *tenantHits, *gatewayHits)
	}
}

// A token the site rejects but the gateway accepts is scoped: detection
// discovers the cloudId and returns it alongside the gateway-verified user.
func TestVerifyAndDetectScopedToken(t *testing.T) {
	srv, tenantHits, gatewayHits := scopedDetectServer(t, http.StatusUnauthorized, true)
	profile := config.Profile{BaseURL: srv.URL, Email: "me@example.com"}

	user, cloudID, err := verifyAndDetectCredential(context.Background(), profile, "tok", 0, 0, gatewayBuilderFor(srv))
	if err != nil {
		t.Fatalf("verifyAndDetectCredential() error = %v", err)
	}
	if cloudID != "detected-cloud-id" {
		t.Fatalf("scoped detection returned cloudId %q, want detected-cloud-id", cloudID)
	}
	if user == nil || user.AccountID != "acc-scoped" {
		t.Fatalf("scoped detection returned wrong user: %+v", user)
	}
	if *tenantHits == 0 || *gatewayHits == 0 {
		t.Fatalf("scoped detection must probe tenant_info and gateway (tenant=%d gateway=%d)", *tenantHits, *gatewayHits)
	}
}

// A token rejected at BOTH the site and the gateway is simply bad: detection
// surfaces the original site auth error and no cloudId.
func TestVerifyAndDetectBadTokenRejectedEverywhere(t *testing.T) {
	srv, _, gatewayHits := scopedDetectServer(t, http.StatusUnauthorized, false)
	profile := config.Profile{BaseURL: srv.URL, Email: "me@example.com"}

	_, cloudID, err := verifyAndDetectCredential(context.Background(), profile, "tok", 0, 0, gatewayBuilderFor(srv))
	if err == nil {
		t.Fatal("verifyAndDetectCredential() error = nil for a token rejected everywhere")
	}
	if cloudID != "" {
		t.Fatalf("bad token returned cloudId %q, want empty", cloudID)
	}
	if *gatewayHits == 0 {
		t.Fatal("a site-rejected token should still be probed against the gateway")
	}
}

// A non-auth failure at the site (network / 5xx) is not a token-type question:
// it is surfaced as-is, and the gateway/tenant_info are never probed.
func TestVerifyAndDetectNonAuthErrorIsSurfaced(t *testing.T) {
	srv, tenantHits, gatewayHits := scopedDetectServer(t, http.StatusServiceUnavailable, true)
	profile := config.Profile{BaseURL: srv.URL, Email: "me@example.com"}

	_, _, err := verifyAndDetectCredential(context.Background(), profile, "tok", 0, 0, gatewayBuilderFor(srv))
	if err == nil {
		t.Fatal("verifyAndDetectCredential() error = nil for a 503 site response")
	}
	if *tenantHits != 0 || *gatewayHits != 0 {
		t.Fatalf("a 5xx site error must not trigger scoped probing (tenant=%d gateway=%d)", *tenantHits, *gatewayHits)
	}
}
