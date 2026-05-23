package contract

import (
	"encoding/json"
	"testing"
)

// envelopeHasKV reports whether the JSON output contains key k anywhere in
// the decoded tree with a scalar value equal to want. Parsing the envelope
// instead of substring-matching keeps assertions independent of JSON
// whitespace (flat vs indented) and key ordering.
func envelopeHasKV(t *testing.T, out []byte, k string, want any) bool {
	t.Helper()
	return jsonWalkKV(parseJSONTree(t, out), k, want)
}

// envelopeHasKey reports whether key k appears anywhere in the decoded tree,
// regardless of its value.
func envelopeHasKey(t *testing.T, out []byte, k string) bool {
	t.Helper()
	return jsonWalkKey(parseJSONTree(t, out), k)
}

// envelopeHasValue reports whether the scalar value want appears anywhere in
// the decoded tree, as a map value or an array element.
func envelopeHasValue(t *testing.T, out []byte, want any) bool {
	t.Helper()
	return jsonWalkValue(parseJSONTree(t, out), want)
}

func parseJSONTree(t *testing.T, out []byte) any {
	t.Helper()
	var doc any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	return doc
}

func jsonWalkKV(node any, k string, want any) bool {
	switch n := node.(type) {
	case map[string]any:
		for key, val := range n {
			if key == k && jsonScalarEqual(val, want) {
				return true
			}
			if jsonWalkKV(val, k, want) {
				return true
			}
		}
	case []any:
		for _, item := range n {
			if jsonWalkKV(item, k, want) {
				return true
			}
		}
	}
	return false
}

func jsonWalkKey(node any, k string) bool {
	switch n := node.(type) {
	case map[string]any:
		for key, val := range n {
			if key == k {
				return true
			}
			if jsonWalkKey(val, k) {
				return true
			}
		}
	case []any:
		for _, item := range n {
			if jsonWalkKey(item, k) {
				return true
			}
		}
	}
	return false
}

func jsonWalkValue(node, want any) bool {
	switch n := node.(type) {
	case map[string]any:
		for _, val := range n {
			if jsonScalarEqual(val, want) || jsonWalkValue(val, want) {
				return true
			}
		}
	case []any:
		for _, item := range n {
			if jsonScalarEqual(item, want) || jsonWalkValue(item, want) {
				return true
			}
		}
	}
	return false
}

// jsonScalarEqual compares a decoded JSON value against an expected Go value,
// bridging JSON's float64 numbers to int expectations.
func jsonScalarEqual(got, want any) bool {
	if w, ok := want.(int); ok {
		f, ok := got.(float64)
		return ok && f == float64(w)
	}
	return got == want
}
