package envelope

import "github.com/matcra587/jira-cli/internal/adf"

// Typed Output structs for the local/meta operation family: auth, cache,
// adf, release notes, self-update, and the schema meta-command. Each struct
// is the single declaration of its envelope's `data` shape (see the package
// doc and outputs_issue.go for the pattern), and the registration beside it
// feeds the exhaustiveness guardrail.
//
// Several ops in this family register as Dynamic rather than a struct. The
// reason is always one of three, never a shortcut:
//   - a generated per-resource field name a struct cannot spell (the flat
//     cache primers, whose list rides under the resource's own key);
//   - a payload whose concrete type lives in a package envelope must not
//     import (cli.ReleaseNotesResult, in internal/cli) — the emission is
//     already typed at its source, so no map literal is at stake;
//   - a shape owned elsewhere: the schema meta-command emits the schema
//     document itself, auth.status is consumed by a map-asserting human
//     renderer in internal/cli, and cache.refresh rides the shared keyed
//     multi-result envelope whose typed form is a cross-family concern.

// AuthLoginOutput is `auth login`'s envelope data: the configured profile
// and how its credential was stored and verified. Credential material never
// appears here — only where the token lives and whether it authenticated.
// The account_id/display_name pair is present exactly when verified is true
// (a verified /myself), so both are pointers set together; the env-backend
// credential_env, a scoped token's cloud_id, and the login-warmed
// boards_cached appear only on the paths that produce them.
type AuthLoginOutput struct {
	Profile            string  `json:"profile"`
	AuthType           string  `json:"auth_type"`
	TokenType          string  `json:"token_type"`
	Scoped             bool    `json:"scoped"`
	SecretBackend      string  `json:"secret_backend"`
	OnePasswordAccount string  `json:"onepassword_account"`
	StoredSecret       bool    `json:"stored_secret"`
	Verified           bool    `json:"verified"`
	SkipVerify         bool    `json:"skip_verify"`
	CredentialEnv      string  `json:"credential_env,omitempty"`
	CloudID            string  `json:"cloud_id,omitempty"`
	AccountID          *string `json:"account_id,omitempty"`
	DisplayName        *string `json:"display_name,omitempty"`
	BoardsCached       *int    `json:"boards_cached,omitempty"`
}

// AuthLogoutOutput is `auth logout`'s envelope data: which profile's
// credential was targeted, its backend, and whether anything was removed
// (false on both the dry-run preview and an already-absent credential).
type AuthLogoutOutput struct {
	Profile string `json:"profile"`
	Backend string `json:"backend"`
	Removed bool   `json:"removed"`
	DryRun  bool   `json:"dry_run"`
}

// AuthSwitchOutput is `auth switch`'s envelope data: the newly active
// profile and the one it replaced. dry_run is true on the preview that
// resolves the target without writing config.
type AuthSwitchOutput struct {
	Active   string `json:"active"`
	Previous string `json:"previous"`
	DryRun   bool   `json:"dry_run"`
}

// AuthWhoamiOutput is `auth whoami`'s envelope data: the account identity
// /myself returned for the active profile. saved reports whether --save
// persisted the resolved account_id back to the file-backed profile.
type AuthWhoamiOutput struct {
	Profile      string `json:"profile"`
	AccountID    string `json:"account_id"`
	AccountType  string `json:"account_type"`
	DisplayName  string `json:"display_name"`
	EmailAddress string `json:"email_address"`
	TimeZone     string `json:"time_zone"`
	Saved        bool   `json:"saved"`
}

// CacheIssueKeysOutput is `cache issuekeys`'s envelope data: the active
// profile's recently used issue keys (newest first, never null) and their
// count. Local state, never fetched from Jira.
type CacheIssueKeysOutput struct {
	Profile   string   `json:"profile"`
	IssueKeys []string `json:"issue_keys"`
	Count     int      `json:"count"`
}

// CacheBoardsOutput is `cache boards`'s envelope data on both the fresh-prime
// and cache-hit paths: the cached board count, freshness, and the truncation
// marker a partial prime carries, plus the shared cache-state trio. primed is
// true only when this invocation fetched from Jira and wrote the file.
type CacheBoardsOutput struct {
	Profile          string `json:"profile"`
	Primed           bool   `json:"primed"`
	FromCache        bool   `json:"from_cache"`
	BoardsCount      int    `json:"boards_count"`
	FetchedAt        string `json:"fetched_at"`
	TTLSeconds       int    `json:"ttl_seconds"`
	Truncated        bool   `json:"truncated"`
	TruncatedReason  string `json:"truncated_reason"`
	CacheState       string `json:"cache_state"`
	CacheSourceState string `json:"cache_source_state"`
	CacheEmpty       bool   `json:"cache_empty"`
}

