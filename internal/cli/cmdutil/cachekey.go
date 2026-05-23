package cmdutil

import (
	"github.com/matcra587/jira-cli/internal/cache"
	"github.com/matcra587/jira-cli/internal/cli/startup"
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/spf13/cobra"
)

// CacheKeyForProfile returns the cache namespace key for an explicit profile,
// derived from the profile identity (name + base URL) and the active config
// path. The key names the on-disk cache directory, so its composition must
// stay stable — see TestCacheKeyFromStartupGolden.
func CacheKeyForProfile(cmd *cobra.Command, profile config.Profile) string {
	return cache.Key(profile.Name, profile.BaseURL, CacheConfigPath(cmd))
}

// CacheConfigPath returns the config path used for cache-key identity: the
// root --config flag value when set, otherwise the default config path.
func CacheConfigPath(cmd *cobra.Command) string {
	if path := ConfigPath(cmd); path != "" {
		return path
	}
	return config.DefaultPath()
}

// CacheKeyFromStartup returns the cache namespace key for the pre-cobra
// startup scan, resolving the named profile against cfg when available and
// falling back to the bare (profile, "", configPath) identity otherwise.
func CacheKeyFromStartup(globals startup.Globals, cfg *config.Config, profileName string) string {
	if globals.ConfigPath == "" {
		globals.ConfigPath = config.DefaultPath()
	}
	if cfg == nil {
		return cache.Key(profileName, "", globals.ConfigPath)
	}
	profile, err := cfg.ResolveProfile(profileName)
	if err != nil {
		return cache.Key(profileName, "", globals.ConfigPath)
	}
	return cache.Key(profile.Name, profile.BaseURL, globals.ConfigPath)
}
