package config

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

const (
	DefaultRefreshIntervalSeconds = 30
	DefaultTimeoutSeconds         = 30
	DefaultWorkdaySeconds         = 28800
)

type AuthType string

const (
	AuthTypeToken AuthType = "token"
	AuthTypeBasic AuthType = "basic"
	AuthTypePAT   AuthType = "pat"
	AuthTypeMTLS  AuthType = "mtls"
)

type SecretBackend string

const (
	SecretBackendKeyring     SecretBackend = "keyring"
	SecretBackendOnePassword SecretBackend = "1password"
)

type Config struct {
	DefaultProfile string            `koanf:"default_profile" toml:"default_profile"`
	Profiles       []Profile         `koanf:"profiles" toml:"profiles"`
	Theme          Theme             `koanf:"theme" toml:"theme"`
	TUI            TUI               `koanf:"tui" toml:"tui"`
	QueriesPath    string            `koanf:"queries_path" toml:"queries_path"`
	Aliases        map[string]string `koanf:"aliases" toml:"aliases"`
	// Editor is the global default external-editor command for rich-text
	// edits. Per-profile Editor overrides this when set.
	// Resolution chain: $JIRA_EDITOR → profile.editor → config.editor →
	// $EDITOR → $VISUAL → "vi".
	Editor string `koanf:"editor" toml:"editor"`
}

type Theme struct {
	Name string `koanf:"name" toml:"name"`
	Path string `koanf:"path" toml:"path"`
}

type TUI struct {
	RefreshInterval int      `koanf:"refresh_interval" toml:"refresh_interval"`
	DefaultTab      string   `koanf:"default_tab" toml:"default_tab"`
	Tabs            []string `koanf:"tabs" toml:"tabs"`
}

type Profile struct {
	Name             string   `koanf:"name" toml:"name"`
	BaseURL          string   `koanf:"base_url" toml:"base_url"`
	AuthType         AuthType `koanf:"auth_type" toml:"auth_type"`
	Email            string   `koanf:"email" toml:"email"`
	Username         string   `koanf:"username" toml:"username"`
	DefaultProject   string   `koanf:"default_project" toml:"default_project"`
	DefaultIssueType string   `koanf:"default_issue_type" toml:"default_issue_type"`
	// DefaultBoard is the profile-scoped default board NAME. When set and
	// no `--board`/`--board-id` flag is supplied, the consuming command
	// resolves it against the boards cache and applies the resulting
	// scope. NOT validated at set time; the cache may not exist yet.
	// Use-time validation lives in boardScopeFromFlags. An explicit
	// `--board ""` suppresses any configured default.
	DefaultBoard       string        `koanf:"default_board" toml:"default_board"`
	RefreshInterval    int           `koanf:"refresh_interval" toml:"refresh_interval"`
	TimeoutSeconds     int           `koanf:"timeout" toml:"timeout"`
	WorkdaySeconds     int           `koanf:"workday_seconds" toml:"workday_seconds"`
	SecretBackend      SecretBackend `koanf:"secret_backend" toml:"secret_backend"`
	OnePasswordAccount string        `koanf:"onepassword_account" toml:"onepassword_account"`
	Vault              string        `koanf:"vault" toml:"vault"`
	Item               string        `koanf:"item" toml:"item"`
	MTLSCertRef        string        `koanf:"mtls_cert_ref" toml:"mtls_cert_ref"`
	MTLSKeyRef         string        `koanf:"mtls_key_ref" toml:"mtls_key_ref"`
	// TeamAccountIDs lists the account IDs of teammates whose issues count
	// as "my team" in TUI filtering. Optional.
	TeamAccountIDs []string `koanf:"team_account_ids" toml:"team_account_ids"`
	// AccountID is the user's own Jira Cloud accountId. Used by `--assignee me`
	// (CLI) and the "A" key (TUI) so assignments target the canonical user
	// identifier. Optional; falls back to email/username when blank.
	AccountID string `koanf:"account_id" toml:"account_id"`
	// ReadOnly blocks every mutation command for this profile and returns a
	// validation error (exit 3). Useful when handing credentials to an AI
	// agent. The env var JIRA_READ_ONLY (truthy: 1/true/yes/on) layers on
	// top — once set globally, every profile becomes read-only regardless
	// of its own setting.
	ReadOnly bool `koanf:"read_only" toml:"read_only"`
	// Editor is the per-profile external-editor command override.
	// When set, takes precedence over the global Config.Editor.
	// Resolution chain: $JIRA_EDITOR → profile.editor → config.editor →
	// $EDITOR → $VISUAL → "vi".
	Editor string `koanf:"editor" toml:"editor"`
}

