// Package jql builds and composes Jira Query Language strings from
// structured inputs. It is pure string logic — no cobra, config, or I/O —
// so the jql/search/issue command layers and shell completion can all share
// one query builder without import cycles.
package jql

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/matcra587/jira-cli/internal/issuekey"
)

// DefaultIssueListJQL is the bounded default query used when an issue list is
// requested with no filters and no explicit sort. It is owned here, in the
// query domain, and consumed by the Jira issue service as its List default.
const DefaultIssueListJQL = "updated >= -365d ORDER BY updated DESC"

// BuildOptions captures the structured filters that compose into a JQL query.
type BuildOptions struct {
	Projects   []string
	Keys       []string
	Epics      []string
	Assignee   string
	Reporter   string
	Statuses   []string
	Priorities []string
	Labels     []string
	IssueTypes []string
	OrderBy    string
	Descending bool
}

// Build renders the options into a complete JQL query (with ORDER BY). With
// no filters and no explicit sort it returns the default bounded issue list.
func (o BuildOptions) Build() (string, error) {
	clauses, err := o.filterClauses()
	if err != nil {
		return "", err
	}

	if len(clauses) == 0 && strings.TrimSpace(o.OrderBy) == "" && !o.Descending {
		return DefaultIssueListJQL, nil
	}
	if len(clauses) == 0 {
		clauses = append(clauses, "updated >= -365d")
	}

	query := strings.Join(clauses, " AND ")
	orderBy := strings.TrimSpace(o.OrderBy)
	if orderBy == "" {
		orderBy = "updated"
	}
	clause, err := orderByClause(orderBy, o.Descending)
	if err != nil {
		return "", err
	}
	return query + clause, nil
}

// orderByClause renders " ORDER BY <field> <DIR>" for a sort field, or "" when
// the field is blank or "none" (sort disabled). The field is validated as a
// safe JQL identifier.
func orderByClause(orderBy string, descending bool) (string, error) {
	orderBy = strings.TrimSpace(orderBy)
	if orderBy == "" || orderBy == "none" {
		return "", nil
	}
	if !isSafeJQLIdentifier(orderBy) {
		return "", fmt.Errorf("invalid order-by field %q", orderBy)
	}
	direction := "ASC"
	if descending {
		direction = "DESC"
	}
	return " ORDER BY " + orderBy + " " + direction, nil
}

func (o BuildOptions) filterClauses() ([]string, error) {
	clauses := make([]string, 0, 8)
	appendInClause := func(field string, values []string) {
		values = CompactStrings(values)
		if len(values) == 0 {
			return
		}
		if len(values) == 1 {
			clauses = append(clauses, field+" = "+jqlValue(values[0]))
			return
		}
		clauses = append(clauses, field+" in ("+joinJQLValues(values)+")")
	}

	appendInClause("project", o.Projects)
	keys, err := issuekey.ParseExpressions(o.Keys, issuekey.Options{MaxExpansion: issuekey.DefaultMaxExpansion})
	if err != nil {
		return nil, err
	}
	appendInClause("key", keys)
	if clause := userClause("assignee", o.Assignee); clause != "" {
		clauses = append(clauses, clause)
	}
	if clause := userClause("reporter", o.Reporter); clause != "" {
		clauses = append(clauses, clause)
	}
	appendInClause("parent", o.Epics)
	statusClauses, err := statusFilterClauses(o.Statuses)
	if err != nil {
		return nil, err
	}
	clauses = append(clauses, statusClauses...)
	appendInClause("priority", o.Priorities)
	appendInClause("labels", o.Labels)
	appendInClause("issuetype", o.IssueTypes)
	return clauses, nil
}

// statusCategoryOrder is Jira's universal three-bucket workflow ordering. It
// is the basis for the comparator status filters; specific workflow statuses
// map onto one of these buckets, so the comparators need no live workflow
// fetch or per-project configuration.
var statusCategoryOrder = []string{"To Do", "In Progress", "Done"}

