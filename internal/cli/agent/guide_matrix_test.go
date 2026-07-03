package agent

import (
	"regexp"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/adf"
)

// The adf_reference guide's "full supported set" section is prose over the
// same facts the support matrix publishes. This test pins the two together:
// every mvp-tier matrix row must be named in the section, and every
// backticked token in the section that names a schema type must have a
// matrix row — so the guide can neither under- nor over-claim.
func TestGuideSupportedSetMatchesMatrix(t *testing.T) {
	raw, err := guideFS.ReadFile("guide/adf_reference.md")
	if err != nil {
		t.Fatal(err)
	}
	guide := string(raw)
	start := strings.Index(guide, "### The full supported set")
	end := strings.Index(guide, "### Block nodes")
	if start < 0 || end < 0 || end <= start {
		t.Fatal("adf_reference.md no longer carries the supported-set section markers")
	}
	section := guide[start:end]

	tokens := map[string]bool{}
	for _, m := range regexp.MustCompile("`([a-zA-Z]+)`").FindAllStringSubmatch(section, -1) {
		tokens[m[1]] = true
	}

	reg := adf.Registry()
	for _, entry := range reg.All() {
		if entry.Status != adf.StatusMVP {
			continue
		}
		if !tokens[entry.Name] {
			t.Errorf("matrix mvp row %q is not named in the guide's supported-set section", entry.Name)
		}
	}
	for token := range tokens {
		if _, node := reg.Lookup(adf.KindNode, token); node {
			continue
		}
		if _, mark := reg.Lookup(adf.KindMark, token); mark {
			continue
		}
		// Non-type tokens (command names, tier labels) are fine — only a
		// token that *looks like* a claim about ADF support must resolve.
		// Everything the section backticks in type position is camelCase or
		// a known lowercase type name; skip obvious non-types.
		switch token {
		case "mvp", "expand", "mediaSingle", "layoutSection", "doc":
			// tier labels and preserve-only examples (doc is curated; expand,
			// mediaSingle, layoutSection are named as preserve-only examples
			// and have synthesized rows — Lookup covers them, listed here only
			// if curation changes).
			continue
		}
		if strings.ToLower(token) != token { // camelCase → reads as a type claim
			t.Errorf("guide names %q in the supported-set section but the matrix has no such row", token)
		}
	}
}
