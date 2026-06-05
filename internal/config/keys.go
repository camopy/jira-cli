package config

import (
	"fmt"
	"strconv"
	"strings"
)

// KeyDesc describes a single configuration key visible to `jira config get/set`.
type KeyDesc struct {
	Name        string
	Description string
	// Choices lists the valid values when the key has a closed enum;
	// nil means freeform string/int.
	Choices []string
}

// profileFieldKey is the template suffix for a profile-scoped key.
// Real keys are `profiles.<name>.<field>` after expansion.
const profileFieldPrefix = "profiles."

var topLevelKeys = []KeyDesc{
	{"default_profile", "Active profile name", nil},
	{"queries_path", "Saved JQL queries directory", nil},
	{"editor", "Default external editor command", nil},
	{"theme.name", "Theme name", ThemeNameValues},
	{"theme.path", "Theme TOML path", nil},
	{"tui.refresh_interval", "TUI refresh seconds", nil},
	{"tui.default_tab", "TUI default tab on launch", []string{"issues", "epics", "search", "activity"}},
}

// profileFieldKeys are templates relative to a profile.
// Their full form is `profiles.<name>.<Name>` once a profile name is bound.
var profileFieldKeys = []KeyDesc{
	{"base_url", "Jira base URL", nil},
	{"auth_type", "Auth type", []string{"token"}},
	{"email", "Atlassian Cloud account email", nil},
	{"default_project", "Default project key", nil},
	{"default_issue_type", "Default issue type", nil},
	{"default_board", "Default board NAME applied when --board is not supplied", nil},
	{"refresh_interval", "Per-profile refresh seconds", nil},
	{"timeout", "HTTP timeout seconds", nil},
	{"workday_seconds", "Workday length used by worklog math", nil},
	{"secret_backend", "Credential backend", []string{"keyring", "1password"}},
	{"onepassword_account", "1Password account name", nil},
	{"vault", "1Password vault", nil},
	{"item", "1Password item", nil},
	{"account_id", "Atlassian Cloud accountId for the current user", nil},
	{"read_only", "Block all mutating commands for this profile", []string{"true", "false"}},
	{"editor", "Per-profile editor override", nil},
}

// Keys returns every valid config key, with profile-scoped keys expanded for
// each profile actually present in cfg. cfg may be nil; in that case only the
// templates are returned with the literal placeholder `<profile>`.
func Keys(cfg *Config) []KeyDesc {
	out := make([]KeyDesc, 0, len(topLevelKeys)+len(profileFieldKeys)*max(1, len(cfgProfiles(cfg))))
	out = append(out, topLevelKeys...)

	profiles := cfgProfiles(cfg)
	if len(profiles) == 0 {
		profiles = []string{"<profile>"}
	}
	for _, name := range profiles {
		for _, k := range profileFieldKeys {
			out = append(out, KeyDesc{
				Name:        profileFieldPrefix + name + "." + k.Name,
				Description: k.Description + " (profile " + name + ")",
				Choices:     k.Choices,
			})
		}
	}
	return out
}

// KeyChoices returns the enum choices for a fully-qualified key, or nil if the
// key is freeform or unknown.
func KeyChoices(key string) []string {
	for _, k := range topLevelKeys {
		if k.Name == key {
			return k.Choices
		}
	}
	if strings.HasPrefix(key, profileFieldPrefix) {
		parts := strings.SplitN(key, ".", 3)
		if len(parts) == 3 {
			for _, k := range profileFieldKeys {
				if k.Name == parts[2] {
					return k.Choices
				}
			}
		}
	}
	return nil
}

func cfgProfiles(cfg *Config) []string {
	if cfg == nil {
		return nil
	}
	names := make([]string, 0, len(cfg.Profiles))
	for _, p := range cfg.Profiles {
		names = append(names, p.Name)
	}
	return names
}

// findProfile returns a pointer to the named profile within c, or nil if it
// doesn't exist.
func (c *Config) findProfile(name string) *Profile {
	for i := range c.Profiles {
		if c.Profiles[i].Name == name {
			return &c.Profiles[i]
		}
	}
	return nil
}

