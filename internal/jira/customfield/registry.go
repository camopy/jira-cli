package customfield

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"time"

	xslices "github.com/gechr/x/slices"
)

// Status tags the support tier a row sits in.
type Status string

const (
	// StatusMVP marks a field type shipped in the minimum-viable set — the
	// only tier currently populated. It exists so the registry envelope can
	// report a support tier per row without the value being a bare literal.
	StatusMVP Status = "mvp"
)

// Capabilities mirrors the capabilities object on pkg/adf.
type Capabilities struct {
	Author   bool `json:"author"`
	Render   bool `json:"render"`
	Preserve bool `json:"preserve"`
	Validate bool `json:"validate"`
	Submit   bool `json:"submit"`
}

// Validator returns nil if the value is acceptable for this type, or a
// typed error describing why it isn't. Encoders rely on validation
// having passed.
type Validator func(value any) error

// Encoder converts a validated value into the JSON shape Jira's REST
// API expects when posted to fields[<id>]. May reshape (e.g., wrap a
// string in {"value": "..."}) or pass through unchanged.
type Encoder func(value any) (any, error)

// Entry is one registry row — same envelope as pkg/adf.Entry plus
// customfield-specific Encoder/Validator pointers.
type Entry struct {
	Kind              string          `json:"kind"`
	Name              string          `json:"name"`
	Status            Status          `json:"status"`
	Capabilities      Capabilities    `json:"capabilities"`
	InputShape        json.RawMessage `json:"input_shape,omitempty"`
	OutputShape       json.RawMessage `json:"output_shape,omitempty"`
	Warnings          []string        `json:"warnings,omitempty"`
	OfficialURL       string          `json:"official_url,omitempty"`
	Notes             string          `json:"notes,omitempty"`
	SubmitDescription string          `json:"submit_description"`

	Validator Validator `json:"-"`
	Encoder   Encoder   `json:"-"`
}

// RegistryView is the read-only handle.
type RegistryView struct {
	entries []Entry
	index   map[string]Entry
}

// All returns a copy of every registry row, in table order, for the schema and
// fieldtypes agent commands. The slice is copied so a consumer cannot mutate the
// package's table of truth.
func (r RegistryView) All() []Entry {
	out := make([]Entry, len(r.entries))
	copy(out, r.entries)
	return out
}

// Lookup returns the entry for a field-type name and whether it exists. The
// mutation pipeline uses it to find the validator/encoder for a field's type;
// an absent name (a marketplace or unmodeled type) reports false so the caller
// can forward the value opaquely rather than branching on type strings.
func (r RegistryView) Lookup(name string) (Entry, bool) {
	e, ok := r.index[name]
	return e, ok
}

const submitDesc = "Customfield: encoded into the Jira REST field payload after schema validation passes."

var fullCaps = Capabilities{Author: true, Render: true, Preserve: true, Validate: true, Submit: true}

// row helps keep the table below visually scannable. official_url points
// at the Atlassian Jira-expressions type-reference page, the only
// allowed source. Per-type subsections of that page document each field
// shape we encode here.
const fieldTypesDocsURL = "https://developer.atlassian.com/cloud/jira/platform/jira-expressions-type-reference/"

func row(name string, validator Validator, encoder Encoder, notes string) Entry {
	return Entry{
		Kind:              "field-type",
		Name:              name,
		Status:            StatusMVP,
		Capabilities:      fullCaps,
		OfficialURL:       fieldTypesDocsURL,
		Notes:             notes,
		SubmitDescription: submitDesc,
		Validator:         validator,
		Encoder:           encoder,
	}
}

// Registry returns the read-only registry view.
func Registry() RegistryView {
	idx := make(map[string]Entry, len(entries))
	for _, e := range entries {
		idx[e.Name] = e
	}
	return RegistryView{entries: entries, index: idx}
}

// passThrough returns the input unchanged. Used by types Jira already
// accepts in its native shape (number, string, date, datetime, labels).
func passThrough(v any) (any, error) { return v, nil }

// liftScalarObject returns an Encoder that lifts a bare scalar into the
// typed object shape {key: value} Jira expects. A value that is already
// an object (an explicit {"value":...} / {"id":...} / {"name":...}
// supplied by the caller) is passed through unchanged so explicit
// input is honored. The companion validator must already have
// accepted both shapes.
func liftScalarObject(key string) Encoder {
	return func(v any) (any, error) {
		if s, ok := v.(string); ok {
			return map[string]any{key: s}, nil
		}
		return v, nil
	}
}

// liftScalarSlice returns an Encoder for array custom fields: a bare
// list of scalars is lifted element-wise into typed objects
// ([]any{"A","B"} -> [{key:"A"},{key:"B"}]); an element that is already
// an object is kept as-is. A non-slice value is returned unchanged for
// the validator to have caught.
func liftScalarSlice(key string) Encoder {
	return func(v any) (any, error) {
		s, ok := v.([]any)
		if !ok {
			return v, nil
		}
		return xslices.Map(s, func(el any) any {
			if str, isStr := el.(string); isStr {
				return map[string]any{key: str}
			}
			return el
		}), nil
	}
}

