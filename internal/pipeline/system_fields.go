package pipeline

import xslices "github.com/gechr/x/slices"

// systemFieldLifts maps each object-valued system field onto the single
// canonical identity key a bare string value is lifted into. One fixed key
// per field, mirroring the custom-field registry's scalar lifting — never a
// digits-means-id guess, so `"priority": "10001"` becomes {"name":"10001"}
// and fails loudly rather than silently addressing an id.
var systemFieldLifts = map[string]string{
	"project":   "key",
	"parent":    "key",
	"issuetype": "name",
	"priority":  "name",
	"assignee":  "accountId",
	"reporter":  "accountId",
}

// systemFieldSliceLifts are the array-valued system fields whose string
// elements take the same treatment ({"name": ...} per element).
var systemFieldSliceLifts = map[string]string{
	"components":  "name",
	"fixVersions": "name",
	"versions":    "name",
}

// LiftSystemFieldShapes lifts bare-string values of the object-valued
// system fields into their canonical Jira wire objects, so the flat
// spelling agents reach for ("project": "X", "priority": "Medium") submits
// the shape Jira actually accepts instead of passing the dry-run and then
// 400ing on the wire. Explicit wire objects — and every other value shape —
// pass through untouched; only bare strings are lifted. Custom fields keep
// their own registry-driven lifting. The input map is not mutated; a
// normalized copy is returned.
func LiftSystemFieldShapes(fields map[string]any) map[string]any {
	out := make(map[string]any, len(fields))
	for k, v := range fields {
		out[k] = liftSystemFieldValue(k, v)
	}
	return out
}

func liftSystemFieldValue(field string, value any) any {
	if identityKey, ok := systemFieldLifts[field]; ok {
		if s, isString := value.(string); isString && s != "" {
			return map[string]any{identityKey: s}
		}
		return value
	}
	identityKey, ok := systemFieldSliceLifts[field]
	if !ok {
		return value
	}
	elements, isSlice := value.([]any)
	if !isSlice {
		return value
	}
	return xslices.Map(elements, func(element any) any {
		if s, isString := element.(string); isString && s != "" {
			return map[string]any{identityKey: s}
		}
		return element
	})
}
