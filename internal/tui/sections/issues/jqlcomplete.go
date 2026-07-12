// JQL completion engine for the search editor: a small tokenizer classifies
// where the cursor is in the query (field, operator, value, or connective
// position) and builds full-line candidates from the instance's autocomplete
// reference data, so the textinput's whole-line ghost suggestion completes
// just the token being typed. Pure functions — the async parts (fetching the
// reference data and live field values) live in the search section.

package issues

import (
	"sort"
	"strings"

	xslices "github.com/gechr/x/slices"
	xstrings "github.com/gechr/x/strings"

	"github.com/matcra587/jira-cli/internal/jira"
)

// tokenKind is what the token under construction at the end of the input is
// expected to be.
type tokenKind int

const (
	wantField tokenKind = iota
	wantOperator
	wantValue
	wantConnective // AND / OR / ORDER BY after a complete clause
	wantDirection  // ASC / DESC after an ORDER BY field
	wantSortMore   // after a sort direction: only another sort field may follow
)

// jqlContext describes the completion point: the kind of token being typed,
// the prefix typed so far (without any opening quote), where that token
// starts in the input, the field the token belongs to (operator/value
// positions), and whether the value prefix was opened with a quote.
type jqlContext struct {
	kind   tokenKind
	prefix string
	start  int    // byte offset where the current token starts
	field  string // active field for wantOperator/wantValue
	quoted bool   // value prefix began with a quote
	list   bool   // the active operator is IN/NOT IN: values need parentheses
}

// connectives and directions are JQL's structural keywords, offered after a
// complete clause / ORDER BY field. Multi-word operators (e.g. "not in") are
// handled by matching the longest operator first.
var (
	jqlConnectives = []string{"AND ", "OR ", "ORDER BY "}
	jqlDirections  = []string{"ASC", "DESC"}
)

// jqlCommonFields and jqlCommonFunctions rank the everyday names ahead of the
// instance's long tail — plugin custom fields ("a4j-incident-…") and JSM SLA
// functions ("breached()") would otherwise win alphabetically and ghost as
// the first suggestion. Unlisted candidates follow, alphabetical.
var (
	jqlCommonFields = []string{
		"project", "assignee", "status", "reporter", "summary", "text",
		"sprint", "labels", "priority", "type", "issuetype", "component",
		"fixVersion", "parent", "key", "statusCategory", "resolution",
		"created", "updated", "due",
	}
	jqlCommonFunctions = []string{
		"currentUser()", "openSprints()", "now()", "startOfDay()",
		"endOfDay()", "startOfWeek()", "endOfWeek()", "membersOf()",
	}
)

// rankCandidates orders cands by their position in priority (trailing spaces
// and case ignored), then naturally for the unlisted rest.
func rankCandidates(cands, priority []string) []string {
	rank := make(map[string]int, len(priority))
	for i, p := range priority {
		rank[strings.ToLower(p)] = i + 1
	}
	at := func(c string) int {
		if r, ok := rank[strings.ToLower(strings.TrimRight(c, " "))]; ok {
			return r
		}
		return len(priority) + 1
	}
	sort.SliceStable(cands, func(i, j int) bool {
		ri, rj := at(cands[i]), at(cands[j])
		if ri != rj {
			return ri < rj
		}
		return xstrings.LessNatural(cands[i], cands[j])
	})
	return cands
}

// jqlComplete classifies the end of the input. It walks tokens left to right
// through a tiny clause state machine: field → operator → value → connective,
// with ORDER BY switching to field → direction. It is deliberately forgiving —
// unknown tokens advance the state rather than failing, so a query the parser
// would reject still completes sensibly while being typed. Parenthesised IN
// lists are not modeled: suggestions go quiet inside them and re-sync at the
// next AND/OR.
func jqlComplete(input string) jqlContext {
	toks := tokenizeJQL(input)
	state := wantField
	field := ""
	ordering := false
	list := false
	for i, t := range toks {
		last := i == len(toks)-1
		if last && !t.complete {
			// The token still being typed is the completion target.
			return jqlContext{kind: state, prefix: t.text, start: t.start, field: field, quoted: t.quoted, list: list}
		}
		switch state {
		case wantField:
			field = t.text
			list = false
			if ordering {
				state = wantDirection
			} else {
				state = wantOperator
			}
		case wantOperator:
			switch {
			case isOperatorWord(t.text):
				// "not", "is", "was", "changed" can extend into a multi-word
				// operator ("not in", "is not") — keep expecting operator.
			case strings.EqualFold(t.text, "EMPTY") || strings.EqualFold(t.text, "NULL"):
				// "is EMPTY": the keyword doubles as the value.
				state = wantConnective
			default:
				list = strings.EqualFold(t.text, "in")
				state = wantValue
			}
		case wantValue:
			state = wantConnective
		case wantConnective:
			switch strings.ToUpper(t.text) {
			case "AND", "OR":
				state = wantField
				ordering = false
			case "ORDER":
				// expect BY next; treat the pair as one connective
			case "BY":
				state = wantField
				ordering = true
			default:
				state = wantField
			}
		case wantDirection:
			// ORDER BY ends the filter clause: after a direction only another
			// sort field (via comma) may follow, never AND/OR.
			state = wantSortMore
		case wantSortMore:
			// stay: we don't model the comma-separated sort tail.
		}
	}
	// Input ends on whitespace (or is empty): the next token starts fresh.
	return jqlContext{kind: state, prefix: "", start: len(input), field: field, list: list}
}

