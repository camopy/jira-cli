package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	clib "github.com/gechr/clib/cli/cobra"
	"github.com/matcra587/jira-cli/internal/cache"
	"github.com/matcra587/jira-cli/pkg/jira"
	"github.com/spf13/cobra"
)

type jqlBuildOptions struct {
	Projects   []string
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

func jqlCommand() *cobra.Command {
	var builder jqlBuildOptions
	cmd := groupCommand("jql", "Build JQL queries", "resources")
	build := &cobra.Command{
		Use:   "build",
		Short: "Build a JQL query from flags",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			scope, precedence, err := boardScopeFromFlags(cmd)
			if err != nil {
				return err
			}
			query, err := builder.Build()
			if err != nil {
				return err
			}
			query = applyBoardClauseToJQL(query, scope)
			data := map[string]any{
				"jql":         query,
				"precedence":  precedence,
				"board_scope": boardScopeEnvelopeData(scope),
			}
			return writeEnvelope(cmd, "jql.build", data)
		},
	}
	addJQLBuilderFlags(build, &builder)
	addBoardScopeFlags(build)
	cmd.AddCommand(build)
	return cmd
}

func addJQLBuilderFlags(cmd *cobra.Command, builder *jqlBuildOptions) {
	cmd.Flags().StringSliceVar(&builder.Projects, "project", nil, "Restrict to Jira project key")
	cmd.Flags().StringSliceVar(&builder.Epics, "epic", nil, "Restrict to issues in epic keys")
	cmd.Flags().StringVar(&builder.Assignee, "assignee", "", `Restrict by assignee; use "me" for currentUser()`)
	cmd.Flags().StringVar(&builder.Reporter, "reporter", "", `Restrict by reporter; use "me" for currentUser()`)
	cmd.Flags().StringSliceVar(&builder.Statuses, "status", nil, "Restrict by status name")
	cmd.Flags().StringSliceVar(&builder.Priorities, "priority", nil, "Restrict by priority")
	cmd.Flags().StringSliceVar(&builder.Labels, "label", nil, "Restrict by label")
	cmd.Flags().StringSliceVar(&builder.IssueTypes, "type", nil, "Restrict by issue type")
	cmd.Flags().StringVar(&builder.OrderBy, "order-by", "updated", "Sort field")
	cmd.Flags().BoolVar(&builder.Descending, "desc", true, "Sort descending")
	clib.Extend(cmd.Flags().Lookup("project"), clib.FlagExtra{Group: "Filters", Placeholder: "KEY", Complete: "predictor=cacheproject,comma"})
	clib.Extend(cmd.Flags().Lookup("epic"), clib.FlagExtra{Group: "Filters", Placeholder: "KEY", Complete: "predictor=cacheepic,comma"})
	clib.Extend(cmd.Flags().Lookup("label"), clib.FlagExtra{Group: "Filters", Placeholder: "NAME", Complete: "predictor=cachelabel,comma"})
	clib.Extend(cmd.Flags().Lookup("type"), clib.FlagExtra{Group: "Filters", Placeholder: "NAME", Complete: "predictor=cacheissuetype,comma"})
	clib.Extend(cmd.Flags().Lookup("assignee"), clib.FlagExtra{Group: "Filters", Placeholder: "USER", Enum: []string{"me", "none"}})
	clib.Extend(cmd.Flags().Lookup("reporter"), clib.FlagExtra{Group: "Filters", Placeholder: "USER", Enum: []string{"me"}})
	clib.Extend(cmd.Flags().Lookup("status"), clib.FlagExtra{Group: "Filters", Placeholder: "NAME"})
	clib.Extend(cmd.Flags().Lookup("priority"), clib.FlagExtra{Group: "Filters", Placeholder: "NAME"})
	clib.Extend(cmd.Flags().Lookup("order-by"), clib.FlagExtra{Group: "Sort", Placeholder: "FIELD", Enum: []string{"updated", "created", "priority", "status", "key", "summary"}, EnumDefault: "updated"})
	clib.Extend(cmd.Flags().Lookup("desc"), clib.FlagExtra{Group: "Sort"})
}

// readCacheJSON loads a cache resource into v for the given profile, ignoring
// TTL so completion stays fast even when the cache is stale. Returns false
// silently on any error so completion never blocks the shell.
func readCacheJSON(profile, resource string, v any) bool {
	entry, ok, _, err := cache.Read(profile, resource, 24*time.Hour*365)
	if err != nil || !ok {
		return false
	}
	return json.Unmarshal(entry.Data, v) == nil
}

func issueListJQL(raw string, builder jqlBuildOptions) (string, error) {
	if raw := strings.TrimSpace(raw); raw != "" {
		clauses := builder.filterClauses()
		if len(clauses) == 0 {
			return raw, nil
		}
		query, orderBy := splitTopLevelOrderBy(raw)
		if strings.TrimSpace(query) == "" {
			return strings.Join(clauses, " AND ") + orderBy, nil
		}
		clauses = append(clauses, parenthesizeJQL(query))
		return strings.Join(clauses, " AND ") + orderBy, nil
	}
	return builder.Build()
}

func (o jqlBuildOptions) filterClauses() []string {
	clauses := make([]string, 0, 8)
	appendInClause := func(field string, values []string) {
		values = compactStrings(values)
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
	if clause := userClause("assignee", o.Assignee); clause != "" {
		clauses = append(clauses, clause)
	}
	if clause := userClause("reporter", o.Reporter); clause != "" {
		clauses = append(clauses, clause)
	}
	appendInClause("parent", o.Epics)
	appendInClause("status", o.Statuses)
	appendInClause("priority", o.Priorities)
	appendInClause("labels", o.Labels)
	appendInClause("issuetype", o.IssueTypes)
	return clauses
}

func (o jqlBuildOptions) Build() (string, error) {
	clauses := o.filterClauses()

	if len(clauses) == 0 && strings.TrimSpace(o.OrderBy) == "" && !o.Descending {
		return jira.DefaultIssueListJQL, nil
	}
	if len(clauses) == 0 {
		clauses = append(clauses, "updated >= -365d")
	}

	query := strings.Join(clauses, " AND ")
	orderBy := strings.TrimSpace(o.OrderBy)
	if orderBy == "" {
		orderBy = "updated"
	}
	if orderBy != "none" {
		if !isSafeJQLIdentifier(orderBy) {
			return "", fmt.Errorf("invalid order-by field %q", orderBy)
		}
		direction := "ASC"
		if o.Descending {
			direction = "DESC"
		}
		query += " ORDER BY " + orderBy + " " + direction
	}
	return query, nil
}

func combineJQLClauses(lhs, rhs string) string {
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

func parenthesizeJQLIfTopLevelOR(query string) string {
	if hasTopLevelWord(query, "OR") {
		return parenthesizeJQL(query)
	}
	return strings.TrimSpace(query)
}

func splitTopLevelOrderBy(query string) (string, string) {
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

func compactStrings(values []string) []string {
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
