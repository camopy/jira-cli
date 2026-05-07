package cli

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"image/color"
	"io"
	"net/url"
	"reflect"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	clibtheme "github.com/gechr/clib/theme"
	"github.com/gechr/clog"
	"github.com/gechr/primer/table"
	termansi "github.com/gechr/x/ansi"
	"github.com/matcra587/jira-cli/pkg/adf"
	"github.com/matcra587/jira-cli/pkg/jira"
)

type PlainOption func(*plainConfig)

type plainConfig struct {
	baseURL   string
	termWidth int
	tty       bool
	theme     *clibtheme.Theme
}

func WithPlainBaseURL(baseURL string) PlainOption {
	return func(cfg *plainConfig) {
		cfg.baseURL = strings.TrimSpace(baseURL)
	}
}

func WithPlainTermWidth(width int) PlainOption {
	return func(cfg *plainConfig) {
		cfg.termWidth = width
	}
}

func WithPlainTTY(tty bool) PlainOption {
	return func(cfg *plainConfig) {
		cfg.tty = tty
	}
}

func WithPlainTheme(theme *clibtheme.Theme) PlainOption {
	return func(cfg *plainConfig) {
		cfg.theme = theme
	}
}

func WritePlain(w io.Writer, data any) error {
	logger := clog.New(clog.NewOutput(w, clog.ColorAuto))
	return writeGenericPlain(logger, "result", data)
}

func WriteCommandPlain(w io.Writer, command string, data any, opts ...PlainOption) error {
	cfg := defaultPlainConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	logger := clog.New(clog.NewOutput(w, clog.ColorAuto))
	switch command {
	case "issue.list":
		return writeIssueListPlain(logger, data, cfg)
	case "auth.status":
		return writeAuthStatusPlain(logger, data, cfg)
	case "issue.attachment.list":
		return WriteAttachmentListPlain(w, command, data, opts...)
	case "issue.comment.list":
		return WriteCommentListPlain(w, command, data, opts...)
	case "issue.watchers.list":
		return WriteWatcherListPlain(w, command, data, opts...)
	case "issue.link.list":
		return WriteLinkListPlain(w, command, data, opts...)
	case "issue.link.types":
		return WriteLinkTypesPlain(w, command, data, opts...)
	case "boards.list":
		return WriteBoardListPlain(w, command, data, opts...)
	default:
		return writeGenericPlain(logger, messageForCommand(command), data)
	}
}

func defaultPlainConfig() plainConfig {
	return plainConfig{
		termWidth: 100,
		theme:     clibtheme.Default(),
	}
}

func writeGenericPlain(logger *clog.Logger, message string, data any) error {
	fields := plainFields(data)
	if len(fields) == 0 {
		return nil
	}
	event := logger.Info()
	for _, field := range fields {
		event = event.Any(field.key, field.value)
	}
	event.Msg(message)
	return nil
}

func messageForCommand(command string) string {
	switch command {
	case "issue.list":
		return "listed issues"
	case "issue.view":
		return "viewed issue"
	case "issue.create":
		return "created issue"
	case "issue.edit":
		return "edited issue"
	case "issue.clone":
		return "cloned issue"
	case "issue.move":
		return "moved issue"
	case "issue.delete":
		return "deleted issue"
	case "issue.transition":
		return "transitioned issue"
	case "issue.comment":
		return "commented on issue"
	case "epic.list":
		return "listed epics"
	case "epic.board":
		return "rendered epic board"
	case "search.jql", "search.saved":
		return "searched issues"
	case "worklog.add":
		return "added worklog"
	case "worklog.list":
		return "listed worklogs"
	case "auth.login":
		return "logged in"
	case "auth.status":
		return "checked auth"
	case "auth.logout":
		return "logged out"
	case "auth.switch":
		return "switched profile"
	case "auth.token":
		return "inspected token"
	case "schema":
		return "rendered schema"
	default:
		return strings.ReplaceAll(command, ".", " ")
	}
}

type plainField struct {
	key   string
	value any
}