// acceptScalarOrObject returns a Validator that accepts EITHER a
// non-empty bare string (a human label / id the encoder will lift) OR
// an object carrying any one of objKeys with a non-empty string value.
// It is the input contract for select-family, user, group, version and
// project custom fields where the CLI lifts a bare label to the typed
// wire shape but also honors an explicit object.
func acceptScalarOrObject(objKeys ...string) Validator {
	return func(v any) error {
		if s, ok := v.(string); ok {
			if s == "" {
				return fmt.Errorf("expected a non-empty string")
			}
			return nil
		}
		m, ok := v.(map[string]any)
		if !ok {
			return fmt.Errorf("expected a string or object, got %T", v)
		}
		for _, k := range objKeys {
			if raw, has := m[k]; has {
				s, ok := raw.(string)
				if !ok {
					return fmt.Errorf("key %q must be a string, got %T", k, raw)
				}
				if s == "" {
					return fmt.Errorf("key %q must be non-empty", k)
				}
				return nil
			}
		}
		return fmt.Errorf("object must carry one of %v", objKeys)
	}
}

// acceptScalarSliceOrObjects returns a Validator for array custom
// fields: it accepts a slice whose every element is either a non-empty
// bare string or an object carrying one of objKeys.
func acceptScalarSliceOrObjects(objKeys ...string) Validator {
	element := acceptScalarOrObject(objKeys...)
	return func(v any) error {
		s, ok := v.([]any)
		if !ok {
			return fmt.Errorf("expected array, got %T", v)
		}
		for i, el := range s {
			if err := element(el); err != nil {
				return fmt.Errorf("element %d: %w", i, err)
			}
		}
		return nil
	}
}

// requireString returns nil iff v is a non-empty string.
func requireString(v any) error {
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf("expected string, got %T", v)
	}
	if s == "" {
		return fmt.Errorf("expected non-empty string")
	}
	return nil
}

// requireNumber accepts float64 (the JSON default) or anything that
// converts cleanly via fmt.Sprintf.
func requireNumber(v any) error {
	switch x := v.(type) {
	case float64, int, int64:
		return nil
	case string:
		_, err := strconv.ParseFloat(x, 64)
		if err != nil {
			return fmt.Errorf("number: %q: %w", x, err)
		}
		return nil
	}
	return fmt.Errorf("expected number, got %T", v)
}

// requireObjectField returns a validator that fails when v is not a map
// containing every named key with a non-empty string value. Stage-4
// performs shape AND type checking, not just key presence.
func requireObjectField(keys ...string) Validator {
	return func(v any) error {
		m, ok := v.(map[string]any)
		if !ok {
			return fmt.Errorf("expected object, got %T", v)
		}
		for _, k := range keys {
			raw, has := m[k]
			if !has {
				return fmt.Errorf("missing required key %q", k)
			}
			s, ok := raw.(string)
			if !ok {
				return fmt.Errorf("key %q must be a string, got %T", k, raw)
			}
			if s == "" {
				return fmt.Errorf("key %q must be non-empty", k)
			}
		}
		return nil
	}
}

// requireSliceOf returns a validator that fails when v is not a slice
// where every element passes the inner validator.
func requireSliceOf(inner Validator) Validator {
	return func(v any) error {
		s, ok := v.([]any)
		if !ok {
			return fmt.Errorf("expected array, got %T", v)
		}
		for i, el := range s {
			if err := inner(el); err != nil {
				return fmt.Errorf("element %d: %w", i, err)
			}
		}
		return nil
	}
}

// requireCascading validates the {value, child:{value}} cascading
// select shape Jira expects. value MUST be a non-empty string; if
// child is supplied it MUST be an object whose value is also a
// non-empty string.
func requireCascading(v any) error {
	m, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("expected object, got %T", v)
	}
	rawValue, has := m["value"]
	if !has {
		return fmt.Errorf("cascadingselect missing required key %q", "value")
	}
	value, ok := rawValue.(string)
	if !ok || value == "" {
		return fmt.Errorf("cascadingselect.value must be a non-empty string, got %T", rawValue)
	}
	if rawChild, has := m["child"]; has {
		child, ok := rawChild.(map[string]any)
		if !ok {
			return fmt.Errorf("cascadingselect.child must be an object")
		}
		rawChildValue, has := child["value"]
		if !has {
			return fmt.Errorf("cascadingselect.child missing required key %q", "value")
		}
		cv, ok := rawChildValue.(string)
		if !ok || cv == "" {
			return fmt.Errorf("cascadingselect.child.value must be a non-empty string, got %T", rawChildValue)
		}
	}
	return nil
}

