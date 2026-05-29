package contract

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestAgentGuideCommandFlagsExistOnHelpSurface(t *testing.T) {
	bin := buildJiraBinary(t)
	schema := loadCommandSurfaceSchema(t, bin)
	globalFlags := schemaGlobalFlags(schema)
	commands := schemaCommandsByPath(schema)
	paths := sortedCommandPaths(commands)
	helpCache := map[string]string{}

	entries, err := os.ReadDir(agentGuideDir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", agentGuideDir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(agentGuideDir, entry.Name())
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", path, err)
		}
		for _, raw := range guideJiraCommandExamples(string(body)) {
			tokens := shellishFields(raw)
			if len(tokens) == 0 || tokens[0] != "jira" {
				continue
			}
			cmdPath, cmdTokens := resolveGuideCommandPath(tokens[1:], paths)
			if cmdPath == "" {
				continue
			}
			cmdSchema := commands[cmdPath]
			remaining := tokens[1+cmdTokens:]
			if len(cmdSchema.Flags) == 0 && len(remaining) > 0 && !strings.HasPrefix(remaining[0], "-") {
				continue
			}
			for _, flag := range flagsUsedAfterCommandPath(remaining, globalFlags) {
				if !commandSchemaHasFlag(cmdSchema, flag) {
					t.Fatalf("%s guide command %q uses %s, but schema for %q does not expose it",
						entry.Name(), raw, flag, "jira "+cmdPath)
				}
				helpText := helpCache[cmdPath]
				if helpText == "" {
					helpText = commandHelp(t, bin, cmdPath)
					helpCache[cmdPath] = helpText
				}
				if !strings.Contains(helpText, flag) {
					t.Fatalf("%s guide command %q uses %s, but %q --help does not show it\n%s",
						entry.Name(), raw, flag, "jira "+cmdPath, helpText)
				}
			}
		}
	}
}

type guideSchema struct {
	Commands    []guideSchemaCommand `json:"commands"`
	GlobalFlags []guideSchemaFlag    `json:"global_flags"`
}

type guideSchemaCommand struct {
	CommandPath string               `json:"command_path"`
	Flags       []guideSchemaFlag    `json:"flags"`
	Subcommands []guideSchemaCommand `json:"subcommands"`
}

type guideSchemaFlag struct {
	Name      string `json:"name"`
	Shorthand string `json:"shorthand"`
}

func loadCommandSurfaceSchema(t *testing.T, bin string) guideSchema {
	t.Helper()
	cmd := exec.Command(bin, "agent", "schema", "--output=compact")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("agent schema error = %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	var schema guideSchema
	if err := json.Unmarshal(stdout.Bytes(), &schema); err != nil {
		t.Fatalf("agent schema JSON error = %v\nstdout=%s", err, stdout.String())
	}
	return schema
}

func schemaGlobalFlags(schema guideSchema) map[string]struct{} {
	out := make(map[string]struct{}, len(schema.GlobalFlags)*2)
	for _, flag := range schema.GlobalFlags {
		out[flag.Name] = struct{}{}
		if flag.Shorthand != "" {
			out["-"+flag.Shorthand] = struct{}{}
		}
	}
	return out
}

func schemaCommandsByPath(schema guideSchema) map[string]guideSchemaCommand {
	out := map[string]guideSchemaCommand{}
	var visit func(guideSchemaCommand)
	visit = func(cmd guideSchemaCommand) {
		path := strings.TrimSpace(strings.TrimPrefix(cmd.CommandPath, "jira"))
		if path != "" {
			out[path] = cmd
		}
		for _, child := range cmd.Subcommands {
			visit(child)
		}
	}
	for _, cmd := range schema.Commands {
		visit(cmd)
	}
	return out
}

func sortedCommandPaths(commands map[string]guideSchemaCommand) []string {
	paths := make([]string, 0, len(commands))
	for path := range commands {
		paths = append(paths, path)
	}
	sort.Slice(paths, func(i, j int) bool {
		return len(strings.Fields(paths[i])) > len(strings.Fields(paths[j]))
	})
	return paths
}

func commandSchemaHasFlag(cmd guideSchemaCommand, want string) bool {
	for _, flag := range cmd.Flags {
		if flag.Name == want {
			return true
		}
		if flag.Shorthand != "" && want == "-"+flag.Shorthand {
			return true
		}
	}
	return false
}

func commandHelp(t *testing.T, bin, cmdPath string) string {
	t.Helper()
	args := append(strings.Fields(cmdPath), "--help")
	cmd := exec.Command(bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s --help error = %v\n%s", cmdPath, err, out)
	}
	return string(out)
}

var guideInlineCodeCommand = regexp.MustCompile("`([^`\\n]*\\bjira\\b[^`\\n]*)`")

func guideJiraCommandExamples(body string) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if !strings.HasPrefix(raw, "jira ") {
			return
		}
		if _, ok := seen[raw]; ok {
			return
		}
		seen[raw] = struct{}{}
		out = append(out, raw)
	}

	for _, match := range guideInlineCodeCommand.FindAllStringSubmatch(body, -1) {
		add(match[1])
	}
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(strings.TrimPrefix(line, "$ "))
		if strings.HasPrefix(trimmed, "jira ") {
			add(trimmed)
		}
	}
	return out
}

func shellishFields(raw string) []string {
	var fields []string
	var b strings.Builder
	var quote rune
	flush := func() {
		if b.Len() == 0 {
			return
		}
		fields = append(fields, strings.TrimRight(b.String(), ".,);"))
		b.Reset()
	}
	for _, r := range raw {
		if quote != 0 {
			if r == quote {
				quote = 0
				continue
			}
			b.WriteRune(r)
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
		case '|':
			flush()
			return fields
		case '#':
			flush()
			return fields
		case ' ', '\t', '\n':
			flush()
		default:
			b.WriteRune(r)
		}
	}
	flush()
	return fields
}

func resolveGuideCommandPath(tokens, paths []string) (string, int) {
	for _, path := range paths {
		parts := strings.Fields(path)
		if len(parts) > len(tokens) {
			continue
		}
		matched := true
		for i := range parts {
			if tokens[i] != parts[i] {
				matched = false
				break
			}
		}
		if matched {
			return path, len(parts)
		}
	}
	return "", 0
}

func flagsUsedAfterCommandPath(tokens []string, globalFlags map[string]struct{}) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, token := range tokens {
		if !strings.HasPrefix(token, "-") || token == "-" {
			continue
		}
		flag := token
		if i := strings.Index(flag, "="); i >= 0 {
			flag = flag[:i]
		}
		if strings.HasPrefix(flag, "---") {
			continue
		}
		if _, ok := globalFlags[flag]; ok {
			continue
		}
		if _, ok := seen[flag]; ok {
			continue
		}
		seen[flag] = struct{}{}
		out = append(out, flag)
	}
	return out
}
