package adf

// Fuzz test for ToMarkdownLossy.
//
// The fuzzer hammers the renderer + lossy detector with arbitrary JSON
// blobs. Two invariants must hold for every input:
//
//   1. ToMarkdownLossy must not panic.
//   2. Every node/mark type that escapes the supported set
//      (renderableMarkdownNodes / renderableMarkdownMarks) must appear
//      in LossyConstructs — silent drops would hide downgrade signals.

import (
	"encoding/json"
	"slices"
	"sort"
	"testing"
)

// FuzzToMarkdownLossy feeds arbitrary JSON to adf.Parse and, when parse
// succeeds, exercises the lossy detector. Inputs that don't decode are
// skipped (the Parse contract is tested elsewhere).
func FuzzToMarkdownLossy(f *testing.F) {
	// Seed with a handful of representative ADF shapes so the corpus
	// starts from coverage-rich inputs rather than pure noise.
	seeds := []string{
		`{"type":"doc","version":1,"content":[]}`,
		`{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"hi"}]}]}`,
		`{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"strong"}]}]}]}`,
		`{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"inlineCard","attrs":{"url":"https://example.com"}}]}]}`,
		`{"type":"doc","version":1,"content":[{"type":"panel","attrs":{"panelType":"info"},"content":[{"type":"paragraph","content":[{"type":"text","text":"note"}]}]}]}`,
		`{"type":"doc","version":1,"content":[{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"a"}]}]}]}]}`,
		`{"type":"doc","version":1,"content":[{"type":"codeBlock","attrs":{"language":"go"},"content":[{"type":"text","text":"package x"}]}]}`,
		// Lightly-malformed shape that still parses — surface for the
		// "every unsupported type lands in LossyConstructs" invariant.
		`{"type":"doc","version":1,"content":[{"type":"weirdNode","content":[]}]}`,
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, payload []byte) {
		doc, _, err := Parse(payload)
		if err != nil {
			return // not an ADF doc; nothing to assert
		}
		// Invariant 1: never panic, regardless of input shape.
		result := ToMarkdownLossy(doc)

		// Invariant 2: every node / mark type outside the renderer's
		// explicit switch must surface in LossyConstructs.
		expected := make(map[string]struct{})
		walkADF(doc, expected)
		for k := range expected {
			if !contains(result.LossyConstructs, k) {
				t.Fatalf("unsupported construct %q not reported in LossyConstructs %v", k, result.LossyConstructs)
			}
		}
		// LossyConstructs must be sorted unique (per the package contract).
		if !sort.StringsAreSorted(result.LossyConstructs) {
			t.Fatalf("LossyConstructs not sorted: %v", result.LossyConstructs)
		}
		seen := make(map[string]struct{}, len(result.LossyConstructs))
		for _, c := range result.LossyConstructs {
			if _, dup := seen[c]; dup {
				t.Fatalf("LossyConstructs has duplicate %q: %v", c, result.LossyConstructs)
			}
			seen[c] = struct{}{}
		}
	})
}

// walkADF recursively collects every node/mark type that is NOT in the
// supported renderer switch. Mirrors collectLossy but kept separate so
// the test never depends on the implementation detail it's verifying.
func walkADF(doc Document, out map[string]struct{}) {
	for _, n := range doc.Content {
		walkNode(n, out)
	}
}

func walkNode(n Node, out map[string]struct{}) {
	if n.Type != "" && !renderableMarkdownNodes[n.Type] {
		out[n.Type] = struct{}{}
	}
	for _, m := range n.Marks {
		if m.Type == "" {
			continue
		}
		if !renderableMarkdownMarks[m.Type] {
			out[m.Type] = struct{}{}
		}
	}
	for _, c := range n.Content {
		walkNode(c, out)
	}
}

func contains(list []string, want string) bool {
	return slices.Contains(list, want)
}

// Smoke compile guard: the fuzz harness above wires through adf.Parse,
// which means encoding/json must remain a valid dependency. Without this
// import-side reference goimports would yank it on a stray re-format.
var _ = json.Valid
