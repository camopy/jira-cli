// MOTIVATION: rich-text fidelity regressions are a recurring class of
// upstream Jira CLI bug — common failure modes include underline
// being dropped, code blocks losing language, URLs with query params
// getting escaped, mention/link comments stripping marks. Comments in
// this file are PROVENANCE ONLY and MUST NOT be a source of
// implementation, fixtures, wording, or test logic.
package guardrails

import (
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/adf"
)

// Every supported mark/attr survives a Parse → Marshal cycle. Checks
// underline / textColor / backgroundColor — the three marks most
// often dropped by third-party converters.
func TestUnderlineTextColorBackgroundColorSurviveRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		json string
	}{
		{"underline", `{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"underline"}]}]}]}`},
		{"textColor", `{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"textColor","attrs":{"color":"#ff0000"}}]}]}]}`},
		{"backgroundColor", `{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"backgroundColor","attrs":{"color":"#fffacd"}}]}]}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc, _, err := adf.Parse([]byte(tc.json))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			out, err := adf.Marshal(doc)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if !strings.Contains(string(out), `"`+tc.name+`"`) {
				t.Fatalf("%s mark dropped on round-trip; got %s", tc.name, out)
			}
		})
	}
}

// URLs with query parameters MUST round-trip semantically through
// Parse → Marshal. We re-parse the output and compare the typed
// link's href, so JSON-level HTML-escaping of `&` (a stdlib default)
// doesn't false-fail the test — what matters is that a downstream
// JSON parser sees the same URL.
func TestURLWithQueryParamsSurvivesRoundTrip(t *testing.T) {
	url := "https://example.atlassian.net/browse/KAN-1?focusedCommentId=12&expand=renderedFields"
	doc, _, err := adf.Parse([]byte(`{"type":"doc","version":1,"content":[
		{"type":"paragraph","content":[
			{"type":"text","text":"see","marks":[{"type":"link","attrs":{"href":"` + url + `"}}]}
		]}
	]}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	out, err := adf.Marshal(doc)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	roundTripped, _, err := adf.Parse(out)
	if err != nil {
		t.Fatalf("re-Parse: %v", err)
	}
	link := roundTripped.Content[0].Content[0]
	if len(link.Marks) != 1 {
		t.Fatalf("link mark lost: %+v", link)
	}
	href, _ := link.Marks[0].Attrs["href"].(string)
	if href != url {
		t.Fatalf("URL with query params mangled.\n want: %s\n  got: %s", url, href)
	}
}

// UUID-like text must NOT be misclassified as a code-block or any
// other special node by a Markdown-style converter. The text node
// carrying a UUID survives intact.
func TestUUIDLikeTextNotMisclassified(t *testing.T) {
	uuid := "550e8400-e29b-41d4-a716-446655440000"
	doc, _, err := adf.Parse([]byte(`{"type":"doc","version":1,"content":[
		{"type":"paragraph","content":[{"type":"text","text":"id ` + uuid + `"}]}
	]}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	first := doc.Content[0]
	if first.Type != "paragraph" {
		t.Fatalf("UUID-bearing text classified as %q, want paragraph", first.Type)
	}
	if !strings.Contains(first.Content[0].Text, uuid) {
		t.Fatalf("UUID lost: %v", first.Content[0])
	}
}

// Mention nodes round-trip with their accountId attr.
func TestMentionAccountIDRoundTrip(t *testing.T) {
	doc, _, err := adf.Parse([]byte(`{"type":"doc","version":1,"content":[
		{"type":"paragraph","content":[
			{"type":"text","text":"hi "},
			{"type":"mention","attrs":{"id":"5b10ac8d82e05b22cc7d4ef5","text":"@matcra587"}}
		]}
	]}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	out, err := adf.Marshal(doc)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(out), "5b10ac8d82e05b22cc7d4ef5") {
		t.Fatalf("mention accountId dropped: %s", out)
	}
}
