package main

import (
	"github.com/matcra587/jira-cli/internal/cache"
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/spf13/cobra"
)

func cacheKeyForProfile(cmd *cobra.Command, profile config.Profile) string {
	return cache.Key(profile.Name, profile.BaseURL, cacheConfigPath(cmd))
}

func cacheConfigPath(cmd *cobra.Command) string {
	if path := configPath(cmd); path != "" {
		return path
	}
	return config.DefaultPath()
}

func cacheKeyFromStartup(startup startupGlobals, cfg *config.Config, profileName string) string {
	if startup.ConfigPath == "" {
		startup.ConfigPath = config.DefaultPath()
	}
	if cfg == nil {
		return cache.Key(profileName, "", startup.ConfigPath)
	}
	profile, err := cfg.ResolveProfile(profileName)
	if err != nil {
		return cache.Key(profileName, "", startup.ConfigPath)
	}
	return cache.Key(profile.Name, profile.BaseURL, startup.ConfigPath)
}
