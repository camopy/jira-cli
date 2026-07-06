package pipeline

import (
	"reflect"
	"testing"
)

// Bare-string values of the object-valued system fields are lifted to their
// single canonical identity key, so the flat spelling agents write submits
// the shape Jira accepts instead of passing the dry-run and 400ing live.
func TestLiftSystemFieldShapesLiftsBareStrings(t *testing.T) {
	got := LiftSystemFieldShapes(map[string]any{
		"project":   "PROJ",
		"parent":    "PROJ-70",
		"issuetype": "Task",
		"priority":  "Medium",
		"assignee":  "712020:abc",
		"reporter":  "712020:def",
	})
	want := map[string]any{
		"project":   map[string]any{"key": "PROJ"},
		"parent":    map[string]any{"key": "PROJ-70"},
		"issuetype": map[string]any{"name": "Task"},
		"priority":  map[string]any{"name": "Medium"},
		"assignee":  map[string]any{"accountId": "712020:abc"},
		"reporter":  map[string]any{"accountId": "712020:def"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("LiftSystemFieldShapes = %#v, want %#v", got, want)
	}
}

// Explicit wire objects pass through untouched — the lift never rewrites a
// shape the caller chose, including id addressing.
func TestLiftSystemFieldShapesKeepsExplicitObjects(t *testing.T) {
	in := map[string]any{
		"project":  map[string]any{"id": "10000"},
		"priority": map[string]any{"id": "3"},
		"assignee": map[string]any{"accountId": "712020:abc"},
		"components": []any{
			map[string]any{"id": "10001"},
		},
	}
	got := LiftSystemFieldShapes(in)
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("LiftSystemFieldShapes rewrote explicit wire objects: %#v", got)
	}
}

// String elements of the array-valued system fields lift per element; object
// elements are kept, so mixed arrays stay valid.
func TestLiftSystemFieldShapesLiftsArrayElements(t *testing.T) {
	got := LiftSystemFieldShapes(map[string]any{
		"components":  []any{"ui", map[string]any{"id": "10001"}},
		"fixVersions": []any{"1.1.0"},
		"versions":    []any{"1.0.0"},
	})
	want := map[string]any{
		"components":  []any{map[string]any{"name": "ui"}, map[string]any{"id": "10001"}},
		"fixVersions": []any{map[string]any{"name": "1.1.0"}},
		"versions":    []any{map[string]any{"name": "1.0.0"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("LiftSystemFieldShapes = %#v, want %#v", got, want)
	}
}

// A numeric-looking string is lifted under the same fixed key as any other
// string — never guessed into an id.
func TestLiftSystemFieldShapesNeverGuessesIDs(t *testing.T) {
	got := LiftSystemFieldShapes(map[string]any{"priority": "10001"})
	want := map[string]any{"priority": map[string]any{"name": "10001"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("LiftSystemFieldShapes = %#v, want %#v", got, want)
	}
}

// Non-system fields and non-string values are untouched, and the input map
// is never mutated.
func TestLiftSystemFieldShapesLeavesEverythingElseAlone(t *testing.T) {
	in := map[string]any{
		"summary":           "Fix the build",
		"labels":            []any{"regression"},
		"duedate":           "2026-06-01",
		"customfield_10010": "Sprint 5",
		"priority":          "",
		"issuetype":         42,
		"components":        "ui",
	}
	got := LiftSystemFieldShapes(in)
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("LiftSystemFieldShapes = %#v, want input unchanged %#v", got, in)
	}
	// Prove the copy: mutating the output must not touch the input.
	got["summary"] = "mutated"
	if in["summary"] != "Fix the build" {
		t.Fatal("LiftSystemFieldShapes returned the input map, not a copy")
	}
}
