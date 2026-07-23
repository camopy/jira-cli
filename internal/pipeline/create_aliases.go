package pipeline

import (
	"fmt"
	"strings"
)

// createAlias maps a CLI create-input alias key onto the Jira wire
// field id the create screen and the REST API actually use. The CLI
// accepts human-friendly aliases (project_key, issue_type,
// assignee_account_id); Jira's createmeta screen and IssueUpdateDetails
// body key on project / issuetype / assignee.
type createAlias struct {
	wireKey string
	// encode wraps the bare alias value into the wire object shape Jira
	// expects for that field.
	encode func(value string) map[string]any
	// wireValueKeys are the wire-object fields that carry the same
	// identity the alias carries, in preference order — used to compare
	// an alias value against an explicit wire object and to read the
	// identity back out of a wire-only payload.
	wireValueKeys []string
}

// createAliases is the fixed alias→wire-field table. It is the single
// source of truth shared by NormalizeCreateAliases (used before the
// pipeline) and any caller that needs to know which keys are aliases.
var createAliases = map[string]createAlias{
	"project_key": {
		wireKey:       "project",
		encode:        func(v string) map[string]any { return map[string]any{"key": v} },
		wireValueKeys: []string{"key", "id"},
	},
	"issue_type": {
		wireKey:       "issuetype",
		encode:        func(v string) map[string]any { return map[string]any{"name": v} },
		wireValueKeys: []string{"name", "id"},
	},
	"assignee_account_id": {
		wireKey:       "assignee",
		encode:        func(v string) map[string]any { return map[string]any{"accountId": v} },
		wireValueKeys: []string{"accountId"},
	},
}

// CreateWireValue reads the identity string for aliasKey out of fields,
// accepting either spelling: the flat alias value, or the wire object's
// identity field ("project": {"key": "X"} yields "X" for project_key).
// Empty means neither spelling carries a value.
func CreateWireValue(fields map[string]any, aliasKey string) string {
	alias, ok := createAliases[aliasKey]
	if !ok {
		return ""
	}
	if v, ok := fields[aliasKey].(string); ok && v != "" {
		return v
	}
	return wireIdentity(fields[alias.wireKey], alias.wireValueKeys)
}

// wireIdentity pulls the first non-empty identity field out of a wire
// object. A bare string wire value counts as its own identity.
func wireIdentity(wire any, keys []string) string {
	switch w := wire.(type) {
	case string:
		return w
	case map[string]any:
		for _, k := range keys {
			if v, ok := w[k].(string); ok && v != "" {
				return v
			}
		}
	}
	return ""
}

// NormalizeCreateAliases translates the CLI create-input aliases into
// the Jira wire field ids BEFORE the mutation pipeline runs. Screen
// validation (stage 3) keys on the wire ids, so an un-normalized alias
// such as project_key would be flagged off-screen even for a default
// create. The input map is not mutated; a normalized copy is returned.
//
// A non-empty string alias value is encoded into its wire object shape
// (project_key "JCT" -> project {"key":"JCT"}). An alias with an empty
// or non-string value is dropped — it carries nothing to send. An alias
// whose wire key is also explicitly set is left for
// NormalizeCreateAliasesChecked to reject; this convenience form keeps
// the explicit value and discards the alias.
func NormalizeCreateAliases(fields map[string]any) map[string]any {
	// The non-strict form resolves conflicts by keeping the explicit wire key,
	// so its normalizer cannot return an error.
	//nolint:errcheck // strict=false makes the helper's error path unreachable
	out, _ := normalizeCreateAliases(fields, false)
	return out
}

// NormalizeCreateAliasesChecked is NormalizeCreateAliases that also
// reports a conflict: when an alias and its wire key are BOTH set, the
// CLI will not silently pick a winner. The user must supply the field
// in exactly one place.
func NormalizeCreateAliasesChecked(fields map[string]any) (map[string]any, error) {
	return normalizeCreateAliases(fields, true)
}

func normalizeCreateAliases(fields map[string]any, strict bool) (map[string]any, error) {
	out := make(map[string]any, len(fields))
	for k, v := range fields {
		if _, isAlias := createAliases[k]; isAlias {
			continue
		}
		out[k] = v
	}
	for aliasKey, alias := range createAliases {
		raw, present := fields[aliasKey]
		if !present {
			continue
		}
		value, ok := raw.(string)
		if !ok || value == "" {
			// An alias with no usable value carries nothing to send.
			continue
		}
		if wire, clash := out[alias.wireKey]; clash {
			// Both spellings set: agreement is not a conflict — a profile
			// default or a duplicated-but-identical value proceeds with the
			// explicit wire object kept. Only a genuine mismatch errors,
			// naming both values so the caller can see which to drop.
			if wireVal := wireIdentity(wire, alias.wireValueKeys); strings.EqualFold(wireVal, value) {
				continue
			} else if strict {
				return nil, fmt.Errorf("create input sets %s=%q and %s=%q with different values; supply the field once or align the values", aliasKey, value, alias.wireKey, wireVal)
			}
			// Convenience form: keep the explicit wire value, drop the alias.
			continue
		}
		out[alias.wireKey] = alias.encode(value)
	}
	return out, nil
}