// statusFilterClauses turns --status values into JQL. The values fall into two
// kinds: positive predicates (a plain status name, or a category comparator)
// are alternatives, OR-ed together; negations are constraints, AND-ed on.
//
//	plain name           one `status = N` / `status in (N, ...)` term
//	<C / <=C / >C / >=C   category comparator over statusCategoryOrder (C must
//	                      be a category: To Do, In Progress, Done)
//	!S                    exclude a specific status (status != S)
//
// So `--status Open,'>=In Progress'` reads as "Open OR in-progress-or-beyond",
// not their (empty) intersection, while `--status '>=In Progress','!Abandoned'`
// keeps the negation as an AND-ed constraint.
func statusFilterClauses(values []string) ([]string, error) {
	values = CompactStrings(values)
	if len(values) == 0 {
		return nil, nil
	}
	var plain, positives, negatives []string
	for _, v := range values {
		switch {
		case strings.HasPrefix(v, "!"):
			name := strings.TrimSpace(v[1:])
			if name == "" {
				return nil, fmt.Errorf("status filter %q: missing status name after %q", v, "!")
			}
			negatives = append(negatives, "status != "+jqlValue(name))
		case statusComparatorPrefix(v) != "":
			clause, err := statusCategoryComparator(v)
			if err != nil {
				return nil, err
			}
			positives = append(positives, clause)
		default:
			plain = append(plain, v)
		}
	}
	switch len(plain) {
	case 0:
	case 1:
		positives = append([]string{"status = " + jqlValue(plain[0])}, positives...)
	default:
		positives = append([]string{"status in (" + joinJQLValues(plain) + ")"}, positives...)
	}

	var clauses []string
	switch len(positives) {
	case 0:
	case 1:
		clauses = append(clauses, positives[0])
	default:
		clauses = append(clauses, "("+strings.Join(positives, " OR ")+")")
	}
	return append(clauses, negatives...), nil
}

// statusComparatorPrefix returns the leading comparator operator in v, or "".
// Two-character operators are tested first so "<=" is not read as "<".
func statusComparatorPrefix(v string) string {
	for _, op := range []string{"<=", ">=", "<", ">"} {
		if strings.HasPrefix(v, op) {
			return op
		}
	}
	return ""
}

// statusCategoryComparator compiles a comparator expression (e.g. ">=In
// Progress") into a statusCategory clause over statusCategoryOrder.
func statusCategoryComparator(v string) (string, error) {
	op := statusComparatorPrefix(v)
	target, ok := statusCategoryIndex(strings.TrimSpace(v[len(op):]))
	if !ok {
		return "", fmt.Errorf("status filter %q: %q comparators take a status category (To Do, In Progress, Done)", v, op)
	}
	var cats []string
	for i, name := range statusCategoryOrder {
		if comparatorMatches(op, i, target) {
			cats = append(cats, name)
		}
	}
	if len(cats) == 0 {
		return "", fmt.Errorf("status filter %q matches no status category", v)
	}
	if len(cats) == 1 {
		return "statusCategory = " + jqlValue(cats[0]), nil
	}
	return "statusCategory in (" + joinJQLValues(cats) + ")", nil
}

// statusCategoryIndex maps a category name (case-insensitive, with or without
// the space in "To Do") to its position in statusCategoryOrder.
func statusCategoryIndex(name string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "to do", "todo":
		return 0, true
	case "in progress", "inprogress":
		return 1, true
	case "done":
		return 2, true
	}
	return 0, false
}

func comparatorMatches(op string, i, target int) bool {
	switch op {
	case "<":
		return i < target
	case "<=":
		return i <= target
	case ">":
		return i > target
	case ">=":
		return i >= target
	}
	return false
}

// IssueList combines a raw JQL string with the builder's structured filters.
// A non-empty raw query is parenthesized and AND-ed beneath the filters. The
// raw query's own top-level ORDER BY always wins; when it has none, the
// builder's --order-by is applied (an empty or "none" order-by adds nothing).
// An empty raw query falls back to Build.
func IssueList(raw string, builder BuildOptions) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return builder.Build()
	}

	clauses, err := builder.filterClauses()
	if err != nil {
		return "", err
	}

	query, orderBy := SplitTopLevelOrderBy(raw)
	if orderBy == "" {
		if orderBy, err = orderByClause(builder.OrderBy, builder.Descending); err != nil {
			return "", err
		}
	}

	if len(clauses) == 0 {
		return query + orderBy, nil
	}
	if strings.TrimSpace(query) == "" {
		return strings.Join(clauses, " AND ") + orderBy, nil
	}
	clauses = append(clauses, parenthesizeJQL(query))
	return strings.Join(clauses, " AND ") + orderBy, nil
}

