package adf

import (
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli/adfmode"
)

func TestDetectDialect(t *testing.T) {
	tests := map[string]struct {
		input string
		want  mdDialect
	}{
		"plain prose defaults to CommonMark": {
			input: "just a sentence\n\nand another paragraph",
			want:  dialectCommonMark,
		},
		"empty input defaults to CommonMark": {
			input: "",
			want:  dialectCommonMark,
		},
		"CommonMark heading": {
			input: "## Title\n\nbody",
			want:  dialectCommonMark,
		},
		"CommonMark fence": {
			input: "```go\nfmt.Println()\n```",
			want:  dialectCommonMark,
		},
		"CommonMark strong emphasis": {
			input: "some **bold** text",
			want:  dialectCommonMark,
		},
		"CommonMark inline link": {
			input: "see [docs](https://example.com)",
			want:  dialectCommonMark,
		},
		"CommonMark image": {
			input: "![alt](https://example.com/x.png)",
			want:  dialectCommonMark,
		},
		"CommonMark blockquote": {
			input: "> quoted line",
			want:  dialectCommonMark,
		},
		"GFM table separator": {
			input: "| a | b |\n|---|---|\n| 1 | 2 |",
			want:  dialectCommonMark,
		},
		"wiki heading": {
			input: "h2. Background\n\nbody text",
			want:  dialectWiki,
		},
		"wiki table header row": {
			input: "||Name||Value||\n|a|1|",
			want:  dialectWiki,
		},
		"wiki monospace": {
			input: "run {{go test}} locally",
			want:  dialectWiki,
		},
		"wiki link": {
			input: "see [the docs|https://example.com]",
			want:  dialectWiki,
		},
		"wiki noformat block": {
			input: "{noformat}\nraw output\n{noformat}",
			want:  dialectWiki,
		},
		"wiki code block with language": {
			input: "{code:java}\nclass A {}\n{code}",
			want:  dialectWiki,
		},
		"mixed document: CommonMark wins": {
			// A wiki heading next to a CommonMark fence: rewriting risks
			// mangling real Markdown, so the ambiguity resolves to leaving
			// everything alone.
			input: "h2. Title\n\n```go\ncode\n```",
			want:  dialectCommonMark,
		},
		"mixed inline: strong emphasis pins CommonMark": {
			input: "h2. Title\n\nwith **bold** prose",
			want:  dialectCommonMark,
		},
		"single leading hash is not a CommonMark signal": {
			// `# item` is a wiki ordered list; only ##+ headings are
			// unambiguous. With a wiki signal present, this is wiki.
			input: "h2. Steps\n\n# first\n# second",
			want:  dialectWiki,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := detectDialect(tc.input); got != tc.want {
				t.Fatalf("detectDialect(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// TestCommonMarkCorpusNeverNormalized is the byte-identity guarantee: for a
// representative corpus of CommonMark documents the detector must classify
// CommonMark, which routes them around the wiki scanner entirely — the
// converter sees the exact bytes the author wrote.
func TestCommonMarkCorpusNeverNormalized(t *testing.T) {
	corpus := []string{
		"plain text",
		"# Single hash heading only\n\nprose",
		"## Heading\n\n- list\n- items\n\n**bold** and *italic* and `code`",
		"```python\nprint('*not bold*')\n```",
		"[link](https://example.com) and ![img](https://example.com/i.png)",
		"> quote\n\n1. ordered\n2. list",
		"| a | b |\n|---|---|\n| 1 | 2 |",
		"para with *emphasis* only", // single-star = CM italic; no wiki signal
		"~~strike~~ and <https://example.com>",
	}
	for _, input := range corpus {
		if detectDialect(input) != dialectCommonMark {
			t.Fatalf("CommonMark corpus document misdetected as wiki:\n%s", input)
		}
	}
}

func TestNormalizeWikiMarkupConstructs(t *testing.T) {
	tests := map[string]struct {
		input string
		want  string
	}{
		"headings h1 through h6": {
			input: "h1. One\nh3. Three\nh6. Six",
			want:  "# One\n### Three\n###### Six",
		},
		"ordered list items": {
			input: "h2. Steps\n\n# first\n# second",
			want:  "## Steps\n\n1. first\n1. second",
		},
		"bold": {
			input: "h2. T\n\nthis is *important* here",
			want:  "## T\n\nthis is **important** here",
		},
		"link": {
			input: "see [the docs|https://example.com/page]",
			want:  "see [the docs](https://example.com/page)",
		},
		"monospace": {
			input: "run {{go test ./...}} first",
			want:  "run `go test ./...` first",
		},
		"table grows separator row": {
			input: "||Name||Value||\n|a|1|",
			want:  "|Name|Value|\n|---|---|\n|a|1|",
		},
		"noformat becomes text fence": {
			input: "{noformat}\nh1. not a heading\n*not bold*\n{noformat}",
			want:  "```text\nh1. not a heading\n*not bold*\n```",
		},
		"code block carries language": {
			input: "{code:java}\nclass A {}\n{code}",
			want:  "```java\nclass A {}\n```",
		},
		"code block with params keeps only a bare language": {
			input: "{code:title=A.java|borderStyle=solid}\nx\n{code}",
			want:  "```\nx\n```",
		},
		"wiki syntax inside noformat stays literal": {
			input: "{noformat}\n{{not code}}\n[not|a link]\n{noformat}",
			want:  "```text\n{{not code}}\n[not|a link]\n```",
		},
		"bold and links inside converted monospace stay literal": {
			input: "{{*cmd* --flag}} then *real bold*",
			want:  "`*cmd* --flag` then **real bold**",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := normalizeWikiMarkup(tc.input)
			if got.text != tc.want {
				t.Fatalf("normalizeWikiMarkup(%q)\n got: %q\nwant: %q", tc.input, got.text, tc.want)
			}
			if len(got.constructs) == 0 {
				t.Fatalf("normalization must report which constructs it rewrote")
			}
		})
	}
}

// TestFromMarkdownWikiDocumentEndToEnd feeds a whole wiki-markup document
// through the public conversion path and pins the resulting ADF shape, the
// informational dialect warning, and strict-mode validity.
func TestFromMarkdownWikiDocumentEndToEnd(t *testing.T) {
	input := strings.Join([]string{
		"h2. Rollout",
		"",
		"Deploy with {{helm upgrade}} and check [the runbook|https://example.com/rb].",
		"",
		"||Step||Owner||",
		"|prep|ops|",
		"",
		"{code:bash}",
		"kubectl get pods",
		"{code}",
		"",
		"(/) done and (x) failed",
	}, "\n")

	doc, warnings, err := FromMarkdownLossy(input)
	if err != nil {
		t.Fatalf("FromMarkdownLossy() error = %v", err)
	}

	var dialectWarned bool
	for _, w := range warnings {
		if w.Type == "markdown_dialect_normalized" {
			dialectWarned = true
			if w.Lossy {
				t.Fatalf("dialect normalization must be informational, got Lossy=true: %+v", w)
			}
		}
		if w.Lossy {
			t.Fatalf("wiki conversion must not be lossy, got %+v", w)
		}
	}
	if !dialectWarned {
		t.Fatalf("expected a markdown_dialect_normalized warning, got %+v", warnings)
	}

	kinds := make([]string, 0, len(doc.Content))
	for _, n := range doc.Content {
		kinds = append(kinds, n.Type)
	}
	want := []string{"heading", "paragraph", "table", "codeBlock", "paragraph"}
	if strings.Join(kinds, ",") != strings.Join(want, ",") {
		t.Fatalf("block kinds = %v, want %v", kinds, want)
	}
	if lang := doc.Content[3].Attrs["language"]; lang != "bash" {
		t.Fatalf("code block language = %v, want bash", lang)
	}

	var emoji []string
	for _, n := range doc.Content[4].Content {
		if n.Type == "emoji" {
			emoji = append(emoji, n.Attrs["shortName"].(string))
		}
	}
	if strings.Join(emoji, ",") != ":check_mark:,:cross_mark:" {
		t.Fatalf("emoji shortNames = %v", emoji)
	}

	normalized, _ := Normalize(doc)
	if _, err := ValidateDoc(normalized, adfmode.ModeStrict); err != nil {
		t.Fatalf("converted wiki document failed strict validation: %v", err)
	}
}

func TestSplitWikiEmoji(t *testing.T) {
	t.Run("text fragments keep marks, emoji stay bare", func(t *testing.T) {
		marks := []Mark{{Type: "strong"}}
		nodes := splitWikiEmoji("ok (/) done", marks)
		if len(nodes) != 3 {
			t.Fatalf("want text,emoji,text; got %+v", nodes)
		}
		if nodes[0].Type != "text" || len(nodes[0].Marks) != 1 || nodes[0].Marks[0].Type != "strong" {
			t.Fatalf("leading text lost its marks: %+v", nodes[0])
		}
		if nodes[1].Type != "emoji" || nodes[1].Attrs["shortName"] != ":check_mark:" || len(nodes[1].Marks) != 0 {
			t.Fatalf("emoji node malformed: %+v", nodes[1])
		}
	})
	t.Run("no shortcut returns nil", func(t *testing.T) {
		if nodes := splitWikiEmoji("plain text", nil); nodes != nil {
			t.Fatalf("want nil for shortcut-free text, got %+v", nodes)
		}
	})
	t.Run("longest shortcut wins", func(t *testing.T) {
		nodes := splitWikiEmoji("sad </3 end", nil)
		if len(nodes) != 3 || nodes[1].Attrs["shortName"] != ":broken_heart:" {
			t.Fatalf("</3 must beat <3: %+v", nodes)
		}
	})
}

// TestWikiEmojiNeverFiresInCode pins the acceptance criterion that emoji
// expansion skips code: a shortcut inside converted {{monospace}} (a code
// span after normalization) and inside a {code} block must stay literal.
func TestWikiEmojiNeverFiresInCode(t *testing.T) {
	input := "h2. T\n\nuse {{grep \"(/)\"}} to find checks\n\n{code}\n(x) literal\n{code}\n\nreal (/) here"
	doc, _, err := FromMarkdownLossy(input)
	if err != nil {
		t.Fatalf("FromMarkdownLossy() error = %v", err)
	}
	var emojiCount int
	var walk func(nodes []Node)
	walk = func(nodes []Node) {
		for _, n := range nodes {
			switch n.Type {
			case "emoji":
				emojiCount++
			case "codeBlock":
				for _, c := range n.Content {
					if strings.Contains(c.Text, "(x) literal") {
						continue // literal shortcut preserved — correct
					}
				}
			}
			for _, c := range n.Content {
				if strings.Contains(c.Text, "(/)") && n.Type != "emoji" {
					// the code-span copy must still contain the shortcut
					hasCode := false
					for _, m := range c.Marks {
						if m.Type == "code" {
							hasCode = true
						}
					}
					if !hasCode && c.Type == "text" && strings.Contains(c.Text, "(/)") {
						t.Fatalf("unexpanded shortcut outside code: %+v", c)
					}
				}
			}
			walk(n.Content)
		}
	}
	walk(doc.Content)
	if emojiCount != 1 {
		t.Fatalf("want exactly 1 expanded emoji (the one outside code), got %d\ndoc: %+v", emojiCount, doc.Content)
	}
}

// TestWikiSoftBreakAfterEmojiStaysValid pins the inline-flow edge where a
// soft line break directly follows an expanded emoji: the joining space
// must land in its own text node, never as top-level text on the emoji.
func TestWikiSoftBreakAfterEmojiStaysValid(t *testing.T) {
	input := "h2. T\n\nfirst (/)\nsecond line"
	doc, _, err := FromMarkdownLossy(input)
	if err != nil {
		t.Fatalf("FromMarkdownLossy() error = %v", err)
	}
	var check func(nodes []Node)
	check = func(nodes []Node) {
		for _, n := range nodes {
			if n.Type == "emoji" && n.Text != "" {
				t.Fatalf("emoji node grew top-level text: %+v", n)
			}
			check(n.Content)
		}
	}
	check(doc.Content)
	normalized, _ := Normalize(doc)
	if _, err := ValidateDoc(normalized, adfmode.ModeStrict); err != nil {
		t.Fatalf("soft-break-after-emoji document failed strict validation: %v", err)
	}
}
