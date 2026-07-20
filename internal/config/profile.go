package config

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	xstrings "github.com/gechr/x/strings"
	"github.com/matcra587/jira-cli/internal/errtax"
)

const (
	// DefaultRefreshIntervalSeconds is the per-profile polling default (watch
	// commands and the like). The dashboard's cadence is the separate
	// DefaultTUIRefreshSeconds below.
	DefaultRefreshIntervalSeconds = 30
	// DefaultTUIRefreshSeconds is the dashboard's auto-refresh cadence; the
	// TUI refetches every section this often unless the user is mid-action.
	DefaultTUIRefreshSeconds = 60
	// DefaultTimeoutSeconds is the per-request HTTP timeout applied to a
	// profile that does not set its own.
	DefaultTimeoutSeconds = 30
	// DefaultWorkdaySeconds is the working day used to convert worklog
	// durations (8h), so "1d" logged means one workday, not 24 hours.
	DefaultWorkdaySeconds = 28800
)

// AuthType is a profile's authentication mechanism. Only AuthTypeToken is
// supported; an unsupported value is rejected at load, never stored as a
// fake-authenticated profile.
type AuthType string

// AuthTypeToken authenticates with an Atlassian API token over HTTP Basic. It
// is the only supported AuthType.
const AuthTypeToken AuthType = "token"

// AtlassianGatewayBaseURL is the Atlassian API gateway root. Scoped (granular)
// API tokens are rejected by the site host and only work when REST calls are
// addressed through this gateway with the site's cloudId in the path. Classic
// API tokens hit the site host directly and never touch the gateway.
const AtlassianGatewayBaseURL = "https://api.atlassian.com"

// GatewayBaseURL builds the REST base URL a scoped-token profile must target:
// https://api.atlassian.com/ex/jira/<cloudID>/ . The trailing slash is load
// bearing — the Jira client resolves its relative paths (rest/api/3/...)
// against this base, and a missing slash would drop the /ex/jira/<cloudID>
// segment.
func GatewayBaseURL(cloudID string) string {
	return AtlassianGatewayBaseURL + "/ex/jira/" + strings.TrimSpace(cloudID) + "/"
}

// ValidateCloudID checks that a cloud_id is a plausible, URL-path-safe
// Atlassian cloudId (a UUID in practice). It is deliberately lenient about the
// exact shape — Atlassian has never promised the UUID format — but rejects
// blanks and anything that could break the gateway path (spaces, slashes,
// control bytes).
func ValidateCloudID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("cloud_id is required for a scoped (granular) API token")
	}
	for _, r := range id {
		switch {
		case xstrings.IsAlphanumericChar(r), r == '-', r == '_', r == '.':
		default:
			return fmt.Errorf("cloud_id %q contains an invalid character %q; expected a UUID-like identifier", id, string(r))
		}
	}
	return nil
}

// SecretBackend names where a profile's credential is stored. It is metadata
// only: the profile records which backend to consult, and the matching store
// type (KeyringStore, OnePasswordStore, EnvCredentialStore) holds the secret.
type SecretBackend string

const (
	// SecretBackendKeyring stores the credential in the OS keyring under a
	// "<host>/<profile>" entry. It is the default.
	SecretBackendKeyring SecretBackend = "keyring"
	// SecretBackendOnePassword stores the credential in a 1Password item. It
	// is CGO-gated and absent from no-CGO release archives.
	SecretBackendOnePassword SecretBackend = "1password"
	// SecretBackendEnv reads the credential from the profile's JIRA_TOKEN_*
	// environment variable at run time and stores nothing. It exists for
	// environments with no usable OS keyring (WSL, headless Linux, containers)
	// and for secret injectors like `op run` — the same variable every backend
	// honors as an override becomes the profile's sole credential source.
	SecretBackendEnv SecretBackend = "env"
)

// Config is the fully decoded configuration: the selected default profile, the
// defined profiles, and the global theme, TUI, alias, and editor settings. It
// is the in-memory shape both the koanf loader and the TOML file map onto.
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