// CacheRefreshRow is one per-resource row inside `cache refresh`'s shared
// keyed-results envelope: the refresh outcome for a single cache resource.
// The op itself registers as Dynamic (the top-level keyed shape is owned by
// the shared cmdutil envelope), but the row is a fixed struct so the builder
// emits no map literal.
type CacheRefreshRow struct {
	Status     string `json:"status"`
	FromCache  bool   `json:"from_cache"`
	Count      int    `json:"count"`
	FetchedAt  string `json:"fetched_at"`
	DurationMS int64  `json:"duration_ms"`
	DryRun     bool   `json:"dry_run"`
}

// ADFRenderOutput is `adf render`'s envelope data: the lossy Markdown
// projection of an ADF document, whether the projection dropped anything,
// and the named constructs that degraded.
type ADFRenderOutput struct {
	Markdown        string   `json:"markdown"`
	Lossy           bool     `json:"lossy"`
	LossyConstructs []string `json:"lossy_constructs"`
}

// UpdateOutput is `update`'s envelope data, a union over the two install
// shapes discriminated by managed. A self-replacing channel (managed=false)
// reports latest and update_available; a managed channel (managed=true)
// reports the installer's update hint instead, and omits both. updated is
// true only after a live self-replace committed.
type UpdateOutput struct {
	Channel         string `json:"channel"`
	Current         string `json:"current"`
	Latest          string `json:"latest,omitempty"`
	UpdateAvailable *bool  `json:"update_available,omitempty"`
	Managed         bool   `json:"managed"`
	Hint            string `json:"hint,omitempty"`
	Updated         bool   `json:"updated"`
	DryRun          bool   `json:"dry_run"`
}

// AliasListOutput is `alias list`'s envelope data: the config's alias
// name→expansion map, wrapped so the payload is an object, with a count that
// spares consumers a keys-length dance.
type AliasListOutput struct {
	Aliases map[string]string `json:"aliases"`
	Count   int               `json:"count"`
}

// AliasSetOutput is `alias set`'s envelope data: the alias written and the
// expansion it replaced (empty on a fresh alias). dry_run previews without
// writing the config file.
type AliasSetOutput struct {
	Name      string `json:"name"`
	Expansion string `json:"expansion"`
	Previous  string `json:"previous"`
	DryRun    bool   `json:"dry_run"`
}

// AliasDeleteOutput is `alias delete`'s envelope data: the alias targeted and
// whether it existed (false when nothing was removed).
type AliasDeleteOutput struct {
	Name    string `json:"name"`
	Deleted bool   `json:"deleted"`
	DryRun  bool   `json:"dry_run"`
}

// AliasImportOutput is `alias import`'s envelope data: how many aliases merged,
// their names, and the per-name reason for every skipped entry.
type AliasImportOutput struct {
	Imported int               `json:"imported"`
	Aliases  []string          `json:"aliases"`
	Skipped  map[string]string `json:"skipped"`
	DryRun   bool              `json:"dry_run"`
}

// ConfigThemeOutput is `config theme`'s envelope data: the resolved theme name
// and path, whether the invocation changed anything, and the dry-run marker.
type ConfigThemeOutput struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Changed bool   `json:"changed"`
	DryRun  bool   `json:"dry_run"`
}

// ConfigInitOutput is `config init`'s envelope data: the bootstrapped profile
// and its metadata. stored_auth is always false — init writes metadata only,
// never a credential.
type ConfigInitOutput struct {
	Profile    string `json:"profile"`
	BaseURL    string `json:"base_url"`
	AuthType   string `json:"auth_type"`
	StoredAuth bool   `json:"stored_auth"`
	DryRun     bool   `json:"dry_run"`
}

// ConfigProfileEntry is one row of `config profile`: a profile name and whether
// it is the active default.
type ConfigProfileEntry struct {
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

// ConfigProfileOutput is `config profile`'s envelope data: the active profile
// and the metadata-only list of configured profiles.
type ConfigProfileOutput struct {
	ActiveProfile string               `json:"active_profile"`
	Profiles      []ConfigProfileEntry `json:"profiles"`
}

// ConfigGetOutput is `config get`'s envelope data: the requested key and its
// value. Value is the config value in its native JSON type (string, integer,
// boolean, …), so it is an opaque `any` the schema leaves unconstrained.
type ConfigGetOutput struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}

// ConfigSetOutput is `config set`'s envelope data: the key and new value, plus
// the value the change replaced (native JSON type, opaque). dry_run previews
// without writing.
type ConfigSetOutput struct {
	Key           string `json:"key"`
	Value         string `json:"value"`
	PreviousValue any    `json:"previous_value"`
	DryRun        bool   `json:"dry_run"`
}

