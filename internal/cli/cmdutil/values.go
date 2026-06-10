package cmdutil

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"

	stdininput "github.com/matcra587/jira-cli/internal/cli/stdin"
)

// stringField reads a string-valued key out of a generic map. A non-string
// value is rendered via fmt.Sprint; a missing or nil value yields "".
func stringField(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	if v, ok := m[key]; ok && v != nil {
		return fmt.Sprint(v)
	}
	return ""
}

// boolField reads a bool-valued key out of a generic map, returning false
// when the key is absent or not a bool.
func boolField(m map[string]any, key string) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}

// StringFromAny returns v when it is a string, or "" otherwise.
func StringFromAny(v any) string {
	switch v := v.(type) {
	case string:
		return v
	default:
		return ""
	}
}

// WireObjectString reads a string field out of a Jira wire object value
// (e.g. the "key" of a {"key":"JCT"} project object). It returns "" when
// the value is not an object or the field is absent / not a string.
func WireObjectString(v any, key string) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	return StringFromAny(m[key])
}

// FirstNonEmpty returns the first non-empty string in values, or "".
func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// CopyAnyMap returns a shallow copy of in.
func CopyAnyMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	maps.Copy(out, in)
	return out
}

// ReadJSONFile decodes exactly one JSON document from path (or stdin when
// path is "-") into dst. A trailing second value, stray structural byte, or
// syntax error is reported as a malformed-payload error.
func ReadJSONFile(path string, dst any) error {
	r, err := stdininput.JSONInput(path)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	dec := json.NewDecoder(r)
	if err := dec.Decode(dst); err != nil {
		return err
	}
	// A --json-input file must hold exactly one JSON document. Decode a
	// second value into a throwaway target: io.EOF is the only acceptable
	// result. Anything else — a second value, a stray trailing `}`/`]`, or
	// a syntax error — means a malformed or concatenated payload. A
	// Decoder.More() check is insufficient: More() reports false for a
	// trailing structural byte, letting `{"summary":"ok"}}` through.
	var trailing json.RawMessage
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return fmt.Errorf("invalid json input %q: unexpected data after the JSON value", path)
	}
	return nil
}

// MarshalNonNilSlice marshals v but rewrites nil slices to `[]` so cache
// files never contain `null`. Without this, decoding back into a typed
// slice produces nil and downstream consumers either crash or paper over
// the bug with `if x == nil` patches.
func MarshalNonNilSlice(v any) (json.RawMessage, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if string(b) == "null" {
		return json.RawMessage("[]"), nil
	}
	return b, nil
}
