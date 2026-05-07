package pipeline

import (
	"errors"

	"github.com/matcra587/jira-cli/internal/cli/adfmode"
	"github.com/matcra587/jira-cli/pkg/adf"
)

// ErrSchemaUnknown is returned by a schema fetcher when the project /
// issue-type / screen schema cannot be resolved. Callers respond by
// attempting one refresh, then either strict-aborting or falling back to
// the known-safe field set.
var ErrSchemaUnknown = errors.New("pipeline: project/issue-type schema unknown")

// SchemaFetcher returns the active screen schema. It MAY return
// ErrSchemaUnknown to indicate a stale/missing cache.
type SchemaFetcher func() (ScreenSchema, error)

// ResolveScreenSchemaStrict fetches the schema, refreshes once if
// unknown, then either returns the resolved schema or aborts (strict
// path). The bool reports whether a refresh was attempted.
func ResolveScreenSchemaStrict(fetch SchemaFetcher) (ScreenSchema, bool, error) {
	schema, err := fetch()
	if err == nil {
		return schema, false, nil
	}
	if !errors.Is(err, ErrSchemaUnknown) {
		return ScreenSchema{}, false, err
	}
	// Refresh once.
	schema, err = fetch()
	if err == nil {
		return schema, true, nil
	}
	return ScreenSchema{}, true, err
}

// ResolveAndApplySchema combines schema resolution + best-effort
// fallback. In best-effort mode, an unresolved schema after one refresh
// triggers known-safe stripping with warnings. In strict mode, the same
// condition aborts via ResolveScreenSchemaStrict's error.
func ResolveAndApplySchema(fields map[string]any, fetch SchemaFetcher, mode adfmode.Mode) (map[string]any, []adf.Warning, error) {
	schema, _, err := ResolveScreenSchemaStrict(fetch)
	if err == nil {
		return ValidateFields(fields, schema, mode)
	}
	if !errors.Is(err, ErrSchemaUnknown) {
		return nil, nil, err
	}
	if mode == adfmode.ModeStrict {
		return nil, nil, err
	}
	out, warnings := ApplyKnownSafeFallback(fields)
	return out, warnings, nil
}
