package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	xfilepath "github.com/gechr/x/filepath"
	xos "github.com/gechr/x/os"
	"github.com/gechr/x/shell"
	"github.com/go-viper/mapstructure/v2"
	koanftoml "github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

type loadOptions struct {
	path string
}

type Option func(*loadOptions)

func WithPath(path string) Option {
	return func(o *loadOptions) {
		o.path = path
	}
}

// Load reads configuration for read-only and resolution paths without
// ever creating a file. When the resolved path exists it is parsed; when
// an explicit --config path is missing it returns an error so a path typo
// cannot be masked; when the default path is missing it returns validated
// defaults without touching disk.
//
// Commands that intend to persist config must use LoadOrInit, which
// bootstraps a default file when none exists.
func Load(opts ...Option) (*Config, error) {
	var lo loadOptions
	for _, opt := range opts {
		opt(&lo)
	}
	explicit := lo.path != ""
	path := lo.path
	if path == "" {
		path = DefaultPath()
	}
	if _, err := os.Stat(path); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if explicit {
			return nil, fmt.Errorf("config file %q does not exist", path)
		}
		// Default path missing: serve validated defaults, write nothing.
		return loadFromKoanf(nil, true)
	}
	return loadFromKoanf(&path, true)
}

// LoadOrInit loads configuration, creating a default config file when
// none exists. Use this only for explicit init and config-write commands.
//
// LoadOrInit returns the persisted, file-backed config: it deliberately
// does NOT apply JIRA_* env overlays. Read-modify-write commands Save the
// result, and persisting a transient env overlay into TOML would corrupt
// the user's stored config. Read-only callers that want the effective
// runtime view (file plus env) must use Load instead.
func LoadOrInit(opts ...Option) (*Config, error) {
	var lo loadOptions
	for _, opt := range opts {
		opt(&lo)
	}
	path := lo.path
	if path == "" {
		path = DefaultPath()
	}
	if err := ensureConfig(path); err != nil {
		return nil, err
	}
	return loadFromKoanf(&path, false)
}

// loadFromKoanf builds a Config from defaults plus an optional TOML file
// and validates it. A nil path means defaults-only. When applyEnvOverlay
// is true the JIRA_* environment overlay is layered onto the effective
// runtime view; when false the persisted, file-backed config is returned
// unchanged so callers can Save it without leaking env values.
func loadFromKoanf(path *string, applyEnvOverlay bool) (*Config, error) {
	k := koanf.New(".")
	if err := k.Load(confmap.Provider(defaultMap(), "."), nil); err != nil {
		return nil, err
	}
	if path != nil {
		if err := k.Load(file.Provider(*path), koanftoml.Parser()); err != nil {
			return nil, fmt.Errorf("loading config file: %w", err)
		}
	}

	var out Config
	if err := strictUnmarshal(k, &out); err != nil {
		return nil, fmt.Errorf("decoding config: %w", err)
	}
	if applyEnvOverlay {
		applyEnv(&out)
	}
	if err := out.Validate(); err != nil {
		return nil, err
	}
	return &out, nil
}

// strictUnmarshal decodes the koanf map into cfg, rejecting any key that
// does not map to a known config field. An unknown key would otherwise be
// dropped silently on the next Save, so the loader fails loudly instead
// and names the offending key. The decode hooks mirror koanf's defaults.
func strictUnmarshal(k *koanf.Koanf, cfg *Config) error {
	return k.UnmarshalWithConf("", cfg, koanf.UnmarshalConf{
		Tag: "koanf",
		DecoderConfig: &mapstructure.DecoderConfig{
			DecodeHook: mapstructure.ComposeDecodeHookFunc(
				mapstructure.StringToTimeDurationHookFunc(),
				mapstructure.TextUnmarshallerHookFunc(),
			),
			WeaklyTypedInput: true,
			ErrorUnused:      true,
			Result:           cfg,
		},
	})
}

// Save atomically writes cfg to path using a temp-file + rename idiom, so
// concurrent or interrupted writes never leave a half-truncated config file.
// The temp file is created in the same directory as path so that os.Rename is
// guaranteed to be atomic on the same filesystem.
func Save(path string, cfg *Config) error {
	if path == "" {
		path = DefaultPath()
	}
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		return err
	}
	return atomicWrite(path, buf.Bytes())
}

