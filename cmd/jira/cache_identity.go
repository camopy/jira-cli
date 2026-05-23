package main

import (
	"github.com/matcra587/jira-cli/internal/cache"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/cli/startup"
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/spf13/cobra"
)

func cacheKeyForProfile(cmd *cobra.Command, profile config.Profile) string {
	return cache.Key(profile.Name, profile.BaseURL, cacheConfigPath(cmd))
}

func cacheConfigPath(cmd *cobra.Command) string {
	if path := cmdutil.ConfigPath(cmd); path != "" {
		return path
	}
	return config.DefaultPath()
}

func cacheKeyFromStartup(globals startup.Globals, cfg *config.Config, profileName string) string {
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
