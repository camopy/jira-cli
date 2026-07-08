// MOTIVATION: two envelope writers once bypassed the shared clog printer path
// (cli.WriteEnvelope) by calling json.NewEncoder(...).Encode() directly. The
// clog path carries the errWriter wrapper that captures broken-pipe / quota
// write failures and pins the documented envelope byte-shape; a raw encoder
// silently loses those write errors and can drift in formatting. Envelope
// output in the command + envelope-machinery layer must go through
// cli.WriteEnvelope / WriteCompact / WriteHumanJSON, never a raw json.Encoder.
package guardrails

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// envelopeWriterDirs are the layers that build and emit envelopes. json.go
// itself is the sanctioned clog wrapper and is exempt.
var envelopeWriterDirs = []string{"../../internal/cli/cmdutil", "../../internal/cli"}

func TestNoRawJSONEncoderInEnvelopeLayer(t *testing.T) {
	var offenders []string
	for _, dir := range envelopeWriterDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("ReadDir(%s) error = %v", dir, err)
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			body, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatalf("ReadFile(%s) error = %v", name, err)
			}
			if strings.Contains(string(body), "json.NewEncoder") {
				offenders = append(offenders, filepath.Join(dir, name))
			}
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("envelope output must route through cli.WriteEnvelope/WriteCompact/WriteHumanJSON, not a raw json.NewEncoder:\n%s",
			strings.Join(offenders, "\n"))
	}
}