// Theme selects the color palette: Name is a clib preset (or "auto"), Path is
// an optional custom theme file. Load never validates Name — an unrecognized
// name degrades to the dark fallback at render rather than blocking commands.
type Theme struct {
	Name string `koanf:"name" toml:"name"`
	Path string `koanf:"path" toml:"path"`
}

// TUI holds the alpha dashboard's settings — refresh cadence, tabs, preview
// layout, key rebindings, and lenses. These affect only `jira tui`.
type TUI struct {
	RefreshInterval int      `koanf:"refresh_interval" toml:"refresh_interval"`
	DefaultTab      string   `koanf:"default_tab" toml:"default_tab"`
	Tabs            []string `koanf:"tabs" toml:"tabs"`
	// Preview docks the issue sidebar: right, left, bottom, hidden, or auto
	// (right on wide terminals, bottom on narrow). Applies on hot-reload too.
	Preview string `koanf:"preview" toml:"preview"`
	// Sections are user-defined dashboard tabs: each entry is a
	// saved JQL query that becomes its own tab with its own result count.
	Sections []TUISection `koanf:"sections" toml:"sections"`
	// PreviewSize is the preview pane's share of the split as a percent
	// (20–80; values outside clamp, 0/absent keeps the 50% default).
	PreviewSize int `koanf:"preview_size" toml:"preview_size"`
	// Keys rebinds TUI actions: action name → key list (the first key shows
	// in help). Unknown action names surface as a footer error rather than
	// failing the load. Applies on hot-reload; removing an entry restores
	// the default binding.
	Keys map[string][]string `koanf:"keys" toml:"keys"`
	// Lenses replace the issues tab's built-in quick-filters (Mine/Sprint/
	// Updated/Reported) with custom titled JQL queries. Absent or empty
	// keeps the built-ins.
	Lenses []TUISection `koanf:"lenses" toml:"lenses"`
	// DefaultLens names the lens (by title, case-insensitive) the issues tab
	// lands on. Absent or unmatched falls back to the first lens.
	DefaultLens string `koanf:"default_lens" toml:"default_lens"`
	// Icons picks the glyph set: "nerd" (Nerd Font codepoints), "unicode"
	// (portable, the default look), or "auto" (Nerd only when the NERD_FONT
	// environment convention says the font is installed). Applies on
	// hot-reload.
	Icons string `koanf:"icons" toml:"icons"`
}

// TUISection is one configured dashboard tab: a title and the JQL it runs.
type TUISection struct {
	Title string `koanf:"title" toml:"title"`
	JQL   string `koanf:"jql" toml:"jql"`
}

