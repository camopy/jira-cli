// Post-write verification for `issue create` / `issue edit --verify`:
// re-fetch the issue after a successful live write and diff the REQUESTED
// wire fields against what the server actually applied. Jira applies writes
// field-by-field and silently drops values it cannot honor (a label not in
// the project scheme, a parent the hierarchy rejects), so a 2xx write is not
// proof the fields landed — only a read back is.

package issue

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/gechr/x/ptr"
	xslices "github.com/gechr/x/slices"
	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/spf13/cobra"
)

// fieldDrop records one requested field the re-fetch shows the server did
// not apply (or applied with a different identity).
type fieldDrop struct {
	Field     string `json:"field"`
	Requested any    `json:"requested"`
	Applied   any    `json:"applied"`
}

// verificationResult is the envelope `verification` block: the applied
// values for every requested field the diff could check, the drops, and the
// requested keys the typed issue model cannot observe (never guessed at —
// an unobservable field must not become a false-positive drop).
type verificationResult struct {
	Applied    map[string]any `json:"applied"`
	Dropped    []fieldDrop    `json:"dropped"`
	Unverified []string       `json:"unverified,omitempty"`
}

// runWriteVerification re-fetches key and diffs requested against the
// server's applied fields. The write has already committed, so a fetch
// failure degrades to a warning — it never fails the command.
func runWriteVerification(cmd *cobra.Command, client *jira.Client, key string, requested map[string]any) (*verificationResult, []adf.Warning) {
	var fetched *jira.Issue
	if err := cmdutil.Spin(cmd, "issue.view", func(ctx context.Context) error {
		var e error
		fetched, _, e = cmdutil.ServicesForClient(client).Issue().Get(ctx, key, nil)
		return e
	}); err != nil {
		return nil, []adf.Warning{{
			Type:    "verification_failed",
			Message: fmt.Sprintf("the write succeeded but verification could not re-fetch %s: %v", key, err),
		}}
	}
	result := verifyAppliedFields(requested, fetched)
	warnings := make([]adf.Warning, 0, len(result.Dropped))
	for _, d := range result.Dropped {
		warnings = append(warnings, adf.Warning{
			Type:    "field_not_applied",
			Field:   d.Field,
			Message: fmt.Sprintf("field %q was not applied: requested %v, server has %v", d.Field, d.Requested, d.Applied),
		})
	}
	return &result, warnings
}