// CombineClauses AND-joins two JQL fragments, dropping either side when blank.
func CombineClauses(lhs, rhs string) string {
	lhs = strings.TrimSpace(lhs)
	rhs = strings.TrimSpace(rhs)
	if lhs == "" {
		return rhs
	}
	if rhs == "" {
		return lhs
	}
	return lhs + " AND " + rhs
}

func parenthesizeJQL(query string) string {
	query = strings.TrimSpace(query)
	if query == "" || isWrappedJQL(query) {
		return query
	}
	return "(" + query + ")"
}

// ParenthesizeIfTopLevelOR wraps query in parentheses only when it contains a
// top-level OR, so AND-composition with another clause keeps the original
// precedence.
func ParenthesizeIfTopLevelOR(query string) string {
	if hasTopLevelWord(query, "OR") {
		return parenthesizeJQL(query)
	}
	return strings.TrimSpace(query)
}

// SplitTopLevelOrderBy splits query into the filter portion and a leading-space
// " ORDER BY ..." suffix (empty when there is no top-level ORDER BY).
func SplitTopLevelOrderBy(query string) (string, string) {
	idx := findTopLevelOrderBy(query)
	if idx == -1 {
		return strings.TrimSpace(query), ""
	}
	return strings.TrimSpace(query[:idx]), " " + strings.TrimSpace(query[idx:])
}

func findTopLevelOrderBy(query string) int {
	tokens := topLevelWordTokens(query)
	for i := 0; i+1 < len(tokens); i++ {
		if strings.EqualFold(tokens[i].word, "ORDER") && strings.EqualFold(tokens[i+1].word, "BY") {
			return tokens[i].start
		}
	}
	return -1
}

func hasTopLevelWord(query, want string) bool {
	for _, token := range topLevelWordTokens(query) {
		if strings.EqualFold(token.word, want) {
			return true
		}
	}
	return false
}

type jqlWordToken struct {
	word  string
	start int
}

func topLevelWordTokens(query string) []jqlWordToken {
	var (
		tokens []jqlWordToken
		depth  int
		quote  rune
		start  = -1
	)
	flush := func(end int) {
		if start == -1 {
			return
		}
		tokens = append(tokens, jqlWordToken{word: query[start:end], start: start})
		start = -1
	}
	for i, r := range query {
		if quote != 0 {
			if r == quote && (i == 0 || query[i-1] != '\\') {
				quote = 0
			}
			continue
		}
		switch {
		case r == '"' || r == '\'':
			flush(i)
			quote = r
		case r == '(':
			flush(i)
			depth++
		case r == ')':
			flush(i)
			if depth > 0 {
				depth--
			}
		case depth == 0 && (unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'):
			if start == -1 {
				start = i
			}
		default:
			flush(i)
		}
	}
	flush(len(query))
	return tokens
}

func isWrappedJQL(query string) bool {
	query = strings.TrimSpace(query)
	if len(query) < 2 || query[0] != '(' || query[len(query)-1] != ')' {
		return false
	}
	depth := 0
	quote := byte(0)
	for i := 0; i < len(query); i++ {
		ch := query[i]
		if quote != 0 {
			if ch == quote && (i == 0 || query[i-1] != '\\') {
				quote = 0
			}
			continue
		}
		switch ch {
		case '"', '\'':
			quote = ch
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 && i != len(query)-1 {
				return false
			}
		}
	}
	return depth == 0
}

func userClause(field, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	switch strings.ToLower(value) {
	case "me", "currentuser()":
		return field + " = currentUser()"
	case "none", "empty", "unassigned":
		return field + " is EMPTY"
	default:
		return field + " = " + jqlValue(value)
	}
}

// CompactStrings returns values with empty/whitespace-only entries dropped and
// the rest trimmed.
func CompactStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func joinJQLValues(values []string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, jqlValue(value))
	}
	return strings.Join(parts, ", ")
}

func jqlValue(value string) string {
	value = strings.TrimSpace(value)
	if isSafeJQLIdentifier(value) {
		return value
	}
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}

func isSafeJQLIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.' {
			continue
		}
		return false
	}
	return true
}