func Defaults() Config {
	return Config{
		DefaultProfile: "default",
		Profiles: []Profile{{
			Name:            "default",
			AuthType:        AuthTypeToken,
			SecretBackend:   SecretBackendKeyring,
			RefreshInterval: DefaultRefreshIntervalSeconds,
			TimeoutSeconds:  DefaultTimeoutSeconds,
			WorkdaySeconds:  DefaultWorkdaySeconds,
		}},
		TUI: TUI{
			RefreshInterval: DefaultRefreshIntervalSeconds,
			DefaultTab:      "issues",
			Tabs:            []string{"issues", "epics", "search", "activity"},
		},
		QueriesPath: "~/.config/jira-cli/queries",
		Aliases:     map[string]string{},
	}
}

func (c *Config) Validate() error {
	if c.DefaultProfile == "" {
		c.DefaultProfile = "default"
	}
	if len(c.Profiles) == 0 {
		c.Profiles = Defaults().Profiles
	}
	if c.Aliases == nil {
		c.Aliases = map[string]string{}
	}
	seen := make(map[string]struct{}, len(c.Profiles))
	for i := range c.Profiles {
		p := &c.Profiles[i]
		if p.Name == "" {
			return errors.New("profile name is required")
		}
		if _, ok := seen[p.Name]; ok {
			return fmt.Errorf("duplicate profile %q", p.Name)
		}
		seen[p.Name] = struct{}{}
		if p.AuthType == "" {
			p.AuthType = AuthTypeToken
		}
		if !supportedAuthType(p.AuthType) {
			return fmt.Errorf("profile %q unsupported auth_type %q", p.Name, p.AuthType)
		}
		if p.SecretBackend == "" {
			p.SecretBackend = SecretBackendKeyring
		}
		if p.RefreshInterval <= 0 {
			p.RefreshInterval = DefaultRefreshIntervalSeconds
		}
		if p.TimeoutSeconds <= 0 {
			p.TimeoutSeconds = DefaultTimeoutSeconds
		}
		if p.WorkdaySeconds <= 0 {
			p.WorkdaySeconds = DefaultWorkdaySeconds
		}
		if err := ValidateBaseURL(p.BaseURL); err != nil {
			return fmt.Errorf("profile %q base_url: %w", p.Name, err)
		}
	}
	if _, ok := seen[c.DefaultProfile]; !ok {
		return fmt.Errorf("default profile %q is not defined", c.DefaultProfile)
	}
	return nil
}

func supportedAuthType(authType AuthType) bool {
	switch authType {
	case AuthTypeToken, AuthTypeBasic, AuthTypePAT, AuthTypeMTLS:
		return true
	default:
		return false
	}
}

// NormalizeBaseURL expands shorthand Jira URLs into their canonical form.
//
//   - "" → ""  (callers handle the empty case explicitly)
//   - "company"               → "https://company.atlassian.net"
//   - "company.atlassian.net" → "https://company.atlassian.net"
//   - "company.example.com"   → "https://company.example.com"  (Server/DC)
//   - "http://localhost:8080" → unchanged  (loopback)
//   - "https://x.example/"    → "https://x.example"  (trailing slash dropped)
//
// The function is intentionally permissive — it only adds the scheme and the
// `.atlassian.net` suffix when both are missing. Callers must still run
// ValidateBaseURL on the result.
func NormalizeBaseURL(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return ""
	}
	// url.Parse fills Host only when a scheme is present; assume https for
	// schemeless input so we can reason about Hostname/Port properly.
	if !strings.Contains(v, "://") {
		v = "https://" + v
	}
	u, err := url.Parse(v)
	if err != nil || u.Host == "" {
		// Unparseable; let ValidateBaseURL surface the failure verbatim.
		return strings.TrimRight(v, "/")
	}
	host := u.Hostname()
	if !strings.Contains(host, ".") && !isLocalHost(host) {
		u.Host = host + ".atlassian.net"
		if port := u.Port(); port != "" {
			u.Host += ":" + port
		}
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return u.String()
}

func ValidateBaseURL(raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("must include scheme and host")
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" && isLocalHost(u.Hostname()) {
		return nil
	}
	return fmt.Errorf("must use https, except http loopback URLs for tests")
}

func isLocalHost(host string) bool {
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

func (c Config) Profile(name string) Profile {
	if name == "" {
		name = c.DefaultProfile
	}
	for _, p := range c.Profiles {
		if p.Name == name {
			return p
		}
	}
	return Profile{Name: name}
}

func (p Profile) Redacted() string {
	parts := []string{p.Name, string(p.AuthType), string(p.SecretBackend), p.BaseURL}
	if p.Email != "" {
		parts = append(parts, p.Email)
	}
	if p.Username != "" {
		parts = append(parts, p.Username)
	}
	if p.OnePasswordAccount != "" {
		parts = append(parts, p.OnePasswordAccount)
	}
	if p.MTLSCertRef != "" {
		parts = append(parts, "mtls_cert_ref")
	}
	if p.MTLSKeyRef != "" {
		parts = append(parts, "mtls_key_ref")
	}
	return strings.Join(parts, " ")
}
