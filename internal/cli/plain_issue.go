package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/gechr/clog"
	"github.com/matcra587/jira-cli/internal/adf"
)

func WriteIssueViewPlain(w io.Writer, command string, data any, opts ...PlainOption) error {
	cfg := defaultPlainConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	logger := newPlainLogger(w)

	m := mapFromAny(data)
	if len(m) == 0 {
		return writeGenericPlain(logger, messageForCommand(command), data)
	}
	if rawResults, ok := m["results"]; ok {
		results := normalizeMapList(rawResults)
		if results == nil {
			return writeGenericPlain(logger, messageForCommand(command), data)
		}
		return writeIssueViewManyPlain(logger, results, cfg)
	}
	issue := mapFromAny(m["issue"])
	if len(issue) == 0 {
		return writeGenericPlain(logger, messageForCommand(command), data)
	}
	fields := mapFromAny(issue["fields"])
	style := authPlainStyle{tty: cfg.tty, theme: cfg.theme}

	key := stringFromMap(issue, "key")
	summary := stringFromMap(fields, "summary")
	header := strings.TrimSpace(strings.Join([]string{key, summary}, "  "))
	if header == "" {
		header = "Issue"
	}
	logger.Info().Parts(clog.PartMessage).Msg(style.bold(header))

	status := nestedString(fields, "status", "name")
	priority := nestedString(fields, "priority", "name")
	assignee := nestedString(fields, "assignee", "displayName")
	if assignee == "" {
		assignee = nestedString(fields, "assignee", "accountId")
	}
	if assignee == "" {
		assignee = "unassigned"
	}
	logger.Info().Parts(clog.PartMessage).Msg(fmt.Sprintf("  status: %s  priority: %s  assignee: %s",
		firstNonEmpty(status, "unknown"),
		firstNonEmpty(priority, "none"),
		assignee,
	))

	if description := issueDescriptionPlain(fields); description != "" {
		logger.Info().Parts(clog.PartMessage).Msg("  " + description)
	}
	return nil
}

func writeIssueViewManyPlain(logger *clog.Logger, results []map[string]any, cfg plainConfig) error {
	successes := make([]map[string]any, 0, len(results))
	failureCount := 0
	for _, result := range results {
		key := stringFromMap(result, "key")
		if ok, _ := result["ok"].(bool); !ok {
			failureCount++
			continue
		}
		issue := mapFromAny(result["issue"])
		row := issueViewSummaryRow(issue)
		if row["key"] == "" {
			row["key"] = key
		}
		successes = append(successes, row)
	}

	event := logger.Info().
		Int("total", len(results)).
		Int("succeeded", len(successes)).
		Int("failed", failureCount)
	if cfg.threads > 0 {
		event = event.Int("threads", cfg.threads)
	}
	event.Msg("viewed issues")
	for _, row := range issueRows(successes, cfg) {
		if row != "" {
			logger.Info().Parts(clog.PartMessage).Msg(row)
		}
	}
	return nil
}

const issueViewFailedKeysPlainLimit = 5

// WriteIssueViewFailureDiagnostics mirrors multi-key issue-view failures to
// stderr for human mode. stdout stays reserved for successful rows.
func WriteIssueViewFailureDiagnostics(w io.Writer, data any, errorsOut []Error) error {
	failures := issueViewFailureKeys(data)
	if len(failures) == 0 || w == nil {
		return nil
	}
	logger := clog.New(clog.NewOutput(w, clog.ColorAuto))
	logger.SetLevel(clog.LevelError)
	shown, omitted := issueViewShownFailureKeys(failures)
	total, succeeded, failed := issueViewFailureCounts(data, len(failures))
	event := logger.Error().
		Int("total", total).
		Int("succeeded", succeeded).
		Int("failed", failed).
		Str("reason", issueViewPlainFailureReason(errorsOut)).
		Str("keys", strings.Join(shown, ", ")).
		Int("shown", len(shown))
	if omitted > 0 {
		event = event.Int("omitted", omitted)
	}
	event = event.Str("hint", "use --output=json for full per-key errors")
	event.Msg("failed keys")
	return nil
}

