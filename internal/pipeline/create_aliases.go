package pipeline

import "fmt"

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
}

// createAliases is the fixed alias→wire-field table. It is the single
// source of truth shared by NormalizeCreateAliases (used before the
// pipeline) and any caller that needs to know which keys are aliases.
var createAliases = map[string]createAlias{
	"project_key": {
		wireKey: "project",
		encode:  func(v string) map[string]any { return map[string]any{"key": v} },
	},
	"issue_type": {
		wireKey: "issuetype",
		encode:  func(v string) map[string]any { return map[string]any{"name": v} },
	},
	"assignee_account_id": {
		wireKey: "assignee",
		encode:  func(v string) map[string]any { return map[string]any{"accountId": v} },
	},
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
		if _, clash := out[alias.wireKey]; clash {
			if strict {
				return nil, fmt.Errorf("create input sets both %q and %q; supply the field in exactly one place", aliasKey, alias.wireKey)
			}
			// Convenience form: keep the explicit wire value, drop the alias.
			continue
		}
		out[alias.wireKey] = alias.encode(value)
	}
	return out, nil
}