// getProfileField resolves a `profiles.<name>.<field>` key. Returns the value
// stringified and true on success; "" and false otherwise.
func (c *Config) getProfileField(key string) (string, bool) {
	parts := strings.SplitN(key, ".", 3)
	if len(parts) != 3 {
		return "", false
	}
	p := c.findProfile(parts[1])
	if p == nil {
		return "", false
	}
	switch parts[2] {
	case "base_url":
		return p.BaseURL, true
	case "auth_type":
		return string(p.AuthType), true
	case "email":
		return p.Email, true
	case "default_project":
		return p.DefaultProject, true
	case "default_issue_type":
		return p.DefaultIssueType, true
	case "default_board":
		return p.DefaultBoard, true
	case "refresh_interval":
		return strconv.Itoa(p.RefreshInterval), true
	case "timeout":
		return strconv.Itoa(p.TimeoutSeconds), true
	case "workday_seconds":
		return strconv.Itoa(p.WorkdaySeconds), true
	case "secret_backend":
		return string(p.SecretBackend), true
	case "onepassword_account":
		return p.OnePasswordAccount, true
	case "vault":
		return p.Vault, true
	case "item":
		return p.Item, true
	case "account_id":
		return p.AccountID, true
	case "read_only":
		return strconv.FormatBool(p.ReadOnly), true
	case "editor":
		return p.Editor, true
	}
	return "", false
}

// setProfileField mutates a `profiles.<name>.<field>` key. Returns nil on
// success, or an error describing why the assignment was rejected (unknown
// profile/field, invalid enum, unparsable int/bool).
func (c *Config) setProfileField(key, value string) error {
	parts := strings.SplitN(key, ".", 3)
	if len(parts) != 3 {
		return fmt.Errorf("unknown config key %q", key)
	}
	p := c.findProfile(parts[1])
	if p == nil {
		return fmt.Errorf("unknown profile %q", parts[1])
	}
	switch parts[2] {
	case "base_url":
		p.BaseURL = NormalizeBaseURL(value)
	case "auth_type":
		if !supportedAuthType(AuthType(value)) {
			return fmt.Errorf("invalid auth_type %q (valid: token)", value)
		}
		p.AuthType = AuthType(value)
	case "email":
		p.Email = value
	case "default_project":
		p.DefaultProject = value
	case "default_issue_type":
		p.DefaultIssueType = value
	case "default_board":
		// NO validation at set time. Cache may be empty.
		// Use-time consumers (boardscope.FromFlags) validate.
		p.DefaultBoard = value
	case "refresh_interval":
		n, err := strconv.Atoi(value)
		if err != nil || n <= 0 {
			return fmt.Errorf("refresh_interval must be a positive integer, got %q", value)
		}
		p.RefreshInterval = n
	case "timeout":
		n, err := strconv.Atoi(value)
		if err != nil || n <= 0 {
			return fmt.Errorf("timeout must be a positive integer, got %q", value)
		}
		p.TimeoutSeconds = n
	case "workday_seconds":
		n, err := strconv.Atoi(value)
		if err != nil || n <= 0 {
			return fmt.Errorf("workday_seconds must be a positive integer, got %q", value)
		}
		p.WorkdaySeconds = n
	case "secret_backend":
		switch SecretBackend(value) {
		case SecretBackendKeyring, SecretBackendOnePassword:
			p.SecretBackend = SecretBackend(value)
		default:
			return fmt.Errorf("invalid secret_backend %q (valid: keyring, 1password)", value)
		}
	case "onepassword_account":
		p.OnePasswordAccount = value
	case "vault":
		p.Vault = value
	case "item":
		p.Item = value
	case "account_id":
		p.AccountID = value
	case "read_only":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("read_only must be true or false, got %q", value)
		}
		p.ReadOnly = b
	case "editor":
		p.Editor = value
	default:
		return fmt.Errorf("unknown profile field %q", parts[2])
	}
	return nil
}
