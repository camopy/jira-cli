package pipeline

import (
	"fmt"

	"github.com/matcra587/jira-cli/pkg/adf"
)

// knownSafeFields is the exact whitelist used as the best-effort
// fallback when project/issue-type schema is unknown after one refresh.
// Adding a field here is a spec amendment.
var knownSafeFields = []string{
	"project",
	"issuetype",
	"summary",
	"description", // assumed already ADF-validated by stage 2
	"labels",
	"priority",
	"assignee",
	"parent", // subtask flows only — caller's responsibility
}

var knownSafeIndex = func() map[string]bool {
	m := make(map[string]bool, len(knownSafeFields))
	for _, f := range knownSafeFields {
		m[f] = true
	}
	return m
}()

// KnownSafeFields returns a copy of the known-safe whitelist.
func KnownSafeFields() []string {
	out := make([]string, len(knownSafeFields))
	copy(out, knownSafeFields)
	return out
}

// IsKnownSafeField reports whether a field is in the known-safe whitelist.
func IsKnownSafeField(name string) bool { return knownSafeIndex[name] }

// ApplyKnownSafeFallback strips every field outside the known-safe
// whitelist and emits one warning per dropped field. Used by the
// best-effort field-validation path after a single schema refresh has
// failed to disambiguate the screen.
func ApplyKnownSafeFallback(fields map[string]any) (map[string]any, []adf.Warning) {
	out := make(map[string]any, len(fields))
	var warnings []adf.Warning
	for k, v := range fields {
		if knownSafeIndex[k] {
			out[k] = v
			continue
		}
		warnings = append(warnings, adf.Warning{
			Type:    "field_not_known_safe",
			Message: fmt.Sprintf("field %q dropped — schema unknown and field is not in the known-safe set", k),
			Field:   k,
			Lossy:   true,
		})
	}
	return out, warnings
}
