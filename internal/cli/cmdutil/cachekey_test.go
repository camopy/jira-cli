package cmdutil_test

import (
	"testing"

	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/cli/startup"
	"github.com/matcra587/jira-cli/internal/config"
)

// TestCacheKeyFromStartupGolden pins the on-disk cache namespace string. The
// key names a real cache directory, so any drift silently invalidates every
// user's cache without a visible error. The want values are computed
// independently as profile + "-" + sha256(abs(configPath) "\x00" siteURL
// "\x00" profile)[:16]. An absolute configPath is used so filepath.Abs is a
// no-op and the test is cwd-stable. The resolved-profile case feeds a BaseURL
// with mixed case and a trailing slash to pin cache.Key's siteURL
// normalization (lowercase + trailing-slash trim) and the (name, siteURL)
// argument order.
func TestCacheKeyFromStartupGolden(t *testing.T) {
	const cfgPath = "/abs/config.toml"
	withProfile := &config.Config{
		DefaultProfile: "default",
		Profiles:       []config.Profile{{Name: "default", BaseURL: "https://X.atlassian.net/"}},
	}
	cases := []struct {
		name        string
		cfg         *config.Config
		profileName string
		want        string
	}{
		{"nil cfg, default profile", nil, "default", "default-293e75211cf5bba2"},
		{"nil cfg, named profile", nil, "work", "work-20203a68762350a6"},
		{"resolved profile with normalized base url", withProfile, "default", "default-ef794cba5138fb60"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := cmdutil.CacheKeyFromStartup(startup.Globals{ConfigPath: cfgPath}, tc.cfg, tc.profileName)
			if got != tc.want {
				t.Fatalf("CacheKeyFromStartup(%q) = %q, want %q", tc.profileName, got, tc.want)
			}
		})
	}
}