// verifyAppliedFields diffs the requested wire fields (the post-pipeline
// SubmitFields map) against a re-fetched issue. Only fields the user
// explicitly requested are compared, each by its Jira identity (labels and
// versions by name set, parent by key, users by accountId, priority and
// issuetype by name, custom fields by id/value/name where determinable).
// Server-added extras are never drops; a requested key the typed issue model
// cannot observe is reported as unverified, never as dropped.
func verifyAppliedFields(requested map[string]any, issue *jira.Issue) verificationResult {
	result := verificationResult{Applied: map[string]any{}, Dropped: []fieldDrop{}}
	fields := &jira.IssueFields{}
	if issue != nil && issue.Fields != nil {
		fields = issue.Fields
	}
	for _, key := range slices.Sorted(maps.Keys(requested)) {
		req := requested[key]
		switch {
		case key == "summary":
			applied := ptr.Deref(fields.Summary)
			result.Applied[key] = applied
			if want, ok := req.(string); ok && want != applied {
				result.Dropped = append(result.Dropped, fieldDrop{Field: key, Requested: want, Applied: applied})
			}
		case key == "labels":
			want := wireStrings(req)
			result.Applied[key] = fields.Labels
			// Requested labels must all be present; extra server-side labels
			// (automation-added) are not drops.
			if missing := xslices.Difference(want, fields.Labels); len(missing) > 0 {
				result.Dropped = append(result.Dropped, fieldDrop{Field: key, Requested: want, Applied: fields.Labels})
			}
		case key == "parent":
			var applied any
			appliedKey := ""
			if fields.Parent != nil {
				appliedKey = ptr.Deref(fields.Parent.Key)
				applied = appliedKey
			}
			result.Applied[key] = applied
			wantKey := wireIdentity(req, "key")
			switch {
			case req == nil:
				// Explicit clear: any surviving parent is a drop.
				if appliedKey != "" {
					result.Dropped = append(result.Dropped, fieldDrop{Field: key, Requested: nil, Applied: appliedKey})
				}
			case wantKey == "":
				// Requested by a shape we cannot identify (e.g. id-only).
				result.Unverified = append(result.Unverified, key)
			case !strings.EqualFold(wantKey, appliedKey):
				result.Dropped = append(result.Dropped, fieldDrop{Field: key, Requested: wantKey, Applied: applied})
			}
		case key == "assignee" || key == "reporter":
			user := fields.Assignee
			if key == "reporter" {
				user = fields.Reporter
			}
			var applied any
			appliedID := ""
			if user != nil {
				appliedID = ptr.Deref(user.AccountID)
				applied = appliedID
			}
			result.Applied[key] = applied
			wantID := wireIdentity(req, "accountId")
			switch {
			case req == nil || (wantID == "" && wireHasKey(req, "accountId")):
				// Explicit unassign ({"accountId": null} or null).
				if appliedID != "" {
					result.Dropped = append(result.Dropped, fieldDrop{Field: key, Requested: nil, Applied: appliedID})
				}
			case wantID == "":
				result.Unverified = append(result.Unverified, key)
			case wantID != appliedID:
				result.Dropped = append(result.Dropped, fieldDrop{Field: key, Requested: wantID, Applied: applied})
			}
		case key == "priority":
			applied := ""
			if fields.Priority != nil {
				applied = ptr.Deref(fields.Priority.Name)
			}
			result.Applied[key] = applied
			wantName := wireIdentity(req, "name")
			switch {
			case wantName == "":
				// id-only request; the typed model carries only the name.
				result.Unverified = append(result.Unverified, key)
			case !strings.EqualFold(wantName, applied):
				result.Dropped = append(result.Dropped, fieldDrop{Field: key, Requested: wantName, Applied: applied})
			}
		case key == "issuetype":
			applied := ""
			if fields.IssueType != nil {
				applied = ptr.Deref(fields.IssueType.Name)
			}
			result.Applied[key] = applied
			wantName := wireIdentity(req, "name")
			switch {
			case wantName == "":
				result.Unverified = append(result.Unverified, key)
			case !strings.EqualFold(wantName, applied):
				result.Dropped = append(result.Dropped, fieldDrop{Field: key, Requested: wantName, Applied: applied})
			}
		case key == "components":
			names := make([]string, 0, len(fields.Components))
			for _, c := range fields.Components {
				names = append(names, ptr.Deref(c.Name))
			}
			verifyNamedSet(&result, key, req, names)
		case key == "fixVersions" || key == "versions":
			versions := fields.FixVersions
			if key == "versions" {
				versions = fields.Versions
			}
			names := make([]string, 0, len(versions))
			for _, v := range versions {
				names = append(names, ptr.Deref(v.Name))
			}
			verifyNamedSet(&result, key, req, names)
		case key == "project" || key == "description":
			// project cannot silently change on a successful write, and ADF
			// description diffing is out of scope for v1.
			continue
		case strings.HasPrefix(key, "customfield_"):
			raw, ok := fields.CustomFields[key]
			var applied any
			if ok {
				_ = json.Unmarshal(raw, &applied)
			}
			result.Applied[key] = applied
			if applied == nil {
				// Explicit clear (requested null) matching an absent value is
				// not a drop.
				if req != nil {
					result.Dropped = append(result.Dropped, fieldDrop{Field: key, Requested: req, Applied: nil})
				}
				continue
			}
			if !customValueMatches(req, applied) {
				result.Dropped = append(result.Dropped, fieldDrop{Field: key, Requested: req, Applied: applied})
			}
		default:
			// A system field the typed issue model does not carry
			// (duedate, environment, ...): its absence from the fetch proves
			// nothing, so it must not become a false-positive drop.
			result.Unverified = append(result.Unverified, key)
		}
	}
	return result
}

