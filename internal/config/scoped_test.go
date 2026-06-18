package config

import (
	"strings"
	"testing"
)

// A scoped-token profile must address Jira through the Atlassian gateway with
// its cloudId in the path; a classic profile hits the site directly. This is
// the single point where the two diverge, so pin both shapes.
func TestProfileClientBaseURL(t *testing.T) {
	classic := Profile{BaseURL: "https://acme.atlassian.net"}
	if got := classic.ClientBaseURL(); got != "https://acme.atlassian.net" {
		t.Fatalf("classic ClientBaseURL() = %q, want the site URL", got)
	}
	if classic.Scoped() {
		t.Fatal("a profile without a cloud_id must not report as scoped")
	}

	scoped := Profile{BaseURL: "https://acme.atlassian.net", CloudID: "a1b2c3d4-0000"}
	want := "https://api.atlassian.com/ex/jira/a1b2c3d4-0000/"
	if got := scoped.ClientBaseURL(); got != want {
		t.Fatalf("scoped ClientBaseURL() = %q, want %q", got, want)
	}
	if !scoped.Scoped() {
		t.Fatal("a profile with a cloud_id must report as scoped")
	}
}

// The gateway base URL must keep its trailing slash so the client's relative
// REST paths resolve under the /ex/jira/<cloudId>/ prefix instead of replacing
// it.
func TestGatewayBaseURLKeepsTrailingSlash(t *testing.T) {
	got := GatewayBaseURL("cid-123")
	if got != "https://api.atlassian.com/ex/jira/cid-123/" {
		t.Fatalf("GatewayBaseURL() = %q", got)
	}
	if !strings.HasSuffix(got, "/") {
		t.Fatalf("GatewayBaseURL() must end in a slash: %q", got)
	}
}

func TestValidateCloudID(t *testing.T) {
	valid := []string{
		"a436116f-02ce-4520-8fbb-7301462a1674",
		"cid-123",
		"ABC_def.456",
	}
	for _, id := range valid {
		if err := ValidateCloudID(id); err != nil {
			t.Errorf("ValidateCloudID(%q) = %v, want nil", id, err)
		}
	}
	invalid := []string{
		"",
		"   ",
		"has space",
		"has/slash",
		"https://api.atlassian.com/ex/jira/x",
	}
	for _, id := range invalid {
		if err := ValidateCloudID(id); err == nil {
			t.Errorf("ValidateCloudID(%q) = nil, want an error", id)
		}
	}
}

// Config.Validate must reject a malformed cloud_id so a broken value cannot
// silently produce an unroutable gateway URL at request time.
func TestConfigValidateRejectsBadCloudID(t *testing.T) {
	cfg := &Config{
		DefaultProfile: "work",
		Profiles: []Profile{{
			Name:    "work",
			BaseURL: "https://acme.atlassian.net",
			CloudID: "bad id/with slash",
		}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Config.Validate() accepted a malformed cloud_id")
	}

	cfg.Profiles[0].CloudID = "a1b2c3d4-0000"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Config.Validate() rejected a valid cloud_id: %v", err)
	}
}

// `jira config set/get profiles.<name>.cloud_id` must round-trip, validate on
// set, and clear scoped routing when set to empty.
func TestSetGetCloudIDKey(t *testing.T) {
	cfg := &Config{
		Profiles: []Profile{{Name: "work", BaseURL: "https://acme.atlassian.net"}},
	}
	const key = "profiles.work.cloud_id"

	if err := cfg.setProfileField(key, "cid-42"); err != nil {
		t.Fatalf("setProfileField(cloud_id) = %v", err)
	}
	if got, ok := cfg.getProfileField(key); !ok || got != "cid-42" {
		t.Fatalf("getProfileField(cloud_id) = %q ok=%v, want cid-42", got, ok)
	}
	if !cfg.Profiles[0].Scoped() {
		t.Fatal("setting cloud_id should make the profile scoped")
	}

	if err := cfg.setProfileField(key, "bad/value"); err == nil {
		t.Fatal("setProfileField accepted an invalid cloud_id")
	}

	// Clearing reverts to a classic, site-addressed token.
	if err := cfg.setProfileField(key, ""); err != nil {
		t.Fatalf("setProfileField(cloud_id, \"\") = %v", err)
	}
	if cfg.Profiles[0].Scoped() {
		t.Fatal("clearing cloud_id should revert the profile to classic")
	}
}