// CacheClearOutput is `cache clear`'s envelope data. removed is an honest union
// the type system cannot narrow: an integer file count on a whole-cache clear,
// a boolean did-it-exist on a single-resource clear — declared via the register
// doc override. resource is present only when one resource was targeted.
type CacheClearOutput struct {
	Profile  string `json:"profile"`
	Resource string `json:"resource,omitempty"`
	Removed  any    `json:"removed"`
	DryRun   bool   `json:"dry_run"`
}

var (
	_ = register("auth.login", AuthLoginOutput{}, nil)
	_ = register("auth.logout", AuthLogoutOutput{}, nil)
	_ = register("auth.switch", AuthSwitchOutput{}, nil)
	_ = register("auth.whoami", AuthWhoamiOutput{}, nil)
	_ = register("cache.issuekeys", CacheIssueKeysOutput{}, nil)
	_ = register("cache.boards", CacheBoardsOutput{}, nil)
	_ = register("adf.render", ADFRenderOutput{}, nil)
	_ = register("update", UpdateOutput{}, nil)
	_ = register("alias.list", AliasListOutput{}, nil)
	_ = register("alias.set", AliasSetOutput{}, nil)
	_ = register("alias.delete", AliasDeleteOutput{}, nil)
	_ = register("alias.import", AliasImportOutput{}, nil)
	_ = register("config.theme", ConfigThemeOutput{}, nil)
	_ = register("config.init", ConfigInitOutput{}, nil)
	_ = register("config.profile", ConfigProfileOutput{}, nil)
	_ = register("config.get", ConfigGetOutput{}, nil)
	_ = register("config.set", ConfigSetOutput{}, nil)

	// cache.clear's removed field is an honest int|boolean union (file count
	// vs did-it-exist) a Go type cannot narrow; the doc override supplies the
	// two-type declaration while the rest of the shape derives from the struct.
	_ = register("cache.clear", CacheClearOutput{}, map[string]any{
		"properties": map[string]any{
			"removed": map[string]any{"type": []string{"integer", "boolean"}},
		},
	})

	// auth.status is consumed by a human renderer in internal/cli that
	// asserts map[string]any directly (not a JSON round-trip), so its data
	// cannot become a struct without editing that renderer; the shape is
	// also deeply dynamic (a per-permission grants map, conditional probe
	// blocks).
	_ = register("auth.status", Dynamic{Reason: "nested live-probe result whose internal/cli human renderer consumes map[string]any directly; per-permission grants and conditional probe blocks are inherently dynamic"}, nil)

	// The schema meta-command emits the schema document itself, built in
	// internal/cli/schema; there is no data payload for a struct to model.
	_ = register("schema", Dynamic{Reason: "meta-command emitting the schema document itself"}, nil)

	// adf.convert emits a bare adf.Document. envelope may import the wire-type
	// package internal/adf (see doc.go), so the output is the real type; the
	// self-referential Node.Content collapses to an opaque object via
	// schemaOfType's cycle guard.
	_ = register("adf.convert", adf.Document{}, nil)

	// release.notes emits cli.ReleaseNotesResult ([]changelog.Release);
	// those types live in internal/cli and the module root, neither
	// importable by this leaf package.
	_ = register("release.notes", Dynamic{Reason: "cli.ReleaseNotesResult carrying []changelog.Release; those types live in internal/cli and the module root, which this leaf package must not import"}, nil)

	// cache.refresh rides the shared keyed multi-result envelope
	// (cmdutil.KeyedResultsData with cli.Error rows); its typed form is a
	// cross-family concern this leaf package cannot express, so the shared
	// keyed_results schema stands. The per-row payload is CacheRefreshRow.
	_ = register("cache.refresh", Dynamic{Reason: "keyed multi-resource results emitted through the shared cmdutil.KeyedResultsData envelope; the row is CacheRefreshRow but the top-level typed shape is a cross-family concern"}, nil)

	// The flat-list cache primers each emit their list under a generated
	// per-resource field name (registry Resource.Key()), which a fixed
	// struct cannot spell; boards is the struct-modeled exception above.
	_ = registerDynamicCachePrimers()
)

// registerDynamicCachePrimers registers each flat-list cache primer as
// Dynamic. The names are listed explicitly (not derived from the resource
// registry) because this leaf package must not import it; a primer added to
// the registry without a matching entry here fails the exhaustiveness
// guardrail, which is the intended tripwire.
func registerDynamicCachePrimers() any {
	const reason = "flat-list cache primer emitting its list under a generated per-resource field name (registry Resource.Key()) a struct cannot spell"
	for _, op := range []string{
		"cache.epics",
		"cache.fields",
		"cache.issuetypes",
		"cache.labels",
		"cache.linktypes",
		"cache.priorities",
		"cache.projects",
		"cache.resolutions",
		"cache.statuses",
	} {
		register(op, Dynamic{Reason: reason}, nil)
	}
	return nil
}
