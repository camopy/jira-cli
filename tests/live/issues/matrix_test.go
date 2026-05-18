//go:build live

package issues

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/matcra587/jira-cli/tests/live/internal/livekit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type matrixRow struct {
	ID      string `yaml:"id"`
	Command string `yaml:"command"`
	Covered bool   `yaml:"covered"`
	Kind    string `yaml:"kind"`
	Cleanup string `yaml:"cleanup"`
	Notes   string `yaml:"notes,omitempty"`
	Reason  string `yaml:"reason,omitempty"`
}

type commandSchema struct {
	Name        string          `json:"name"`
	Subcommands []commandSchema `json:"subcommands,omitempty"`
}

type schemaPayload struct {
	Commands []commandSchema `json:"commands"`
}

func TestLiveMatrixCoversEveryIssueCommand(t *testing.T) {
	livekit.RequireProject(t)
	bin := livekit.BuildBinary(t)
	rows := readIssueMatrix(t)

	stdout, stderr, err := runSchemaCompact(t, bin)
	require.NoError(t, err, "jira agent schema --output=compact\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	var schema schemaPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &schema))

	leafCommands := issueDomainLeafCommands(schema)
	rowsByCommand := map[string][]matrixRow{}
	for _, row := range rows {
		rowsByCommand[row.Command] = append(rowsByCommand[row.Command], row)
		assert.NotEmpty(t, row.ID, "matrix row missing id")
		assert.Contains(t, []string{"mutating", "read", "destructive"}, row.Kind, "matrix row %s has invalid kind", row.ID)
		assert.Contains(t, []string{"delete", "survivor", "none"}, row.Cleanup, "matrix row %s has invalid cleanup", row.ID)
		if row.Covered {
			_, ok := liveCoveredMatrixIDs[row.ID]
			assert.True(t, ok, "covered matrix row %s has no live test reference", row.ID)
		} else {
			assert.NotEmpty(t, row.Reason, "uncovered matrix row %s must explain reason", row.ID)
		}
	}

	for _, command := range leafCommands {
		assert.NotEmpty(t, rowsByCommand[command], "schema issue-domain leaf %q has no matrix row", command)
	}
}

func readIssueMatrix(t *testing.T) []matrixRow {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(matrixDir(), "issues_matrix.yaml"))
	require.NoError(t, err)
	var rows []matrixRow
	require.NoError(t, yaml.Unmarshal(body, &rows))
	require.NotEmpty(t, rows)
	return rows
}

// matrixDir returns the directory holding this suite's source and
// issues_matrix.yaml, resolved relative to this file.
func matrixDir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("runtime.Caller failed")
	}
	return filepath.Dir(file)
}

func runSchemaCompact(t *testing.T, bin string) (string, string, error) {
	t.Helper()
	s := livekit.Suite{Bin: bin}
	return s.RunRaw(t, "agent", "schema", "--output=compact")
}

func issueDomainLeafCommands(schema schemaPayload) []string {
	var out []string
	for _, root := range schema.Commands {
		if root.Name != "jira" {
			continue
		}
		for _, child := range root.Subcommands {
			switch child.Name {
			case "issue", "worklog":
				out = append(out, leafCommands("jira "+child.Name, child)...)
			}
		}
	}
	sort.Strings(out)
	return out
}

func leafCommands(prefix string, cmd commandSchema) []string {
	if len(cmd.Subcommands) == 0 {
		return []string{prefix}
	}
	var out []string
	for _, child := range cmd.Subcommands {
		out = append(out, leafCommands(prefix+" "+child.Name, child)...)
	}
	return out
}
