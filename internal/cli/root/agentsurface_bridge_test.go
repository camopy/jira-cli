package root

import (
	"encoding/json"
	"slices"
	"testing"

	clib "github.com/gechr/clib/cli/cobra"
	"github.com/gechr/clib/complete"
)

// TestDocentCompletionsBridgeToClibEnum pins that mountAgentSurface copies
// docent's cobra-registered flag completions into clib enum metadata, so the
// clib-driven completion path offers them. Without the bridge these
// completions are silently dead: jira-cli never invokes cobra's completion
// callbacks.
func TestDocentCompletionsBridgeToClibEnum(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path []string
		flag string
		want []string
	}{
		{
			name: "agent export format",
			path: []string{"agent", "export"},
			flag: "format",
			want: []string{"agent-skill", "claude-skill"},
		},
		{
			name: "agent export scope",
			path: []string{"agent", "export"},
			flag: "scope",
			want: []string{"project", "user"},
		},
		{
			name: "agent export harness",
			path: []string{"agent", "export"},
			flag: "harness",
			want: []string{"claude-code", "codex"},
		},
		{
			name: "agent guide section",
			path: []string{"agent", "guide"},
			flag: "section",
			want: []string{"Decide", "Run", "Save", "Preconditions", "Recover", "Next"},
		},
		{
			name: "human guide section",
			path: []string{"guide"},
			flag: "section",
			want: []string{"Decide", "Run", "Save", "Preconditions", "Recover", "Next"},
		},
		{
			name: "agent schema path",
			path: []string{"agent", "schema"},
			flag: "path",
			want: []string{"jira", "jira issue create"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmd, _, err := NewRootCommandForTest()
			if err != nil {
				t.Fatalf("build root: %v", err)
			}

			sub, _, err := cmd.Find(tt.path)
			if err != nil {
				t.Fatalf("find %v: %v", tt.path, err)
			}
			flag := sub.Flags().Lookup(tt.flag)
			if flag == nil {
				t.Fatalf("flag --%s not found on %v", tt.flag, tt.path)
			}

			raw := flag.Annotations["clib.extra"]
			if len(raw) == 0 {
				t.Fatalf("flag --%s carries no clib extra metadata", tt.flag)
			}
			var extra clib.FlagExtra
			if err := json.Unmarshal([]byte(raw[0]), &extra); err != nil {
				t.Fatalf("unmarshal clib extra: %v", err)
			}

			for _, value := range tt.want {
				if !slices.Contains(extra.Enum, value) {
					t.Errorf("enum for --%s = %v, missing %q", tt.flag, extra.Enum, value)
				}
			}
			if len(extra.EnumTerse) != len(extra.Enum) {
				t.Fatalf(
					"enum descriptions for --%s = %d, want one for each of %d values",
					tt.flag,
					len(extra.EnumTerse),
					len(extra.Enum),
				)
			}
			for i, description := range extra.EnumTerse {
				if description == "" {
					t.Errorf("enum description for --%s value %q is empty", tt.flag, extra.Enum[i])
				}
			}
		})
	}
}

func TestCompletionGeneratorIncludesHiddenAgentExportEnums(t *testing.T) {
	t.Parallel()

	cmd, _, err := NewRootCommandForTest()
	if err != nil {
		t.Fatalf("build root: %v", err)
	}

	agentCmd, _, err := cmd.Find([]string{"agent"})
	if err != nil {
		t.Fatalf("find agent command: %v", err)
	}
	if !agentCmd.Hidden {
		t.Fatal("agent command must remain hidden from human help")
	}

	export := findCompletionSubcommand(t, completionGenerator(cmd).Subs, "agent", "export")
	if !agentCmd.Hidden {
		t.Fatal("completion generation left the agent command visible")
	}

	tests := []struct {
		flag string
		want []string
	}{
		{flag: "format", want: []string{"agent-skill", "claude-skill"}},
		{flag: "scope", want: []string{"project", "user"}},
		{flag: "harness", want: []string{"claude-code", "codex"}},
	}
	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			for _, spec := range export.Specs {
				if spec.LongFlag == tt.flag {
					got := make([]string, len(spec.ValueDescs))
					for i, value := range spec.ValueDescs {
						got[i] = value.Value
						if value.Desc == "" {
							t.Errorf("--%s completion description for %q is empty", tt.flag, value.Value)
						}
					}
					if !slices.Equal(got, tt.want) {
						t.Fatalf("--%s completion values = %v, want %v", tt.flag, got, tt.want)
					}
					return
				}
			}
			t.Fatalf("completion tree has no --%s flag", tt.flag)
		})
	}
}

func findCompletionSubcommand(t *testing.T, subs []complete.SubSpec, path ...string) complete.SubSpec {
	t.Helper()

	for i, name := range path {
		found := false
		for _, sub := range subs {
			if sub.Name == name {
				subs = sub.Subs
				if i == len(path)-1 {
					return sub
				}
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("completion tree has no command path %v", path)
		}
	}

	t.Fatalf("completion command path is empty")
	return complete.SubSpec{}
}
