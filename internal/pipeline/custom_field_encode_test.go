package pipeline_test

import (
	"reflect"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli/adfmode"
	"github.com/matcra587/jira-cli/internal/pipeline"
	"github.com/matcra587/jira-cli/pkg/adf"
)

// The stage-4 encoder must genuinely ENCODE, not merely validate: when
// the screen schema declares a field's type, a bare scalar label is
// lifted to the typed wire shape. A bare "High" for a select field is
// encoded as {"value":"High"} on the wire.
func TestRunMutationEncodesBareSelectLabel(t *testing.T) {
	schema := pipeline.ScreenSchema{
		Project:     "KAN",
		IssueType:   "Story",
		ValidFields: map[string]bool{"summary": true, "customfield_40001": true},
		FieldTypes:  map[string]string{"customfield_40001": "select"},
	}
	out := pipeline.RunMutation(pipeline.MutationInput{
		Mode: adfmode.ModeStrict,
		Fields: map[string]any{
			"summary":           "ok",
			"customfield_40001": "High",
		},
		Schema: schema,
		DryRun: true,
	})
	if out.Aborted {
		t.Fatalf("bare select label must encode, not abort: %+v", out)
	}
	got := out.SubmitFields["customfield_40001"]
	want := map[string]any{"value": "High"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bare select label not lifted to typed shape: got %#v want %#v", got, want)
	}
}

// An already-typed select object is accepted as-is — the encoder does
// not double-wrap an explicit {"value":...} or {"id":...} input.
func TestRunMutationKeepsExplicitSelectObject(t *testing.T) {
	schema := pipeline.ScreenSchema{
		ValidFields: map[string]bool{"customfield_40002": true},
		FieldTypes:  map[string]string{"customfield_40002": "select"},
	}
	out := pipeline.RunMutation(pipeline.MutationInput{
		Mode:   adfmode.ModeStrict,
		Fields: map[string]any{"customfield_40002": map[string]any{"id": "10100"}},
		Schema: schema,
		DryRun: true,
	})
	if out.Aborted {
		t.Fatalf("explicit select id object must pass: %+v", out)
	}
	got := out.SubmitFields["customfield_40002"]
	want := map[string]any{"id": "10100"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("explicit select object must be kept as-is: got %#v", got)
	}
}