// requireParentKey returns a validator for parent issue refs: the
// value MUST be a map carrying a `key` whose value matches Jira's
// issue-key shape (`PROJ-123`).
func requireParentKey() Validator {
	keyShape := regexp.MustCompile(`^[A-Z][A-Z0-9_]+-\d+$`)
	return func(v any) error {
		m, ok := v.(map[string]any)
		if !ok {
			return fmt.Errorf("parent: expected object, got %T", v)
		}
		raw, has := m["key"]
		if !has {
			return fmt.Errorf("parent missing required key %q", "key")
		}
		s, ok := raw.(string)
		if !ok {
			return fmt.Errorf("parent.key must be a string, got %T", raw)
		}
		if !keyShape.MatchString(s) {
			return fmt.Errorf("parent.key %q must match Jira issue-key shape (e.g. PROJ-123)", s)
		}
		return nil
	}
}

// requireDateString validates a calendar date in YYYY-MM-DD format.
// Uses time.Parse, which rejects out-of-range months/days, non-existent
// dates (Feb 30, Sep 31), and any non-numeric components — no regex
// needed.
func requireDateString(v any) error {
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf("date must be a string, got %T", v)
	}
	if _, err := time.Parse("2006-01-02", s); err != nil {
		return fmt.Errorf("date %q must be YYYY-MM-DD: %w", s, err)
	}
	return nil
}

// requireDatetimeString validates an ISO-8601 datetime with timezone.
// Accepts the layouts Jira's REST API emits (`+HHMM`, `+HH:MM`, `Z`)
// and rejects natural-language values like "tomorrow". time.Parse
// handles range/leap-year/timezone correctness for free.
func requireDatetimeString(v any) error {
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf("datetime must be a string, got %T", v)
	}
	for _, layout := range datetimeLayouts {
		if _, err := time.Parse(layout, s); err == nil {
			return nil
		}
	}
	return fmt.Errorf("datetime %q must be ISO-8601 with timezone (e.g. 2026-05-04T10:00:00.000+0000)", s)
}

// datetimeLayouts covers the Jira-emitted variants. RFC3339 / RFC3339Nano
// handle `Z` and `±HH:MM`; the third layout is Jira's no-colon offset
// (`+0000`) variant.
var datetimeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.000-0700",
	"2006-01-02T15:04:05-0700",
}

// entries is the table-of-truth — adding a new field type means adding
// one row here plus a validator round-trip case in registry_coverage_test.go.
var entries = []Entry{
	row("string", requireString, passThrough,
		"Plain string field; Jira accepts the raw value."),
	row("number", requireNumber, passThrough,
		"Numeric field; floats and ints accepted."),
	row("date", requireDateString, passThrough,
		"YYYY-MM-DD date string with in-range month/day."),
	row("datetime", requireDatetimeString, passThrough,
		"ISO-8601 datetime string with timezone offset."),
	row("labels", requireSliceOf(func(v any) error { return requireString(v) }), passThrough,
		"Array of plain string labels."),
	row("select", acceptScalarOrObject("value", "id"), liftScalarObject("value"),
		"Single-select; a bare label is encoded as {\"value\":\"<option>\"}; an explicit {\"value\"}/{\"id\"} object is kept."),
	row("multiselect", acceptScalarSliceOrObjects("value", "id"), liftScalarSlice("value"),
		"Multi-select; a bare label list is encoded as an array of {\"value\":\"<option>\"}."),
	row("user", acceptScalarOrObject("accountId", "id"), liftScalarObject("accountId"),
		"User picker; a bare account id is encoded as {\"accountId\":\"<id>\"}."),
	row("multiuser", acceptScalarSliceOrObjects("accountId", "id"), liftScalarSlice("accountId"),
		"Multi-user picker; a bare account-id list is encoded as an array of {\"accountId\":\"<id>\"}."),
	row("group", acceptScalarOrObject("name"), liftScalarObject("name"),
		"Group picker; a bare group name is encoded as {\"name\":\"<group>\"}."),
	row("multigroup", acceptScalarSliceOrObjects("name"), liftScalarSlice("name"),
		"Multi-group picker; a bare group-name list is encoded as an array of {\"name\":\"<group>\"}."),
	row("components", requireSliceOf(requireObjectField("name")), passThrough,
		"Array of {\"name\":\"<component>\"}."),
	row("version", requireSliceOf(requireObjectField("name")), passThrough,
		"Affects-version array."),
	row("fixversions", requireSliceOf(requireObjectField("name")), passThrough,
		"Fix-version array."),
	row("versionpicker", acceptScalarOrObject("name", "id"), liftScalarObject("name"),
		"Single-version custom field; a bare version name is encoded as {\"name\":\"<version>\"}."),
	row("multiversionpicker", acceptScalarSliceOrObjects("name", "id"), liftScalarSlice("name"),
		"Multi-version custom field; a bare version-name list is encoded as an array of {\"name\":\"<version>\"}."),
	row("projectpicker", acceptScalarOrObject("key", "id"), liftScalarObject("key"),
		"Project-picker custom field; a bare project key is encoded as {\"key\":\"<project>\"}."),
	row("parent", requireParentKey(), passThrough,
		"Parent issue link by Jira key (matches PROJ-123 shape)."),
	row("cascadingselect", requireCascading, passThrough,
		"Cascading select; payload is {\"value\":\"<top>\",\"child\":{\"value\":\"<sub>\"}}."),
}
