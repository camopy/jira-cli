package main

import (
	"bytes"
	"strings"
	"testing"
)

// The `agent guide` custom help func must write through the command's
// own output stream (cmd.OutOrStdout()), not os.Stdout. A help func that
// bypasses the command stream is untestable and ignores any redirect a
// caller installs.
func TestAgentGuideHelpWritesToCommandStream(t *testing.T) {
	cmd := agentGuideCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	// Invoke the help func directly — this is what `--help` triggers.
	cmd.Help() //nolint:errcheck // help func returns the stream write error, asserted via buf below

	got := buf.String()
	if got == "" {
		t.Fatal("agent guide --help produced no output on the command stream — help bypasses cmd.OutOrStdout()")
	}
	// The custom help appends a Sections block listing guide slugs.
	if !strings.Contains(strings.ToLower(got), "section") {
		t.Fatalf("agent guide help did not render the Sections block:\n%s", got)
	}
}
