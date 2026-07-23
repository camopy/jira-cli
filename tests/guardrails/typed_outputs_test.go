// MOTIVATION: envelope data built as ad-hoc map[string]any let builders,
// schemas, renderers, and docs drift independently — the contract-v2 audit
// found live paths dropping fields their dry-runs carried and schemas
// declaring strings the code emitted as objects. Typed Output structs in
// internal/envelope are the one-declaration fix, but only if every
// operation has one: this guardrail walks the verb registry and fails on
// any operation without a registered typed output (or explicit Dynamic
// exception), so an envelope can never again be born as an untyped map.
// During the migration it doubled as the done-meter, counting down the
// unconverted operations.
package guardrails

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/envelope"
)

// nonEnvelopeOps are verb-registry entries that never write an envelope of
// their own: spinner/progress labels and narration steps whose data (if
// any) rides inside another operation's envelope. Additions here need the
// same justification as a nolint — a real envelope op must never hide here.
var nonEnvelopeOps = map[string]string{
	"auth.login.discover": "clog narration label inside auth.login",
	"update.check":        "spinner label inside update",
	"user.resolve":        "spinner label inside issue.watchers.* flows",
}

func TestEveryOperationHasTypedEnvelopeOutput(t *testing.T) {
	registered := envelope.Registered()
	var missing []string
	for _, op := range cli.OperationNames() {
		if _, excluded := nonEnvelopeOps[op]; excluded {
			continue
		}
		if !slices.Contains(registered, op) {
			missing = append(missing, op)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("%d of %d operations lack a typed envelope output in internal/envelope:\n  %s",
			len(missing), len(cli.OperationNames()), strings.Join(missing, "\n  "))
	}
	// The exclusion list must stay honest too: an op listed there but no
	// longer in the verb registry is stale and must be removed.
	for op := range nonEnvelopeOps {
		if !slices.Contains(cli.OperationNames(), op) {
			t.Fatalf("nonEnvelopeOps lists %q but the verb registry does not contain it", op)
		}
	}
}

// envelopeOpString matches the operation-name argument of a cmdutil envelope
// writer at a call site, e.g. `cmdutil.WriteEnvelope(cmd, "issue.edit", …)`.
var envelopeOpString = regexp.MustCompile(`cmdutil\.Write(?:Keyed[A-Za-z]*|Web)?Envelope[A-Za-z]*\(\s*[A-Za-z]+,\s*"([a-z][a-z0-9._-]*[a-z0-9])"[,)]`)

// TestEveryEnvelopeCommandStringIsRegistered widens the inventory beyond the
// verb registry: some envelope ops (alias.*, config.*, cache.clear) have no
// verb entry, so the verb-walk alone cannot see them. This scan extracts
// every literal operation string passed to a cmdutil envelope writer across
// the command packages and requires a typed-output registration for each —
// an envelope op cannot hide outside the registry.
func TestEveryEnvelopeCommandStringIsRegistered(t *testing.T) {
	registered := envelope.Registered()
	found := map[string][]string{}
	root := "../../internal/cli"
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if strings.Contains(path, "/cmdutil/") {
			return nil // the writer layer itself
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, match := range envelopeOpString.FindAllStringSubmatch(string(raw), -1) {
			op := match[1]
			found[op] = append(found[op], path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if len(found) == 0 {
		t.Fatal("scan found no envelope op strings — the regexp or layout drifted")
	}
	var missing []string
	for op, files := range found {
		if !slices.Contains(registered, op) {
			missing = append(missing, op+" ("+files[0]+")")
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		t.Fatalf("%d envelope operations lack a typed-output registration:\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
}

func TestRegisteredOutputsAreConcreteOrDocumentedDynamic(t *testing.T) {
	outputs := envelope.Outputs()
	registered := envelope.Registered()
	if len(outputs) != len(registered) {
		t.Fatalf("registration inventory differs: Outputs=%d Registered=%d", len(outputs), len(registered))
	}

	var dynamic []string
	var sliceOutputs []string
	for _, op := range registered {
		value, ok := outputs[op]
		if !ok {
			t.Fatalf("registered operation %q has no output value", op)
		}
		if exception, ok := value.(envelope.Dynamic); ok {
			if strings.TrimSpace(exception.Reason) == "" {
				t.Fatalf("dynamic output %q has no reason", op)
			}
			dynamic = append(dynamic, op)
			continue
		}
		typ := reflect.TypeOf(value)
		if typ.Kind() == reflect.Slice && typ.Name() != "" &&
			typ.Elem().Kind() == reflect.Struct && typ.Elem().Name() != "" {
			sliceOutputs = append(sliceOutputs, op)
			continue
		}
		if typ.Kind() != reflect.Struct || typ.Name() == "" {
			t.Fatalf("fixed output %q uses top-level %s; use a named struct or documented Dynamic", op, typ)
		}
	}

	wantSliceOutputs := []string{"agent.adf-matrix", "agent.fieldtypes"}
	if !slices.Equal(sliceOutputs, wantSliceOutputs) {
		t.Fatalf("named slice carriers changed without updating the reviewed inventory:\n got: %v\nwant: %v", sliceOutputs, wantSliceOutputs)
	}

	wantDynamic := []string{
		"auth.status",
		"cache.epics",
		"cache.fields",
		"cache.issuetypes",
		"cache.labels",
		"cache.linktypes",
		"cache.priorities",
		"cache.projects",
		"cache.refresh",
		"cache.resolutions",
		"cache.statuses",
		"release.notes",
		"schema",
	}
	if !slices.Equal(dynamic, wantDynamic) {
		t.Fatalf("dynamic exceptions changed without updating the reviewed inventory:\n got: %v\nwant: %v", dynamic, wantDynamic)
	}
}

func TestNamedSliceOutputsKeepDerivedItemSchemas(t *testing.T) {
	outputs := envelope.Outputs()
	for _, operation := range []string{"agent.adf-matrix", "agent.fieldtypes"} {
		schema := envelope.SchemaOf(outputs[operation], envelope.Doc(operation))
		if schema["type"] != "array" {
			t.Fatalf("%s schema type = %#v, want array: %+v", operation, schema["type"], schema)
		}
		items, ok := schema["items"].(map[string]any)
		if !ok || items["type"] != "object" {
			t.Fatalf("%s schema items = %#v, want typed objects", operation, schema["items"])
		}
		properties, ok := items["properties"].(map[string]any)
		if !ok || len(properties) == 0 {
			t.Fatalf("%s item schema has no properties: %+v", operation, items)
		}
	}
}

func TestSemanticOutputMembersStayConcrete(t *testing.T) {
	checks := []struct {
		operation string
		path      []string
	}{
		{"issue.comment.list", []string{"Comments", "[]"}},
		{"issue.comment.list", []string{"Comments", "[]", "Author", "*"}},
		{"issue.comment.list", []string{"Comments", "[]", "UpdateAuthor", "*"}},
		{"issue.comment.list", []string{"Comments", "[]", "Visibility", "*"}},
		{"issue.comment.edit", []string{"Comment", "*"}},
		{"issue.comment.edit", []string{"Comment", "*", "Author", "*"}},
		{"issue.comment.edit", []string{"Comment", "*", "UpdateAuthor", "*"}},
		{"issue.comment.edit", []string{"Comment", "*", "Visibility", "*"}},
		{"issue.attachment.list", []string{"Attachments", "[]"}},
		{"issue.attachment.list", []string{"Attachments", "[]", "Author"}},
		{"issue.attachment.add", []string{"Files", "[]"}},
		{"issue.attachment.add", []string{"Attachments", "[]"}},
		{"issue.attachment.add", []string{"Attachments", "[]", "Author"}},
		{"issue.watchers.list", []string{"Watchers", "[]"}},
		{"issue.watchers.add", []string{"Watchers", "*", "[]"}},
		{"issue.watchers.remove", []string{"Watchers", "*", "[]"}},
		{"issue.clone", []string{"Payload"}},
		{"issue.move", []string{"Payload"}},
		{"issue.delete", []string{"Payload"}},
	}
	outputs := envelope.Outputs()
	for _, check := range checks {
		t.Run(check.operation+"."+strings.Join(check.path, "."), func(t *testing.T) {
			typ := reflect.TypeOf(outputs[check.operation])
			for _, step := range check.path {
				switch step {
				case "*":
					if typ.Kind() != reflect.Pointer {
						t.Fatalf("%s: got %s, want pointer", strings.Join(check.path, "."), typ)
					}
					typ = typ.Elem()
				case "[]":
					if typ.Kind() != reflect.Slice {
						t.Fatalf("%s: got %s, want slice", strings.Join(check.path, "."), typ)
					}
					typ = typ.Elem()
				default:
					field, ok := typ.FieldByName(step)
					if !ok {
						t.Fatalf("%s has no field %s", typ, step)
					}
					typ = field.Type
				}
			}
			if typ.Kind() == reflect.Map || typ.Kind() == reflect.Interface {
				t.Fatalf("%s member regressed to %s", strings.Join(check.path, "."), typ)
			}
			if typ.Kind() != reflect.Struct {
				t.Fatalf("%s member = %s, want a concrete struct", strings.Join(check.path, "."), typ)
			}
		})
	}
}

func TestSemanticDynamicMembersStayReviewed(t *testing.T) {
	checks := []struct {
		operation string
		path      []string
		kind      reflect.Kind
	}{
		{"issue.comment.list", []string{"Comments", "[]", "Body"}, reflect.Interface},
		{"issue.comment.list", []string{"Warnings", "[]"}, reflect.Map},
		{"issue.create", []string{"Preview"}, reflect.Map},
		{"issue.create", []string{"Verification"}, reflect.Interface},
		{"issue.edit", []string{"Fields"}, reflect.Map},
		{"issue.edit", []string{"Update"}, reflect.Map},
		{"issue.edit", []string{"Verification"}, reflect.Interface},
		{"issue.comment.edit", []string{"BodyADFSummary"}, reflect.Interface},
		{"issue.transition", []string{"Fields"}, reflect.Map},
		{"issue.transition", []string{"Comment"}, reflect.Interface},
		{"issue.transition", []string{"Update"}, reflect.Map},
		{"issue.clone", []string{"Payload", "Fields"}, reflect.Map},
		{"issue.move", []string{"Payload", "Fields"}, reflect.Map},
		{"issue.delete", []string{"Payload", "Fields"}, reflect.Map},
	}
	outputs := envelope.Outputs()
	for _, check := range checks {
		t.Run(check.operation+"."+strings.Join(check.path, "."), func(t *testing.T) {
			typ := reflect.TypeOf(outputs[check.operation])
			for _, step := range check.path {
				switch step {
				case "*":
					if typ.Kind() != reflect.Pointer {
						t.Fatalf("%s: got %s, want pointer", strings.Join(check.path, "."), typ)
					}
					typ = typ.Elem()
				case "[]":
					if typ.Kind() != reflect.Slice {
						t.Fatalf("%s: got %s, want slice", strings.Join(check.path, "."), typ)
					}
					typ = typ.Elem()
				default:
					field, ok := typ.FieldByName(step)
					if !ok {
						t.Fatalf("%s has no field %s", typ, step)
					}
					typ = field.Type
				}
			}
			if typ.Kind() != check.kind {
				t.Fatalf("%s member = %s, want reviewed dynamic %s", strings.Join(check.path, "."), typ, check.kind)
			}
		})
	}
}

func TestIssueIdentityDoesNotLeakIntoUnrelatedOutputs(t *testing.T) {
	outputs := envelope.Outputs()
	for _, operation := range []string{"boards.list", "cache.boards", "config.get", "user.search"} {
		t.Run(operation, func(t *testing.T) {
			typ := reflect.TypeOf(outputs[operation])
			if typ.Kind() != reflect.Struct {
				t.Fatalf("negative control %s is not a concrete struct: %s", operation, typ)
			}
			if _, exists := typ.FieldByName("Issue"); exists {
				t.Fatalf("unrelated operation %s unexpectedly acquired issue identity", operation)
			}
		})
	}
}