func plainFields(data any) []plainField {
	fields := []plainField{}
	switch v := data.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			writePlainField(&fields, key, v[key])
		}
	default:
		writePlainField(&fields, "value", v)
	}
	return fields
}

func writePlainField(fields *[]plainField, key string, value any) {
	writePlainValue(fields, key, value)
}

func writePlainValue(fields *[]plainField, key string, value any) {
	switch v := value.(type) {
	case adf.Document:
		writePlainLine(fields, key, normalizePlain(adf.ToPlain(v)))
	case *adf.Document:
		if v == nil {
			writePlainLine(fields, key, "")
			return
		}
		writePlainLine(fields, key, normalizePlain(adf.ToPlain(*v)))
	case []any:
		writePlainLine(fields, key, v)
	case map[string]any:
		if doc, ok := adfDocumentFromMap(v); ok {
			writePlainLine(fields, key, normalizePlain(adf.ToPlain(doc)))
			return
		}
		keys := make([]string, 0, len(v))
		for childKey := range v {
			keys = append(keys, childKey)
		}
		sort.Strings(keys)
		for _, childKey := range keys {
			writePlainValue(fields, key+"."+childKey, v[childKey])
		}
	default:
		writePlainLine(fields, key, value)
	}
}

func writePlainLine(fields *[]plainField, key string, value any) {
	*fields = append(*fields, plainField{key: key, value: plainFieldValue(value)})
}

func writeIssueListPlain(logger *clog.Logger, data any, cfg plainConfig) error {
	m, ok := data.(map[string]any)
	if !ok {
		return writeGenericPlain(logger, "listed issues", data)
	}
	rawIssues, detail, jql, ok := issueListPlainData(m)
	if !ok {
		return writeGenericPlain(logger, "listed issues", data)
	}
	issues, ok := normalizeIssueRows(rawIssues)
	if !ok {
		return writeGenericPlain(logger, "listed issues", data)
	}
	event := logger.Info().
		Int("count", len(issues)).
		Bool("detail", detail)
	if jql != "" {
		event = event.Str("jql", jql)
	}
	event.Msg("listed issues")
	if len(issues) == 0 {
		return nil
	}
	for _, row := range issueRows(issues, cfg) {
		if row != "" {
			logger.Info().Parts(clog.PartMessage).Msg(row)
		}
	}
	return nil
}

func issueListPlainData(data map[string]any) (rawIssues any, detail bool, jql string, ok bool) {
	rawIssues, ok = data["issues"]
	if !ok {
		return nil, false, "", false
	}
	if v, ok := data["detail"].(bool); ok {
		detail = v
	}
	if v, ok := data["jql"].(string); ok {
		jql = v
	}
	return rawIssues, detail, jql, true
}

type issueTableRow struct {
	Key      string
	Summary  string
	Status   string
	Assignee string
	Priority string
}

func issueRows(issues []map[string]any, cfg plainConfig) []string {
	rows := make([]issueTableRow, 0, len(issues))
	for _, issue := range issues {
		rows = append(rows, issueTableRow{
			Key:      formatHumanField(issue["key"]),
			Summary:  formatHumanField(issue["summary"]),
			Status:   formatHumanField(issue["status"]),
			Assignee: formatAssignee(issue["assignee"]),
			Priority: formatHumanField(issue["priority"]),
		})
	}
	rendered := issueTableRenderer(cfg).Render(rows)
	if rendered.String() == "" {
		return nil
	}
	return strings.Split(rendered.String(), "\n")
}