func issueViewFailureCounts(data any, failureKeys int) (int, int, int) {
	m := mapFromAny(data)
	results := normalizeMapList(m["results"])
	total := len(results)
	succeeded := intFromMap(m, "succeeded")
	failed := intFromMap(m, "failed")
	if failed == 0 {
		failed = failureKeys
	}
	if total == 0 {
		total = succeeded + failed
	}
	if succeeded == 0 && total > failed {
		succeeded = total - failed
	}
	return total, succeeded, failed
}

func issueViewPlainFailureReason(errorsOut []Error) string {
	if len(errorsOut) == 0 {
		return "error"
	}
	top := errorsOut[0]
	if top.Code != "" {
		return strings.ReplaceAll(top.Code, "_", " ")
	}
	if top.Type != "" {
		return top.Type
	}
	return "error"
}

func issueViewFailureKeys(data any) []string {
	m := mapFromAny(data)
	results := normalizeMapList(m["results"])
	if len(results) == 0 {
		return nil
	}
	failures := make([]string, 0)
	for _, result := range results {
		if ok, _ := result["ok"].(bool); ok {
			continue
		}
		if key := stringFromMap(result, "key"); key != "" {
			failures = append(failures, key)
		}
	}
	return failures
}

func issueViewShownFailureKeys(keys []string) ([]string, int) {
	limit := issueViewFailedKeysPlainLimit
	if len(keys) < limit {
		limit = len(keys)
	}
	return keys[:limit], len(keys) - limit
}

func issueViewSummaryRow(issue map[string]any) map[string]any {
	fields := mapFromAny(issue["fields"])
	return map[string]any{
		"key":      stringFromMap(issue, "key"),
		"summary":  stringFromMap(fields, "summary"),
		"status":   nestedString(fields, "status", "name"),
		"assignee": mapFromAny(fields["assignee"]),
		"priority": nestedString(fields, "priority", "name"),
	}
}

func WriteIssueTransitionsPlain(w io.Writer, command string, data any, opts ...PlainOption) error {
	cfg := defaultPlainConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	logger := clog.New(clog.NewOutput(w, clog.ColorAuto))

	m, ok := data.(map[string]any)
	if !ok {
		return writeGenericPlain(logger, messageForCommand(command), data)
	}
	transitions := normalizeMapList(m["transitions"])
	style := authPlainStyle{tty: cfg.tty, theme: cfg.theme}

	issue, _ := m["issue"].(string)
	header := "Transitions"
	if issue != "" {
		header += " on " + issue
	}
	logger.Info().Parts(clog.PartMessage).Msg(style.bold(header))

	if len(transitions) == 0 {
		logger.Info().Parts(clog.PartMessage).Msg(style.dim("  (no transitions available)"))
		return nil
	}
	for _, transition := range transitions {
		id := stringFromMap(transition, "id")
		name := stringFromMap(transition, "name")
		logger.Info().Parts(clog.PartMessage).Msg("  " + padRight(id, 4) + "  " + name)
	}
	return nil
}

func mapFromAny(value any) map[string]any {
	if value == nil {
		return nil
	}
	if m, ok := value.(map[string]any); ok {
		return m
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func stringFromMap(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	switch value := m[key].(type) {
	case string:
		return value
	case fmt.Stringer:
		return value.String()
	default:
		return ""
	}
}

func intFromMap(m map[string]any, key string) int {
	if m == nil {
		return 0
	}
	switch value := m[key].(type) {
	case int:
		return value
	case float64:
		return int(value)
	default:
		return 0
	}
}

func nestedString(m map[string]any, key, child string) string {
	return stringFromMap(mapFromAny(m[key]), child)
}

func issueDescriptionPlain(fields map[string]any) string {
	docMap := mapFromAny(fields["description"])
	doc, ok := adfDocumentFromMap(docMap)
	if !ok {
		return ""
	}
	return normalizePlain(adf.ToPlain(doc))
}
