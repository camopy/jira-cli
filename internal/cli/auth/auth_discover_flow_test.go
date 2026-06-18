package auth

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/gechr/clog"
	"github.com/matcra587/jira-cli/internal/config"
)

// Under --debug the scoped-detection flow must be legible in the clog output:
// a "discovering cloud ID" line when the site rejects the token, then a
// "discovered cloud ID" line carrying the id once the gateway confirms it.
// Both phrases come from the operation-verb registry (auth.login.discover), so
// this also proves that verb is wired.
func TestVerifyAndDetect_DebugNarratesDiscoveryFlow(t *testing.T) {
	buf := &bytes.Buffer{}
	clog.SetOutput(clog.NewOutput(buf, clog.ColorNever))
	clog.SetVerbose(true)
	t.Cleanup(func() {
		clog.SetVerbose(false)
		clog.SetOutput(clog.NewOutput(os.Stderr, clog.ColorAuto))
	})

	srv, _, _ := scopedDetectServer(t, http.StatusUnauthorized, true)
	profile := config.Profile{BaseURL: srv.URL, Email: "me@example.com"}

	_, cloudID, err := verifyAndDetectCredential(context.Background(), profile, "tok", 0, 0, gatewayBuilderFor(srv))
	if err != nil {
		t.Fatalf("verifyAndDetectCredential: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "discovering cloud ID") {
		t.Fatalf("debug output missing the discovering-cloud-ID flow line:\n%s", out)
	}
	if !strings.Contains(out, "discovered cloud ID") || !strings.Contains(out, cloudID) {
		t.Fatalf("debug output missing 'discovered cloud ID id=%s':\n%s", cloudID, out)
	}
}

// The classic path (site accepts the token) must NOT emit the discovery flow —
// there is no cloud ID to discover.
func TestVerifyAndDetect_ClassicEmitsNoDiscoveryFlow(t *testing.T) {
	buf := &bytes.Buffer{}
	clog.SetOutput(clog.NewOutput(buf, clog.ColorNever))
	clog.SetVerbose(true)
	t.Cleanup(func() {
		clog.SetVerbose(false)
		clog.SetOutput(clog.NewOutput(os.Stderr, clog.ColorAuto))
	})

	srv, _, _ := scopedDetectServer(t, http.StatusOK, false)
	profile := config.Profile{BaseURL: srv.URL, Email: "me@example.com"}

	if _, cloudID, err := verifyAndDetectCredential(context.Background(), profile, "tok", 0, 0, gatewayBuilderFor(srv)); err != nil || cloudID != "" {
		t.Fatalf("classic detection: cloudID=%q err=%v", cloudID, err)
	}
	out := buf.String()
	if strings.Contains(out, "cloud ID") {
		t.Fatalf("classic path emitted a discovery flow line:\n%s", out)
	}
	if !strings.Contains(out, "confirmed classic token") {
		t.Fatalf("classic path should confirm the token type:\n%s", out)
	}
}