func issueTableRenderer(cfg plainConfig) *table.Renderer[issueTableRow] {
	th := primerTheme{theme: cfg.theme, styled: cfg.tty}
	a := termansi.New(
		termansi.WithTerminal(cfg.tty),
		termansi.WithHyperlinkFallback(termansi.HyperlinkFallbackText),
	)
	ctx := table.NewRenderContext(th, a)
	columns := []table.Column[issueTableRow]{
		{
			Name:   "key",
			Header: "KEY",
			Render: func(row issueTableRow, ctx *table.RenderContext) table.Cell {
				text := row.Key
				if link := issueURL(cfg.baseURL, row.Key); link != "" {
					if cfg.tty {
						text = lipgloss.NewStyle().Underline(true).Render(text)
					}
					text = ctx.Ansi.Hyperlink(link, text)
				}
				return table.StyledCell(text, row.Key)
			},
		},
		{
			Name:   "summary",
			Header: "SUMMARY",
			Flex:   true,
			Render: func(row issueTableRow, _ *table.RenderContext) table.Cell {
				return table.TextCell(row.Summary)
			},
		},
		{
			Name:   "status",
			Header: "STATUS",
			Render: func(row issueTableRow, _ *table.RenderContext) table.Cell {
				return styledCell(statusStyle(cfg, row.Status), row.Status)
			},
		},
		{
			Name:   "assignee",
			Header: "ASSIGNEE",
			Flex:   true,
			Render: func(row issueTableRow, _ *table.RenderContext) table.Cell {
				assignee := firstNonEmpty(row.Assignee, "unassigned")
				return styledCell(assigneeStyle(cfg, row.Assignee), assignee)
			},
		},
		{
			Name:   "priority",
			Header: "PRIORITY",
			Render: func(row issueTableRow, _ *table.RenderContext) table.Cell {
				return styledCell(priorityStyle(cfg, row.Priority), row.Priority)
			},
		},
	}
	return table.NewRenderer(columns, ctx, table.WithTTY(cfg.tty), table.WithTermWidth(cfg.termWidth))
}

type primerTheme struct {
	theme  *clibtheme.Theme
	styled bool
}

func (t primerTheme) RenderBold(s string) string {
	if !t.styled || t.theme == nil || t.theme.Bold == nil {
		return s
	}
	return t.theme.Bold.Render(s)
}

func (t primerTheme) RenderDim(s string) string {
	if !t.styled || t.theme == nil || t.theme.Dim == nil {
		return s
	}
	return t.theme.Dim.Render(s)
}

func (t primerTheme) EntityColors() []color.Color {
	if !t.styled || t.theme == nil {
		return nil
	}
	return t.theme.EntityColors
}

func styledCell(style lipgloss.Style, text string) table.Cell {
	if text == "" {
		return table.TextCell("")
	}
	return table.StyledCell(style.Render(text), text)
}

func statusStyle(cfg plainConfig, status string) lipgloss.Style {
	if !cfg.tty {
		return lipgloss.NewStyle()
	}
	return hashStyle(cfg.theme, "status:"+status)
}

func priorityStyle(cfg plainConfig, priority string) lipgloss.Style {
	if !cfg.tty || cfg.theme == nil {
		return lipgloss.NewStyle()
	}
	switch normalizeStyleKey(priority) {
	case "blocker", "critical", "highest", "p0", "p1":
		return foregroundStyle(cfg.theme.Red).Bold(true)
	case "high", "p2":
		return foregroundStyle(cfg.theme.Orange).Bold(true)
	case "medium", "normal", "p3":
		return foregroundStyle(cfg.theme.Yellow)
	case "low", "p4":
		return foregroundStyle(cfg.theme.Green)
	case "lowest", "trivial", "p5":
		return dimStyle(cfg)
	default:
		return hashStyle(cfg.theme, "priority:"+priority)
	}
}

func assigneeStyle(cfg plainConfig, assignee string) lipgloss.Style {
	if strings.TrimSpace(assignee) == "" {
		return dimStyle(cfg)
	}
	if !cfg.tty {
		return lipgloss.NewStyle()
	}
	return hashStyle(cfg.theme, "assignee:"+assignee)
}

func dimStyle(cfg plainConfig) lipgloss.Style {
	if !cfg.tty || cfg.theme == nil || cfg.theme.Dim == nil {
		return lipgloss.NewStyle()
	}
	return *cfg.theme.Dim
}

