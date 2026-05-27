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
	logger := clog.New(clog.NewOutput(w, clog.ColorAuto))

	m, ok := data.(map[string]any)
	if !ok {
		return writeGenericPlain(logger, messageForCommand(command), data)
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
