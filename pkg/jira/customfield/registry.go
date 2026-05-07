// Package customfield is the Jira custom-field type registry.
//
// Symmetric with pkg/adf — same envelope, same single-source-of-truth
// pattern. Each entry knows how to validate user-supplied input for
// its type and encode it into the JSON Jira's REST API expects.
//
// Registry consumers (CLI command code) MUST go through this package
// instead of branching on field type names directly.
package customfield

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// Status tags the support tier a row sits in.
type Status string

const (
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

func (r RegistryView) All() []Entry {
	out := make([]Entry, len(r.entries))
	copy(out, r.entries)
	return out
}

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
	row("select", requireObjectField("value"), passThrough,
		"Single-select; payload is {\"value\":\"<option>\"}."),
	row("multiselect", requireSliceOf(requireObjectField("value")), passThrough,
		"Multi-select; payload is array of {\"value\":\"<option>\"}."),
	row("user", requireObjectField("accountId"), passThrough,
		"User picker; accountId is the Jira identifier."),
	row("group", requireObjectField("name"), passThrough,
		"Group picker; group name in {\"name\":\"<group>\"}."),
	row("components", requireSliceOf(requireObjectField("name")), passThrough,
		"Array of {\"name\":\"<component>\"}."),
	row("version", requireSliceOf(requireObjectField("name")), passThrough,
		"Affects-version array."),
	row("fixversions", requireSliceOf(requireObjectField("name")), passThrough,
		"Fix-version array."),
	row("parent", requireParentKey(), passThrough,
		"Parent issue link by Jira key (matches PROJ-123 shape)."),
	row("cascadingselect", requireCascading, passThrough,
		"Cascading select; payload is {\"value\":\"<top>\",\"child\":{\"value\":\"<sub>\"}}."),
}