// atomicWrite writes data to path via a temp file and rename in the same
// directory, so a reader never observes a partial config. It first resolves
// path through any symlink (writeThroughPath) so the rename rewrites the link's
// target rather than replacing the link, and creates the resolved target's
// directory if it does not exist yet. Every config write — initial creation and
// later updates alike — goes through here, so symlink and atomicity guarantees
// hold uniformly.
func atomicWrite(path string, data []byte) error {
	path = writeThroughPath(path)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if !xos.IsWritableDir(dir) {
		return fmt.Errorf("config directory %q is not writable", dir)
	}
	tmp, err := os.CreateTemp(dir, ".atomic-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		// Best-effort cleanup when rename below fails.
		_ = os.Remove(tmpName)
	}()
	if err := os.Chmod(tmpName, 0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// writeThroughPath resolves path so an atomic temp-file+rename rewrites a
// symlinked config file's target rather than replacing the link. A live link
// resolves via EvalSymlinks; a dangling link (its target not created yet) is
// followed one level via Readlink so the first write lands on the intended
// target. A path that is not a symlink — including a genuinely new file — is
// returned unchanged.
func writeThroughPath(path string) string {
	if resolved, err := xfilepath.Resolve(path); err == nil {
		return resolved
	}
	// Resolve failed — e.g. a dangling link whose target is not created yet.
	// If path is itself a symlink, follow it one level so the write lands on
	// the declared target rather than clobbering the link.
	if link, err := xos.IsSymlink(path); err != nil || !link {
		return path
	}
	dest, err := os.Readlink(path)
	if err != nil {
		return path
	}
	if !filepath.IsAbs(dest) {
		dest = filepath.Join(filepath.Dir(path), dest)
	}
	return dest
}

// Get returns the string form of a configuration value addressed by its
// user-facing dot-notation key. Profile-scoped keys take the form
// `profiles.<name>.<field>`. Returns ok=false for unknown keys or missing
// profiles.
func (c Config) Get(key string) (string, bool) {
	switch key {
	case "default_profile":
		return c.DefaultProfile, true
	case "queries_path":
		return c.QueriesPath, true
	case "editor":
		return c.Editor, true
	case "theme.name":
		return c.Theme.Name, true
	case "theme.path":
		return c.Theme.Path, true
	case "tui.refresh_interval":
		return strconv.Itoa(c.TUI.RefreshInterval), true
	case "tui.default_tab":
		return c.TUI.DefaultTab, true
	}
	if strings.HasPrefix(key, profileFieldPrefix) {
		return c.getProfileField(key)
	}
	return "", false
}

// Set assigns a configuration value addressed by its user-facing
// dot-notation key. Profile-scoped keys take the form
// `profiles.<name>.<field>`. Returns an error describing why the
// assignment was rejected (unknown key, missing profile, invalid enum,
// unparsable int/bool).
func (c *Config) Set(key, value string) error {
	switch key {
	case "default_profile":
		c.DefaultProfile = value
		return nil
	case "queries_path":
		c.QueriesPath = value
		return nil
	case "editor":
		c.Editor = value
		return nil
	case "theme.name":
		// Validate on the way in: an unrecognized name set here is a user typo
		// to reject, but config load tolerates a stale name (it falls back to
		// dark) so an upstream rename never blocks a command.
		if err := ValidateThemeName(value); err != nil {
			return err
		}
		c.Theme.Name = value
		return nil
	case "theme.path":
		c.Theme.Path = value
		return nil
	case "tui.refresh_interval":
		n, err := strconv.Atoi(value)
		if err != nil || n <= 0 {
			return fmt.Errorf("tui.refresh_interval must be a positive integer, got %q", value)
		}
		c.TUI.RefreshInterval = n
		return nil
	case "tui.default_tab":
		switch value {
		case "issues", "epics", "search", "activity":
			c.TUI.DefaultTab = value
			return nil
		}
		return fmt.Errorf("invalid tui.default_tab %q (valid: issues, epics, search, activity)", value)
	}
	if strings.HasPrefix(key, profileFieldPrefix) {
		return c.setProfileField(key, value)
	}
	return fmt.Errorf("unknown config key %q", key)
}

func DefaultPath() string {
	root, err := shell.XDGConfigHome()
	if err != nil || root == "" {
		root = ".config"
	}
	return filepath.Join(root, "jira-cli", "config.toml")
}

func ensureConfig(path string) error {
	// Resolve through any symlink so existence is tested — and the seed below
	// is written — against the link's real target, never the link itself.
	target := writeThroughPath(path)
	exists, err := xos.Exists(target)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	// Seed the initial config by persisting the canonical Defaults() through
	// Save — the one config writer. This keeps a single source of truth for the
	// defaults (the same Defaults() that feeds the in-memory load layer) instead
	// of a hand-maintained TOML template that could drift, and inherits Save's
	// atomic, symlink-aware write (a symlinked config.toml is followed, not
	// clobbered, and a missing target directory is created).
	d := Defaults()
	return Save(path, &d)
}

func defaultMap() map[string]any {
	d := Defaults()
	return map[string]any{
		"default_profile": d.DefaultProfile,
		"queries_path":    d.QueriesPath,
		"aliases":         d.Aliases,
		"tui": map[string]any{
			"refresh_interval": d.TUI.RefreshInterval,
			"default_tab":      d.TUI.DefaultTab,
			"tabs":             d.TUI.Tabs,
		},
		"profiles": []map[string]any{{
			"name":             "default",
			"auth_type":        string(AuthTypeToken),
			"secret_backend":   string(SecretBackendKeyring),
			"refresh_interval": DefaultRefreshIntervalSeconds,
			"timeout":          DefaultTimeoutSeconds,
			"workday_seconds":  DefaultWorkdaySeconds,
		}},
	}
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("JIRA_DEFAULT_PROFILE"); v != "" {
		cfg.DefaultProfile = v
		if !profileExists(cfg.Profiles, v) {
			cfg.Profiles = append(cfg.Profiles, Profile{Name: v})
		}
	}
	for _, env := range os.Environ() {
		key, val, ok := strings.Cut(env, "=")
		if !ok || !strings.HasPrefix(key, "JIRA_PROFILE_") {
			continue
		}
		rest := strings.TrimPrefix(key, "JIRA_PROFILE_")
		idx := strings.LastIndex(rest, "_")
		if idx < 0 {
			continue
		}
		name, field := rest[:idx], rest[idx+1:]
		if strings.HasSuffix(rest, "_BASE_URL") {
			name, field = strings.TrimSuffix(rest, "_BASE_URL"), "BASE_URL"
		}
		if strings.HasSuffix(rest, "_AUTH_TYPE") {
			name, field = strings.TrimSuffix(rest, "_AUTH_TYPE"), "AUTH_TYPE"
		}
		if strings.HasSuffix(rest, "_REFRESH_INTERVAL") {
			name, field = strings.TrimSuffix(rest, "_REFRESH_INTERVAL"), "REFRESH_INTERVAL"
		}
		if strings.HasSuffix(rest, "_SECRET_BACKEND") {
			name, field = strings.TrimSuffix(rest, "_SECRET_BACKEND"), "SECRET_BACKEND"
		}
		if strings.HasSuffix(rest, "_WORKDAY_SECONDS") {
			name, field = strings.TrimSuffix(rest, "_WORKDAY_SECONDS"), "WORKDAY_SECONDS"
		}
		if strings.HasSuffix(rest, "_ONEPASSWORD_ACCOUNT") {
			name, field = strings.TrimSuffix(rest, "_ONEPASSWORD_ACCOUNT"), "ONEPASSWORD_ACCOUNT"
		}
		if strings.HasSuffix(rest, "_DEFAULT_PROJECT") {
			name, field = strings.TrimSuffix(rest, "_DEFAULT_PROJECT"), "DEFAULT_PROJECT"
		}
		if strings.HasSuffix(rest, "_DEFAULT_ISSUE_TYPE") {
			name, field = strings.TrimSuffix(rest, "_DEFAULT_ISSUE_TYPE"), "DEFAULT_ISSUE_TYPE"
		}
		if strings.HasSuffix(rest, "_DEFAULT_BOARD") {
			name, field = strings.TrimSuffix(rest, "_DEFAULT_BOARD"), "DEFAULT_BOARD"
		}
		if strings.HasSuffix(rest, "_MTLS_CERT_REF") {
			name, field = strings.TrimSuffix(rest, "_MTLS_CERT_REF"), "MTLS_CERT_REF"
		}
		if strings.HasSuffix(rest, "_MTLS_KEY_REF") {
			name, field = strings.TrimSuffix(rest, "_MTLS_KEY_REF"), "MTLS_KEY_REF"
		}
		name = strings.ToLower(strings.ReplaceAll(name, "_", "-"))
		setProfileEnv(cfg, name, field, val)
	}
}

func profileExists(profiles []Profile, name string) bool {
	for _, p := range profiles {
		if p.Name == name {
			return true
		}
	}
	return false
}

func setProfileEnv(cfg *Config, name, field, value string) {
	idx := -1
	for i := range cfg.Profiles {
		if cfg.Profiles[i].Name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		cfg.Profiles = append(cfg.Profiles, Profile{Name: name})
		idx = len(cfg.Profiles) - 1
	}
	p := &cfg.Profiles[idx]
	switch field {
	case "BASE_URL":
		p.BaseURL = value
	case "AUTH_TYPE":
		p.AuthType = AuthType(value)
	case "EMAIL":
		p.Email = value
	case "DEFAULT_PROJECT":
		p.DefaultProject = value
	case "DEFAULT_ISSUE_TYPE":
		p.DefaultIssueType = value
	case "DEFAULT_BOARD":
		p.DefaultBoard = value
	case "SECRET_BACKEND":
		p.SecretBackend = SecretBackend(value)
	case "ONEPASSWORD_ACCOUNT":
		p.OnePasswordAccount = value
	case "VAULT":
		p.Vault = value
	case "ITEM":
		p.Item = value
	case "REFRESH_INTERVAL":
		if n, err := strconv.Atoi(value); err == nil {
			p.RefreshInterval = n
		}
	case "TIMEOUT":
		if n, err := strconv.Atoi(value); err == nil {
			p.TimeoutSeconds = n
		}
	case "WORKDAY_SECONDS":
		if n, err := strconv.Atoi(value); err == nil {
			p.WorkdaySeconds = n
		}
	}
}