// A bare multi-select label list is lifted to an array of typed
// objects.
func TestRunMutationEncodesBareMultiSelectLabels(t *testing.T) {
	schema := pipeline.ScreenSchema{
		ValidFields: map[string]bool{"customfield_40003": true},
		FieldTypes:  map[string]string{"customfield_40003": "multiselect"},
	}
	out := pipeline.RunMutation(pipeline.MutationInput{
		Mode:   adfmode.ModeStrict,
		Fields: map[string]any{"customfield_40003": []any{"A", "B"}},
		Schema: schema,
		DryRun: true,
	})
	if out.Aborted {
		t.Fatalf("bare multiselect labels must encode: %+v", out)
	}
	got := out.SubmitFields["customfield_40003"]
	want := []any{map[string]any{"value": "A"}, map[string]any{"value": "B"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bare multiselect labels not lifted: got %#v", got)
	}
}

// A bare account-id string for a user-picker field is lifted to
// {"accountId":...}.
func TestRunMutationEncodesBareUserAccountID(t *testing.T) {
	schema := pipeline.ScreenSchema{
		ValidFields: map[string]bool{"customfield_40004": true},
		FieldTypes:  map[string]string{"customfield_40004": "userpicker"},
	}
	out := pipeline.RunMutation(pipeline.MutationInput{
		Mode:   adfmode.ModeStrict,
		Fields: map[string]any{"customfield_40004": "acc-99"},
		Schema: schema,
		DryRun: true,
	})
	if out.Aborted {
		t.Fatalf("bare user accountId must encode: %+v", out)
	}
	got := out.SubmitFields["customfield_40004"]
	want := map[string]any{"accountId": "acc-99"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bare user accountId not lifted: got %#v", got)
	}
}

// A textarea custom field given a plain string / Markdown is encoded as
// an ADF document — Cloud v3 multi-line-text fields require ADF, not a
// bare string.
func TestRunMutationEncodesTextareaAsADF(t *testing.T) {
	schema := pipeline.ScreenSchema{
		ValidFields: map[string]bool{"customfield_40005": true},
		FieldTypes:  map[string]string{"customfield_40005": "textarea"},
	}
	out := pipeline.RunMutation(pipeline.MutationInput{
		Mode:   adfmode.ModeStrict,
		Fields: map[string]any{"customfield_40005": "some **bold** text"},
		Schema: schema,
		DryRun: true,
	})
	if out.Aborted {
		t.Fatalf("textarea string must encode to ADF, not abort: %+v", out)
	}
	got := out.SubmitFields["customfield_40005"]
	doc, ok := got.(adf.Document)
	if !ok {
		t.Fatalf("textarea value must be encoded as an adf.Document, got %T", got)
	}
	if doc.Type != "doc" || doc.Version != 1 {
		t.Fatalf("textarea ADF document malformed: %#v", doc)
	}
}

// The known Atlassian system tokens multiuserpicker / multigrouppicker
// / multiversion / version / project must be recognized as known types:
// a value supplied in the wrong shape for one of them is rejected as a
// known-type error, not opaque-forwarded with a wrong "unknown type"
// warning.
func TestRunMutationRecognizesAdditionalKnownTokens(t *testing.T) {
	cases := map[string]any{
		"multiuserpicker":  float64(7),                            // not an array
		"multigrouppicker": float64(7),                            // not an array
		"multiversion":     float64(7),                            // not an array
		"version":          float64(7),                            // neither a name string nor an object
		"project":          map[string]any{"unexpected": "field"}, // object missing key/id
	}
	for token, badValue := range cases {
		t.Run(token, func(t *testing.T) {
			schema := pipeline.ScreenSchema{
				ValidFields: map[string]bool{"customfield_50001": true},
				FieldTypes:  map[string]string{"customfield_50001": token},
			}
			out := pipeline.RunMutation(pipeline.MutationInput{
				Mode:   adfmode.ModeStrict,
				Fields: map[string]any{"customfield_50001": badValue},
				Schema: schema,
			})
			if !out.Aborted || out.AbortedAt != pipeline.StageCustomField {
				t.Fatalf("token %q with a malformed value must abort at stage 4 as a known type, got %+v", token, out)
			}
		})
	}
}

// A bare version name is lifted to {"name":...}; a bare project key is
// lifted to {"key":...}.
func TestRunMutationEncodesBareVersionAndProject(t *testing.T) {
	schema := pipeline.ScreenSchema{
		ValidFields: map[string]bool{"customfield_50002": true, "customfield_50003": true},
		FieldTypes: map[string]string{
			"customfield_50002": "version",
			"customfield_50003": "project",
		},
	}
	out := pipeline.RunMutation(pipeline.MutationInput{
		Mode: adfmode.ModeStrict,
		Fields: map[string]any{
			"customfield_50002": "1.0",
			"customfield_50003": "KAN",
		},
		Schema: schema,
		DryRun: true,
	})
	if out.Aborted {
		t.Fatalf("bare version/project must encode: %+v", out)
	}
	if got := out.SubmitFields["customfield_50002"]; !reflect.DeepEqual(got, map[string]any{"name": "1.0"}) {
		t.Fatalf("bare version not lifted: got %#v", got)
	}
	if got := out.SubmitFields["customfield_50003"]; !reflect.DeepEqual(got, map[string]any{"key": "KAN"}) {
		t.Fatalf("bare project not lifted: got %#v", got)
	}
}

// A textarea field already given an ADF document is accepted as-is.
func TestRunMutationKeepsTextareaADFDocument(t *testing.T) {
	schema := pipeline.ScreenSchema{
		ValidFields: map[string]bool{"customfield_40006": true},
		FieldTypes:  map[string]string{"customfield_40006": "textarea"},
	}
	doc := map[string]any{
		"type":    "doc",
		"version": float64(1),
		"content": []any{},
	}
	out := pipeline.RunMutation(pipeline.MutationInput{
		Mode:   adfmode.ModeStrict,
		Fields: map[string]any{"customfield_40006": doc},
		Schema: schema,
		DryRun: true,
	})
	if out.Aborted {
		t.Fatalf("textarea ADF object must pass: %+v", out)
	}
}