// Profile is one Jira site's connection metadata: its base URL, account email,
// auth type, and which secret backend holds the token. It carries no credential
// — the secret lives in the backend named by SecretBackend. A scoped
// (granular) token profile also stores a CloudID for gateway routing.
type Profile struct {
	Name             string   `koanf:"name" toml:"name"`
	BaseURL          string   `koanf:"base_url" toml:"base_url"`
	AuthType         AuthType `koanf:"auth_type" toml:"auth_type"`
	Email            string   `koanf:"email" toml:"email"`
	DefaultProject   string   `koanf:"default_project" toml:"default_project"`
	DefaultIssueType string   `koanf:"default_issue_type" toml:"default_issue_type"`
	// DefaultBoard is the profile-scoped default board NAME. When set and
	// no `--board`/`--board-id` flag is supplied, the consuming command
	// resolves it against the boards cache and applies the resulting
	// scope. NOT validated at set time; the cache may not exist yet.
	// Use-time validation lives in boardscope.FromFlags. An explicit
	// `--board ""` suppresses any configured default.
	DefaultBoard    string        `koanf:"default_board" toml:"default_board"`
	RefreshInterval int           `koanf:"refresh_interval" toml:"refresh_interval"`
	TimeoutSeconds  int           `koanf:"timeout" toml:"timeout"`
	WorkdaySeconds  int           `koanf:"workday_seconds" toml:"workday_seconds"`
	SecretBackend   SecretBackend `koanf:"secret_backend" toml:"secret_backend"`
	// CloudID, when set, marks this profile as using a scoped (granular)
	// Atlassian API token. The auth mechanism is unchanged — HTTP Basic with
	// the account email and token — but REST calls must route through the
	// Atlassian gateway (https://api.atlassian.com/ex/jira/<cloud_id>/...)
	// because the site host rejects scoped tokens. BaseURL is still stored as
	// the site URL for cloudId discovery, display, and browser links; the
	// effective request base URL is derived by ClientBaseURL. Empty means a
	// classic API token addressed directly at the site.
	CloudID            string `koanf:"cloud_id" toml:"cloud_id"`
	OnePasswordAccount string `koanf:"onepassword_account" toml:"onepassword_account"`
	Vault              string `koanf:"vault" toml:"vault"`
	Item               string `koanf:"item" toml:"item"`
	// TeamAccountIDs lists the account IDs of teammates whose issues count
	// as "my team" in TUI filtering. Optional.
	TeamAccountIDs []string `koanf:"team_account_ids" toml:"team_account_ids"`
	// AccountID is the user's own Jira Cloud accountId. Used by `--assignee me`
	// (CLI) and the "A" key (TUI) so assignments target the canonical user
	// identifier. Optional; falls back to email when blank.
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

// Defaults is the canonical zero-config Config: one keyring-backed "default"
// profile and the built-in TUI layout. It is the single source of truth for
// defaults, feeding both the koanf default layer and the file seeded by
// LoadOrInit, so the two can never drift.
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
			RefreshInterval: DefaultTUIRefreshSeconds,
			DefaultTab:      "issues",
			Tabs:            []string{"issues", "epics", "search", "activity"},
		},
		QueriesPath: DefaultQueriesPath(),
		Aliases:     map[string]string{},
	}
}

// Validate normalizes and checks a decoded Config, mutating it in place: it
// fills blank per-profile fields with their defaults, rejects duplicate or
// unnamed profiles, unsupported auth types and secret backends, malformed base
// URLs and cloud IDs, and a default profile that names nothing. theme.name is
// deliberately not validated here — it is cosmetic and must not make config
// load (and thus every command) fail on an upstream rename.
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
		if !supportedSecretBackend(p.SecretBackend) {
			return fmt.Errorf("profile %q unsupported secret_backend %q", p.Name, p.SecretBackend)
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
		if p.CloudID != "" {
			if err := ValidateCloudID(p.CloudID); err != nil {
				return fmt.Errorf("profile %q cloud_id: %w", p.Name, err)
			}
		}
	}
	if _, ok := seen[c.DefaultProfile]; !ok {
		return fmt.Errorf("default profile %q is not defined", c.DefaultProfile)
	}
	// theme.name is deliberately not validated here. It is cosmetic and resolves
	// with a dark fallback when unrecognized, so an upstream rename must not make
	// config load — and therefore every command — fail. The write path
	// (`config theme --name`) validates before saving.
	return nil
}

func supportedAuthType(authType AuthType) bool {
	return authType == AuthTypeToken
}

