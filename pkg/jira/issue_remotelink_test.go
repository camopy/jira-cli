package jira

import (
	"context"
	"strings"
	"testing"
)

// : web-link URLs must be restricted to http:// and https://
// before any HTTP call to Jira. Defense-in-depth — Jira itself strips
// `javascript:` at render time but the CLI shouldn't accept anything
// other than the two web schemes its name implies.
//
// AddRemoteLink rejects everything outside the http/https allowlist
// with a clear error naming the rejected scheme. The check fires
// before Client.Do, so no HTTP request is made for blocked schemes.
func TestAddRemoteLinkRejectsNonHTTPSchemes(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{"javascript", "javascript:alert(1)"},
		{"file", "file:///etc/passwd"},
		{"ftp", "ftp://attacker.example/payload"},
		{"data", "data:text/html,<script>alert(1)</script>"},
		{"vbscript", "vbscript:msgbox"},
		{"mailto", "mailto:team@example.com"},
		{"about", "about:blank"},
		{"missing-scheme", "example.com/no-scheme"},
		{"empty-after-scheme", "http://"},
	}
	svc := &issueService{client: nil} // no client — call must fail before reaching transport
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.AddRemoteLink(context.Background(), "KAN-1", &RemoteLinkRequest{
				URL:   tc.url,
				Title: "x",
			})
			if err == nil {
				t.Fatalf("expected error for URL %q, got nil", tc.url)
			}
			if !strings.Contains(err.Error(), "scheme") && !strings.Contains(err.Error(), "URL") {
				t.Fatalf("error must mention scheme/URL; got %q", err.Error())
			}
		})
	}
}

func TestValidateWebLinkURLAcceptsHTTPAndHTTPS(t *testing.T) {
	cases := []string{
		"http://example.com",
		"https://example.com",
		"https://example.com/path?q=1#frag",
		"HTTP://EXAMPLE.COM", // case-insensitive scheme per RFC 3986
		"HTTPS://example.com",
	}
	for _, u := range cases {
		t.Run(u, func(t *testing.T) {
			if err := validateWebLinkURL(u); err != nil {
				t.Fatalf("validateWebLinkURL(%q) = %v, want nil", u, err)
			}
		})
	}
}