// verifyNamedSet checks a requested name-identified array field (components,
// fixVersions, versions): every requested name must be present in the
// applied names; extras are not drops. Requested elements without a name
// identity mark the field unverified rather than guessed at.
func verifyNamedSet(result *verificationResult, key string, req any, applied []string) {
	result.Applied[key] = applied
	want, allNamed := wireNames(req)
	if !allNamed {
		result.Unverified = append(result.Unverified, key)
		return
	}
	missing := make([]string, 0)
	for _, name := range want {
		if !xslices.ContainsFold(applied, name) {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		result.Dropped = append(result.Dropped, fieldDrop{Field: key, Requested: want, Applied: applied})
	}
}

// wireStrings extracts a string list from a wire value that may arrive as
// []string (flag path) or []any (JSON path).
func wireStrings(v any) []string {
	switch list := v.(type) {
	case []string:
		return list
	case []any:
		out := make([]string, 0, len(list))
		for _, item := range list {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// wireIdentity extracts a string identity field from a wire object
// (map[string]any or map[string]string), returning "" when absent.
func wireIdentity(v any, field string) string {
	switch m := v.(type) {
	case map[string]any:
		s, _ := m[field].(string)
		return s
	case map[string]string:
		return m[field]
	}
	return ""
}

// wireHasKey reports whether a wire object carries the given key at all —
// used to distinguish {"accountId": null} (explicit unassign) from an
// object identified some other way.
func wireHasKey(v any, field string) bool {
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	_, present := m[field]
	return present
}

// wireNames extracts the name identities from a requested array of wire
// objects. ok is false when any element lacks a name — the caller then
// reports the field unverified instead of diffing a partial set.
func wireNames(v any) (names []string, ok bool) {
	list, isList := v.([]any)
	if !isList {
		if typed, isTyped := v.([]map[string]any); isTyped {
			list = make([]any, len(typed))
			for i, m := range typed {
				list[i] = m
			}
		} else {
			return nil, false
		}
	}
	names = make([]string, 0, len(list))
	for _, item := range list {
		name := wireIdentity(item, "name")
		if name == "" {
			return nil, false
		}
		names = append(names, name)
	}
	return names, true
}

// customValueMatches reports whether a requested custom-field value is
// reflected in the applied value. Identity is matched by id, then value,
// then name for option objects; scalars compare loosely (a JSON number
// round-trips as float64); arrays require every requested element to match
// some applied element. Shapes with no determinable identity count as
// applied — presence is the only claim the diff can make.
func customValueMatches(req, applied any) bool {
	switch want := req.(type) {
	case nil:
		return true // nothing requested to contradict
	case string, bool, float64, int, int64:
		if _, isMap := applied.(map[string]any); isMap {
			// A scalar submit that the server echoes as an option object
			// (e.g. a select submitted by raw value) — match on id/value/name.
			return scalarEqual(want, wireIdentity(applied, "id")) ||
				scalarEqual(want, wireIdentity(applied, "value")) ||
				scalarEqual(want, wireIdentity(applied, "name"))
		}
		return scalarEqual(want, applied)
	case map[string]any:
		got, isMap := applied.(map[string]any)
		if !isMap {
			return false
		}
		for _, identity := range []string{"id", "value", "name", "accountId", "key"} {
			if w, present := want[identity]; present && w != nil {
				return scalarEqual(w, got[identity])
			}
		}
		return true // no determinable identity: present is enough
	case []any:
		got, isList := applied.([]any)
		if !isList {
			return false
		}
		for _, item := range want {
			matched := false
			for _, appliedItem := range got {
				if customValueMatches(item, appliedItem) {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		}
		return true
	}
	return true
}

// scalarEqual compares two scalar-ish values loosely: JSON round-trips turn
// ints into float64, so 3 and 3.0 must compare equal.
func scalarEqual(a, b any) bool {
	return fmt.Sprint(a) == fmt.Sprint(b)
}