func supportedSecretBackend(backend SecretBackend) bool {
	switch backend {
	case SecretBackendKeyring, SecretBackendOnePassword, SecretBackendEnv:
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

// ValidateBaseURL accepts an empty URL (an unconfigured profile), any https
// URL, and http only for loopback hosts (localhost/127.0.0.1/::1, for tests).
// It rejects a plaintext http URL to a real host, since a token would travel
// unencrypted. Run NormalizeBaseURL first to expand shorthand.
func ValidateBaseURL(raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if xstrings.AnyEmpty(u.Scheme, u.Host) {
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

// ErrProfileNotDefined is returned when a requested profile, or the
// configured default profile, names no profile defined in the config.
// Callers test for it with errors.Is.
var ErrProfileNotDefined = errors.New("profile is not defined")

// ProfileNotDefinedError carries the name of the profile that could not
// be resolved. It wraps ErrProfileNotDefined, so callers can use either
// errors.Is(err, ErrProfileNotDefined) or errors.As to recover the name.
type ProfileNotDefinedError struct {
	Name string
}

func (e ProfileNotDefinedError) Error() string {
	if e.Name == "" {
		return "no default profile is configured"
	}
	return fmt.Sprintf("profile %q is not defined", e.Name)
}

func (ProfileNotDefinedError) Unwrap() error { return ErrProfileNotDefined }

// Code classifies the failure under profile_not_defined: a typoed or
// unprovisioned --profile fails closed instead of degrading into
// fabricated empty results.
func (ProfileNotDefinedError) Code() errtax.Code { return errtax.CodeProfileNotDefined }

// ProfileIncompleteError is returned when a requested profile exists but
// cannot serve live commands because it has no base URL (and no cloud_id to
// derive one). Callers recover the name with errors.As.
type ProfileIncompleteError struct {
	Name string
}

func (e ProfileIncompleteError) Error() string {
	return fmt.Sprintf("profile %q has no base URL configured", e.Name)
}

// Code classifies the failure under profile_incomplete: the profile
// cannot serve live commands until it has a base URL.
func (ProfileIncompleteError) Code() errtax.Code { return errtax.CodeProfileIncomplete }

// The value forms pin the intended value receivers.
var (
	_ errtax.Coded = ProfileNotDefinedError{}
	_ errtax.Coded = ProfileIncompleteError{}
)

// Profile resolves a profile by name, falling back to the default profile
// when name is empty. A name that matches no defined profile yields a
// synthetic Profile carrying only that name.
//
// This fabricating behavior is convenient for read-only display paths but
// unsafe for credential-admin or destructive commands. Those callers must
// use ResolveProfile, which refuses an unknown name instead of inventing
// one.
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

// ResolveProfile resolves the profile a command should act on. An empty
// name resolves the configured default profile; a non-empty name must
// match a defined profile exactly. An unknown name, or an empty name with
// no configured default, returns a ProfileNotDefinedError wrapping
// ErrProfileNotDefined. It never fabricates a synthetic profile, so a
// typoed --profile cannot silently target the wrong credential namespace
// or cache.
func (c Config) ResolveProfile(name string) (Profile, error) {
	if name == "" {
		name = c.DefaultProfile
	}
	if name == "" {
		return Profile{}, ProfileNotDefinedError{}
	}
	for _, p := range c.Profiles {
		if p.Name == name {
			return p, nil
		}
	}
	return Profile{}, ProfileNotDefinedError{Name: name}
}

// ClientBaseURL is the REST base URL the Jira client must target for this
// profile. A classic API-token profile addresses the site directly; a scoped
// (granular) token profile carries a CloudID and must route through the
// Atlassian gateway, the only base URL that accepts scoped tokens. This is the
// single chokepoint that turns a cloud_id into gateway routing — every client
// constructor and credential-verification path funnels its base URL through
// here, so classic and scoped profiles diverge in exactly one place.
func (p Profile) ClientBaseURL() string {
	if p.CloudID != "" {
		return GatewayBaseURL(p.CloudID)
	}
	return p.BaseURL
}

// Scoped reports whether this profile authenticates with a scoped (granular)
// API token, i.e. one routed through the Atlassian gateway via a cloud_id.
func (p Profile) Scoped() bool {
	return p.CloudID != ""
}

// Redacted is a space-joined, secret-free summary of the profile — name, auth
// type, backend, base URL, and (when set) email and 1Password account — safe to
// log or print. It carries no credential because a Profile never holds one.
func (p Profile) Redacted() string {
	parts := []string{p.Name, string(p.AuthType), string(p.SecretBackend), p.BaseURL}
	if p.Email != "" {
		parts = append(parts, p.Email)
	}
	if p.OnePasswordAccount != "" {
		parts = append(parts, p.OnePasswordAccount)
	}
	return strings.Join(parts, " ")
}
