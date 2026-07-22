package contract

import (
	"encoding/json"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// docentSchema mirrors the docent Command JSON `jira agent schema` emits —
// only the fields these tests assert on.
type docentSchema struct {
	Name            string             `json:"name"`
	Path            string             `json:"path"`
	Description     string             `json:"description"`
	Flags           []docentSchemaFlag `json:"flags"`
	FlagGroups      []docentFlagGroup  `json:"flag_groups"`
	Children        []docentSchema     `json:"children"`
	InputSchema     map[string]any     `json:"input_schema"`
	OutputSchema    map[string]any     `json:"output_schema"`
	HasInputSchema  bool               `json:"has_input_schema"`
	HasOutputSchema bool               `json:"has_output_schema"`
	Extensions      map[string]any     `json:"extensions"`
	Defs            map[string]any     `json:"$defs"`
}

type docentSchemaFlag struct {
	Name        string         `json:"name"`
	Shorthand   string         `json:"shorthand"`
	Description string         `json:"description"`
	Type        string         `json:"type"`
	Default     string         `json:"default"`
	Enum        []string       `json:"enum"`
	Persistent  bool           `json:"persistent"`
	Extensions  map[string]any `json:"extensions"`
}

type docentFlagGroup struct {
	Kind  string   `json:"kind"`
	Flags []string `json:"flags"`
}

func loadAgentSchema(t *testing.T, args ...string) docentSchema {
	t.Helper()
	out, err := exec.Command(buildJiraBinary(t), append([]string{"agent", "schema"}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("agent schema error = %v\n%s", err, out)
	}
	var root docentSchema
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatalf("schema output is not JSON: %v\n%s", err, out)
	}
	return root
}

// loadAgentSchemaShapes loads the full tree with every schema body
// embedded (`--shapes`) and materializes the root $defs pool back into
// the bodies, so shape assertions read plain objects regardless of which
// bodies docent chose to pool.
func loadAgentSchemaShapes(t *testing.T) docentSchema {
	t.Helper()
	root := loadAgentSchema(t, "--shapes")
	materializeSchemaRefs(&root, root.Defs)
	return root
}

// materializeSchemaRefs substitutes {"$ref": "#/$defs/<name>"} objects
// with their pooled bodies, recursing through the whole tree.
func materializeSchemaRefs(cmd *docentSchema, defs map[string]any) {
	cmd.InputSchema, _ = resolveSchemaRefs(cmd.InputSchema, defs).(map[string]any)
	cmd.OutputSchema, _ = resolveSchemaRefs(cmd.OutputSchema, defs).(map[string]any)
	for i := range cmd.Children {
		materializeSchemaRefs(&cmd.Children[i], defs)
	}
}

func resolveSchemaRefs(v any, defs map[string]any) any {
	switch val := v.(type) {
	case map[string]any:
		if ref, ok := val["$ref"].(string); ok && len(val) == 1 {
			name := strings.TrimPrefix(ref, "#/$defs/")
			if body, ok := defs[name]; ok {
				return resolveSchemaRefs(body, defs)
			}
		}
		out := make(map[string]any, len(val))
		for k, child := range val {
			out[k] = resolveSchemaRefs(child, defs)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, child := range val {
			out[i] = resolveSchemaRefs(child, defs)
		}
		return out
	default:
		return v
	}
}

func findSchemaCommand(cmd docentSchema, path string) *docentSchema {
	if cmd.Path == path {
		return &cmd
	}
	for _, child := range cmd.Children {
		if found := findSchemaCommand(child, path); found != nil {
			return found
		}
	}
	return nil
}

func findSchemaFlag(flags []docentSchemaFlag, name string) *docentSchemaFlag {
	for i := range flags {
		if flags[i].Name == name {
			return &flags[i]
		}
	}
	return nil
}

func hasSchemaFlagGroup(groups []docentFlagGroup, kind string, want ...string) bool {
	for _, group := range groups {
		if group.Kind != kind || len(group.Flags) != len(want) {
			continue
		}
		seen := make(map[string]bool, len(group.Flags))
		for _, flag := range group.Flags {
			seen[flag] = true
		}
		ok := true
		for _, flag := range want {
			if !seen[flag] {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// clibExtension digs the clib per-flag extension map out of a flag's
// extensions entry.
func clibExtension(flag *docentSchemaFlag) map[string]any {
	if flag == nil {
		return nil
	}
	clib, _ := flag.Extensions["clib"].(map[string]any)
	return clib
}

// clibFlagView flattens a docent flag and its clib extension entry into
// the fields the metadata contract tests assert on, so the per-family
// want tables read the same as they did against the legacy schema.
type clibFlagView struct {
	Name        string
	Group       string
	Placeholder string
	Completion  string
	ValueHint   string
	Terse       string
	EnumDefault string
	Enum        []string
	EnumTerse   []string
}

func clibViewOf(flag docentSchemaFlag) clibFlagView {
	view := clibFlagView{Name: flag.Name}
	ext := clibExtension(&flag)
	str := func(key string) string {
		s, _ := ext[key].(string)
		return s
	}
	strs := func(key string) []string {
		raw, _ := ext[key].([]any)
		if raw == nil {
			return nil
		}
		out := make([]string, 0, len(raw))
		for _, v := range raw {
			s, _ := v.(string)
			out = append(out, s)
		}
		return out
	}
	view.Group = str("group")
	view.Placeholder = str("placeholder")
	view.Completion = str("complete")
	view.ValueHint = str("hint")
	view.Terse = str("terse")
	view.EnumDefault = str("enumDefault")
	view.EnumTerse = strs("enumTerse")
	view.Enum = enumOf(flag)
	return view
}

// enumOf reads the flag's top-level enum list from the raw schema flag.
func enumOf(flag docentSchemaFlag) []string {
	return flag.Enum
}

// findClibView resolves one named flag (leading dashes tolerated) on a
// command into its flattened clib view.
func findClibView(flags []docentSchemaFlag, name string) *clibFlagView {
	flag := findSchemaFlag(flags, strings.TrimLeft(name, "-"))
	if flag == nil {
		return nil
	}
	view := clibViewOf(*flag)
	return &view
}

func TestSchemaCommandIncludesCommandTree(t *testing.T) {
	root := loadAgentSchema(t)
	if root.Name != "jira" || len(root.Children) == 0 {
		t.Fatalf("schema missing command tree: name=%q children=%d", root.Name, len(root.Children))
	}
}

// The schema root must carry the contract revision so an agent can detect
// a breaking change before reusing saved recipes.
func TestSchemaCommandReportsContractVersion(t *testing.T) {
	root := loadAgentSchema(t)
	version, _ := root.Extensions["contract_version"].(string)
	semver := regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	if !semver.MatchString(version) {
		t.Fatalf("extensions.contract_version = %q, want MAJOR.MINOR.PATCH semver", version)
	}
}

func TestSchemaCommandIncludesDetailedFlagSignatures(t *testing.T) {
	root := loadAgentSchema(t)
	profile := findSchemaFlag(root.Flags, "profile")
	if profile == nil {
		t.Fatalf("schema missing root --profile flag: %+v", root.Flags)
	}
	if profile.Type != "string" || profile.Shorthand != "P" || profile.Description == "" || !profile.Persistent {
		t.Fatalf("profile flag signature incomplete: %+v", *profile)
	}
}

func TestAgentSchemaPublishesLiveLeafPathsAndFlagGroups(t *testing.T) {
	root := loadAgentSchemaShapes(t)

	outputClib := clibExtension(findSchemaFlag(root.Flags, "output"))
	if outputClib == nil {
		t.Fatalf("agent schema missing global --output flag with clib extension")
	}
	enumTerse, _ := outputClib["enumTerse"].([]any)
	if outputClib["enumDefault"] != "auto" || len(enumTerse) != 4 {
		t.Fatalf("agent schema dropped --output enum metadata: %+v", outputClib)
	}

	issueList := findSchemaCommand(root, "jira issue list")
	if issueList == nil {
		t.Fatalf("agent schema missing leaf path %q", "jira issue list")
	}
	for _, want := range []string{"board", "board-id", "key", "parallelism"} {
		if findSchemaFlag(issueList.Flags, want) == nil {
			t.Fatalf("jira issue list schema missing local flag --%s", want)
		}
	}
	if findSchemaFlag(issueList.Flags, "output") != nil {
		t.Fatalf("jira issue list duplicated global --output in local flags")
	}
	if !hasSchemaFlagGroup(issueList.FlagGroups, "mutually_exclusive", "board", "board-id") {
		t.Fatalf("jira issue list missing board mutex group: %+v", issueList.FlagGroups)
	}

	issueView := findSchemaCommand(root, "jira issue view")
	if issueView == nil {
		t.Fatalf("agent schema missing leaf path %q", "jira issue view")
	}
	if findSchemaFlag(issueView.Flags, "parallelism") == nil {
		t.Fatalf("jira issue view schema missing local flag --parallelism")
	}
	if _, ok := issueView.OutputSchema["properties"]; !ok {
		t.Fatalf("jira issue view output schema missing or stub: %+v", issueView.OutputSchema)
	}

	for _, path := range []string{
		"jira epic add",
		"jira epic remove",
		"jira issue attachment add",
		"jira issue attachment list",
		"jira issue clone",
		"jira issue comment",
		"jira issue comment add",
		"jira issue comment list",
		"jira issue delete",
		"jira issue edit",
		"jira issue link",
		"jira issue link list",
		"jira issue move",
		"jira issue transition",
		"jira issue unwatch",
		"jira issue watchers list",
		"jira issue watch",
		"jira issue watchers add",
		"jira issue watchers remove",
		"jira issue weblink",
		"jira worklog add",
		"jira worklog list",
	} {
		cmd := findSchemaCommand(root, path)
		if cmd == nil {
			t.Fatalf("agent schema missing leaf path %q", path)
		}
		if findSchemaFlag(cmd.Flags, "parallelism") == nil {
			t.Fatalf("%s schema missing local flag --parallelism", path)
		}
	}

	jqlBuild := findSchemaCommand(root, "jira jql build")
	if jqlBuild == nil {
		t.Fatalf("agent schema missing leaf path %q", "jira jql build")
	}
	if findSchemaFlag(jqlBuild.Flags, "key") == nil {
		t.Fatalf("jira jql build schema missing local flag --key")
	}

	issueLink := findSchemaCommand(root, "jira issue link")
	if issueLink == nil {
		t.Fatalf("agent schema missing leaf path %q", "jira issue link")
	}
	if !hasSchemaFlagGroup(issueLink.FlagGroups, "required_together", "to", "type") {
		t.Fatalf("jira issue link missing required-together to/type group: %+v", issueLink.FlagGroups)
	}

	issueCreate := findSchemaCommand(root, "jira issue create")
	if issueCreate == nil {
		t.Fatalf("agent schema missing leaf path %q", "jira issue create")
	}
	inProps, _ := issueCreate.InputSchema["properties"].(map[string]any)
	outProps, _ := issueCreate.OutputSchema["properties"].(map[string]any)
	if inProps["project_key"] == nil || outProps["issue"] == nil {
		t.Fatalf("jira issue create schema bindings look mis-keyed: input has project_key=%v, output has issue=%v",
			inProps["project_key"] != nil, outProps["issue"] != nil)
	}
}

// TestAgentSchemaBindsLeafInputAndOutputSchemas pins the leaf bindings an
// agent introspects: the comment add/edit leaves (the commands that
// actually take --json-input) must embed an input schema — not just the
// runnable group alias — and the read/list verbs must embed an output
// schema so the response shape is discoverable from the schema surface
// alone.
func TestAgentSchemaBindsLeafInputAndOutputSchemas(t *testing.T) {
	// The default tree is structure-only: shape markers, no bodies. Pin
	// that policy once here, then assert the bodies on the --shapes form.
	structure := loadAgentSchema(t)
	marker := findSchemaCommand(structure, "jira issue comment add")
	if marker == nil || !marker.HasInputSchema || !marker.HasOutputSchema ||
		marker.InputSchema != nil || marker.OutputSchema != nil {
		t.Fatalf("default tree must carry both shape markers without bodies: %+v", marker)
	}

	root := loadAgentSchemaShapes(t)
	for _, path := range []string{
		"jira issue comment",
		"jira issue comment add",
		"jira issue comment edit",
	} {
		cmd := findSchemaCommand(root, path)
		if cmd == nil {
			t.Fatalf("agent schema missing path %q", path)
		}
		if cmd.InputSchema == nil {
			t.Errorf("%s missing embedded input schema", path)
		}
	}
	// Each leaf must embed the *right* schema, not just any schema: the
	// named property is distinctive to that operation's Output struct, so
	// a mis-keyed registry entry fails here instead of passing on
	// presence alone.
	for path, distinctive := range map[string]string{
		"jira issue comment list":    "comments",
		"jira issue attachment list": "attachments",
		"jira issue link list":       "links",
		"jira issue link types":      "count",
		"jira issue transition":      "transition",
		"jira worklog list":          "worklogs",
		"jira worklog add":           "worklog",
		"jira boards list":           "boards",
		"jira cache labels":          "cache_state",
		"jira cache linktypes":       "cache_state",
		"jira cache boards":          "boards_count",
		"jira cache refresh":         "results",
		"jira cache clear":           "removed",
	} {
		cmd := findSchemaCommand(root, path)
		if cmd == nil {
			t.Fatalf("agent schema missing path %q", path)
		}
		props, ok := cmd.OutputSchema["properties"].(map[string]any)
		if !ok {
			t.Errorf("%s output schema missing or stub: %+v", path, cmd.OutputSchema)
			continue
		}
		if props[distinctive] == nil {
			t.Errorf("%s output schema lacks its distinctive property %q — wrong schema bound?", path, distinctive)
		}
	}
}