func hashStyle(theme *clibtheme.Theme, key string) lipgloss.Style {
	if theme == nil || len(theme.EntityColors) == 0 || strings.TrimSpace(key) == "" {
		return lipgloss.NewStyle()
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(strings.TrimSpace(key))))

	return lipgloss.NewStyle().Foreground(theme.EntityColors[h.Sum32()%uint32(len(theme.EntityColors))])
}

func foregroundStyle(style *lipgloss.Style) lipgloss.Style {
	if style == nil {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Foreground(style.GetForeground())
}

func normalizeStyleKey(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func issueURL(baseURL, key string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	key = strings.TrimSpace(key)
	if baseURL == "" || key == "" {
		return ""
	}
	return baseURL + "/browse/" + url.PathEscape(key)
}

func formatAssignee(value any) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case *jira.User:
		if v == nil {
			return ""
		}
		return jiraUserName(*v)
	case jira.User:
		return jiraUserName(v)
	case map[string]any:
		for _, key := range []string{"displayName", "display_name", "name", "emailAddress", "email", "accountId", "account_id"} {
			if s, ok := v[key].(string); ok && strings.TrimSpace(s) != "" {
				return s
			}
		}
	}
	return formatHumanField(value)
}

func jiraUserName(user jira.User) string {
	if user.DisplayName != nil && strings.TrimSpace(*user.DisplayName) != "" {
		return *user.DisplayName
	}
	if user.EmailAddress != nil && strings.TrimSpace(*user.EmailAddress) != "" {
		return *user.EmailAddress
	}
	if user.AccountID != nil && strings.TrimSpace(*user.AccountID) != "" {
		return *user.AccountID
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func normalizeIssueRows(raw any) ([]map[string]any, bool) {
	switch issues := raw.(type) {
	case []map[string]any:
		return issues, true
	case []*jira.Issue:
		return issueMapsFromPointers(issues), true
	case []jira.Issue:
		rows := make([]map[string]any, 0, len(issues))
		for i := range issues {
			rows = append(rows, issueMapFromJiraIssue(&issues[i]))
		}
		return rows, true
	case []any:
		rows := make([]map[string]any, 0, len(issues))
		for _, issue := range issues {
			switch v := issue.(type) {
			case map[string]any:
				rows = append(rows, v)
			case *jira.Issue:
				rows = append(rows, issueMapFromJiraIssue(v))
			case jira.Issue:
				rows = append(rows, issueMapFromJiraIssue(&v))
			default:
				return nil, false
			}
		}
		return rows, true
	default:
		return nil, false
	}
}

func issueMapsFromPointers(issues []*jira.Issue) []map[string]any {
	rows := make([]map[string]any, 0, len(issues))
	for _, issue := range issues {
		rows = append(rows, issueMapFromJiraIssue(issue))
	}
	return rows
}

func issueMapFromJiraIssue(issue *jira.Issue) map[string]any {
	row := map[string]any{
		"key":      "",
		"summary":  "",
		"status":   "",
		"assignee": nil,
		"priority": "",
		"updated":  "",
	}
	if issue == nil {
		return row
	}
	if issue.Key != nil {
		row["key"] = *issue.Key
	}
	if issue.Fields == nil {
		return row
	}
	if issue.Fields.Summary != nil {
		row["summary"] = *issue.Fields.Summary
	}
	if issue.Fields.Status != nil && issue.Fields.Status.Name != nil {
		row["status"] = *issue.Fields.Status.Name
	}
	if issue.Fields.Assignee != nil {
		row["assignee"] = issue.Fields.Assignee
	}
	if issue.Fields.Priority != nil && issue.Fields.Priority.Name != nil {
		row["priority"] = *issue.Fields.Priority.Name
	}
	if issue.Fields.Updated != nil {
		row["updated"] = *issue.Fields.Updated
	}
	return row
}

func formatHumanField(value any) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

func plainFieldValue(value any) any {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	}
	rv := reflect.ValueOf(value)
	if rv.IsValid() && (rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array) && rv.Len() == 0 {
		return "[]"
	}
	b, err := json.Marshal(value)
	if err == nil {
		return string(b)
	}
	return fmt.Sprint(value)
}

func adfDocumentFromMap(value map[string]any) (adf.Document, bool) {
	if value["type"] != "doc" {
		return adf.Document{}, false
	}
	data, err := json.Marshal(value)
	if err != nil {
		return adf.Document{}, false
	}
	var doc adf.Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return adf.Document{}, false
	}
	return doc, true
}

