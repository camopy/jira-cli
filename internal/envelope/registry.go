package envelope

import (
	"fmt"
	"maps"
	"sort"
)

// outputs maps operation names (verbs.go keys, e.g. "issue.edit") to the
// zero value of the typed Output struct their envelopes are built from.
// Registration lives beside the struct definitions in this package's
// outputs_*.go files; the guardrail test in tests/guardrails fails while
// any registered verb lacks an entry here, which makes this map the
// migration's done-meter and, afterwards, its regression guard.
var outputs = map[string]any{}

// docs holds per-operation prose overrides (descriptions, enums, formats)
// registered beside the structs — the escape hatch for meaning the type
// system cannot express, applied on top of any prose harvested from the
// legacy hand-written schemas.
var docs = map[string]map[string]any{}

// register records op's typed Output and returns v so registration can sit
// beside the struct as a package-level `var _ = register(...)` — explicit
// wiring, no init ordering. It panics on a duplicate: two structs claiming
// one operation is a programmer error caught at startup, same posture as
// the guide-embed checks.
func register(op string, v any, doc map[string]any) any {
	if _, dup := outputs[op]; dup {
		panic(fmt.Sprintf("envelope: duplicate output registration for %q", op))
	}
	outputs[op] = v
	if doc != nil {
		docs[op] = doc
	}
	return v
}

// Dynamic marks an operation whose data shape cannot be a fixed struct —
// a generated per-resource field name (the cache.<resource> family) or a
// registry dump emitted as a bare array. Registering one is a conscious,
// documented exception: the exhaustiveness guardrail counts it, and schema
// derivation leaves the op's hand-written schema untouched. Reason is
// mandatory and should say why a struct cannot express the shape.
type Dynamic struct {
	Reason string
}

// Doc returns op's registered prose overrides, or nil.
func Doc(op string) map[string]any {
	return docs[op]
}

// Registered returns the operation names with typed outputs, sorted.
func Registered() []string {
	ops := make([]string, 0, len(outputs))
	for op := range outputs {
		ops = append(ops, op)
	}
	sort.Strings(ops)
	return ops
}

// Outputs returns a copy of the registration map for schema derivation:
// callers derive each op's published schema from the registered zero value.
func Outputs() map[string]any {
	return maps.Clone(outputs)
}