type jqlToken struct {
	text     string
	start    int
	complete bool // followed by a space (or closed quote) — not being typed
	quoted   bool
}

// tokenizeJQL splits on spaces, keeping quoted runs ('…' or "…") as one
// token. An unterminated quote yields an incomplete token whose text is the
// content after the quote — the value prefix the user is mid-typing.
func tokenizeJQL(input string) []jqlToken {
	var toks []jqlToken
	i := 0
	for i < len(input) {
		if input[i] == ' ' {
			i++
			continue
		}
		start := i
		if q := input[i]; q == '\'' || q == '"' {
			j := strings.IndexByte(input[i+1:], q)
			if j < 0 { // unterminated: mid-typing a quoted value
				toks = append(toks, jqlToken{text: input[i+1:], start: start, quoted: true})
				return toks
			}
			toks = append(toks, jqlToken{text: input[i+1 : i+1+j], start: start, complete: true, quoted: true})
			i += j + 2
			continue
		}
		j := strings.IndexByte(input[i:], ' ')
		if j < 0 {
			toks = append(toks, jqlToken{text: input[i:], start: start})
			return toks
		}
		toks = append(toks, jqlToken{text: input[i : i+j], start: start, complete: true})
		i += j
	}
	return toks
}

// isOperatorWord reports a word that can extend into a multi-word operator
// ("not in", "is not", "was in", "changed after"). "in" is deliberately not
// here: it always ends the operator.
func isOperatorWord(t string) bool {
	switch strings.ToLower(t) {
	case "not", "is", "was", "changed":
		return true
	}
	return false
}

// candidatesFor builds the token candidates for a completion point from the
// reference data: field names, the active field's operators, structural
// keywords, or — for values — the JQL functions (live field values arrive
// asynchronously and are merged by the caller via valueCandidates).
func candidatesFor(ref jira.JQLReference, c jqlContext) []string {
	var out []string
	switch c.kind {
	case wantField:
		for _, f := range ref.Fields {
			out = append(out, f.Value+" ")
		}
		return rankCandidates(out, jqlCommonFields)
	case wantOperator:
		for _, f := range ref.Fields {
			if strings.EqualFold(f.Value, c.field) {
				for _, op := range f.Operators {
					out = append(out, op+" ")
				}
			}
		}
	case wantValue:
		for _, fn := range ref.Functions {
			out = append(out, fn.Value)
		}
		return rankCandidates(out, jqlCommonFunctions)
	case wantConnective:
		out = append(out, jqlConnectives...)
	case wantDirection:
		out = append(out, jqlDirections...)
	case wantSortMore:
		// ORDER BY ends the query: no AND/OR after a sort direction, and the
		// comma-separated sort tail isn't modeled — offer nothing.
	}
	xslices.SortNatural(out)
	return out
}

// completionLines turns token candidates into full-line suggestions for the
// textinput's whole-line prefix matcher: everything before the token plus the
// candidate. A suggestion must extend the exact text already typed, so
// quoting follows the user: with an open quote (or nothing typed yet)
// multi-word values complete fully quoted; after a bare prefix only
// space-free values are offered — a quote injected mid-token could never
// prefix-match and would suggest a line the user didn't type.
func completionLines(input string, c jqlContext, candidates []string) []string {
	base := input[:c.start]
	if base != "" && !strings.HasSuffix(base, " ") {
		// A closed quote ends a token without a trailing space ("…"AND would
		// glue the keyword to the quote) — rejoin with one.
		base += " "
	}
	var out []string
	for _, cand := range candidates {
		if len(cand) <= len(c.prefix) || !strings.HasPrefix(strings.ToLower(cand), strings.ToLower(c.prefix)) {
			continue
		}
		if c.kind == wantValue {
			multiword := strings.Contains(strings.TrimRight(cand, " "), " ")
			switch {
			case c.list:
				// IN/NOT IN takes a parenthesised list; only completable from
				// an empty prefix — a typed prefix can't grow an opening paren.
				if c.prefix != "" {
					continue
				}
				if multiword {
					cand = `"` + cand + `"`
				}
				cand = "(" + cand + ")"
			case c.quoted && strings.HasSuffix(cand, ")"):
				continue // a function call quoted is a literal string, not a call
			case c.quoted || (c.prefix == "" && multiword):
				cand = `"` + strings.TrimRight(cand, " ") + `"`
			case multiword:
				continue // bare prefix can't grow into a quoted value
			}
		}
		out = append(out, base+cand)
	}
	return out
}

// valueField reports the field to fetch live value suggestions for: only at
// a value position, and only when the reference marks the field as
// suggestable. The empty string means "don't fetch".
func valueField(ref jira.JQLReference, c jqlContext) string {
	if c.kind != wantValue {
		return ""
	}
	for _, f := range ref.Fields {
		if strings.EqualFold(f.Value, c.field) && f.Auto {
			return f.Value
		}
	}
	return ""
}