func normalizePlain(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

// normalizeMapList accepts either []any or []map[string]any (Go can't
// type-assert across these even when the contents match) and returns a
// uniform slice. Handy when the caller built the data with concrete
// element types instead of the more permissive []any.
func normalizeMapList(v any) []map[string]any {
	switch list := v.(type) {
	case []map[string]any:
		return list
	case []any:
		out := make([]map[string]any, 0, len(list))
		for _, item := range list {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
}

// writeAuthStatusPlain renders the auth.status envelope as a per-profile
// block: identity, credential, and live permission grid. Replaces the
// generic key=value dump that mashed everything onto one line.
func writeAuthStatusPlain(logger *clog.Logger, data any, cfg plainConfig) error {
	m, ok := data.(map[string]any)
	if !ok {
		return writeGenericPlain(logger, "checked auth", data)
	}
	active, _ := m["active_profile"].(string)
	profiles := normalizeMapList(m["profiles"])
	if len(profiles) == 0 {
		// nothing to render — fall back to the generic dump so we at least
		// surface the envelope shape.
		return writeGenericPlain(logger, "checked auth", data)
	}

	th := cfg.theme
	style := authPlainStyle{tty: cfg.tty, theme: th}

	// User asked for the active profile only — auth status is the
	// "is THIS one working?" overview, not a multi-profile audit.
	rendered := 0
	for _, p := range profiles {
		name, _ := p["profile"].(string)
		if active != "" && name != active {
			continue
		}
		header := style.bold(name) + style.dim(" (active)")
		if rendered > 0 {
			logger.Info().Parts(clog.PartMessage).Msg("")
		}
		rendered++
		logger.Info().Parts(clog.PartMessage).Msg(style.label("Profile:    ") + header)

		valid, _ := p["valid"].(bool)
		source, _ := p["source"].(string)
		redacted, _ := p["redacted"].(string)
		credLine := style.dim(source) + "  " + style.dim(redacted)
		if valid {
			credLine = style.ok() + " " + credLine
		} else if errStr, _ := p["error"].(string); errStr != "" {
			credLine = style.no() + " " + style.dim(source) + "  " + style.warn(errStr)
		} else {
			credLine = style.no() + " " + credLine
		}
		logger.Info().Parts(clog.PartMessage).Msg(style.label("  Credential:  ") + credLine)

		remote, _ := p["remote"].(map[string]any)
		if remote == nil {
			logger.Info().Parts(clog.PartMessage).Msg(style.label("  Remote:      ") + style.dim("(probe skipped)"))
			continue
		}
		if site, _ := remote["site"].(string); site != "" {
			logger.Info().Parts(clog.PartMessage).Msg(style.label("  Site:        ") + style.url(site))
		}
		if myself, _ := remote["myself"].(map[string]any); myself != nil {
			logger.Info().Parts(clog.PartMessage).Msg(style.label("  Identity:    ") + formatAuthIdentity(myself, style))
			if hint, _ := myself["hint"].(string); hint != "" {
				logger.Info().Parts(clog.PartMessage).Msg("               " + style.warn(hint))
			}
		}
		if perms, _ := remote["permissions"].(map[string]any); perms != nil {
			logger.Info().Parts(clog.PartMessage).Msg(style.label("  Permissions: ") + formatAuthPerms(perms, style))
			if hint, _ := perms["hint"].(string); hint != "" {
				logger.Info().Parts(clog.PartMessage).Msg("               " + style.warn(hint))
			}
		}
	}
	return nil
}

// authPlainStyle centralizes theme decisions for the auth.status renderer
// so every line goes through the same tty/no-tty branching. Without TTY
// (test mode, no-color terminal) every method returns plain text.
type authPlainStyle struct {
	tty   bool
	theme *clibtheme.Theme
}

func (s authPlainStyle) bold(text string) string {
	if !s.tty || s.theme == nil || s.theme.Bold == nil {
		return text
	}
	return s.theme.Bold.Render(text)
}

func (s authPlainStyle) dim(text string) string {
	if !s.tty || s.theme == nil || s.theme.Dim == nil {
		return text
	}
	return s.theme.Dim.Render(text)
}

func (s authPlainStyle) label(text string) string { return s.dim(text) }

func (s authPlainStyle) url(text string) string {
	if !s.tty || s.theme == nil || s.theme.Blue == nil {
		return text
	}
	return s.theme.Blue.Render(text)
}

func (s authPlainStyle) warn(text string) string {
	if !s.tty || s.theme == nil || s.theme.Yellow == nil {
		return text
	}
	return s.theme.Yellow.Render(text)
}

func (s authPlainStyle) ok() string {
	if !s.tty {
		return "+"
	}
	if s.theme != nil && s.theme.Green != nil {
		return s.theme.Green.Render("✓")
	}
	return "✓"
}

func (s authPlainStyle) no() string {
	if !s.tty {
		return "-"
	}
	if s.theme != nil && s.theme.Red != nil {
		return s.theme.Red.Render("✗")
	}
	return "✗"
}

func (s authPlainStyle) emph(text string) string {
	if !s.tty || s.theme == nil || s.theme.Green == nil {
		return text
	}
	return s.theme.Green.Render(text)
}

func formatAuthIdentity(myself map[string]any, s authPlainStyle) string {
	if v, _ := myself["ok"].(bool); !v {
		status := ""
		if code, ok := myself["status"].(float64); ok {
			status = " HTTP " + strings.TrimSuffix(formatHumanField(code), ".0")
		}
		return s.no() + " /myself unreachable" + s.dim(status)
	}
	email, _ := myself["email"].(string)
	if email == "" {
		email = "(unknown)"
	}
	return s.ok() + " " + s.bold(email)
}

func formatAuthPerms(perms map[string]any, s authPlainStyle) string {
	if v, _ := perms["ok"].(bool); !v {
		return s.no() + " " + s.dim("/mypermissions failed")
	}
	grants := normalizeBoolMap(perms["grants"])
	if len(grants) == 0 {
		return s.dim("(none reported)")
	}
	names := make([]string, 0, len(grants))
	for k := range grants {
		names = append(names, k)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	granted := 0
	for _, name := range names {
		mark := s.no()
		if grants[name] {
			mark = s.ok()
			granted++
		}
		// Compact display: BROWSE_PROJECTS → browse
		short := strings.ToLower(strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(name, "_PROJECTS"), "_ISSUES"), "S"))
		parts = append(parts, mark+" "+short)
	}
	count := strings.TrimSuffix(formatHumanField(float64(granted)), ".0")
	total := strings.TrimSuffix(formatHumanField(float64(len(names))), ".0")
	prefix := ""
	if granted == len(names) {
		prefix = s.ok() + " " + s.emph("all "+count) + s.dim(" · ")
	} else {
		prefix = s.bold(count) + s.dim("/"+total) + s.dim(" · ")
	}
	return prefix + strings.Join(parts, s.dim("  "))
}

// normalizeBoolMap accepts a map keyed by string with boolean values that
// may have been built as map[string]bool (preferred for clarity) or
// map[string]any (after JSON round-trips). Returns a flat map[string]bool.
func normalizeBoolMap(v any) map[string]bool {
	switch m := v.(type) {
	case map[string]bool:
		return m
	case map[string]any:
		out := make(map[string]bool, len(m))
		for k, raw := range m {
			if b, ok := raw.(bool); ok {
				out[k] = b
			}
		}
		return out
	}
	return nil
}
