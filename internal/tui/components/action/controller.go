package action

import (
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	xstrings "github.com/gechr/x/strings"
	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/tui/components/form"
	"github.com/matcra587/jira-cli/internal/tui/components/input"
	"github.com/matcra587/jira-cli/internal/tui/theme"
)

// Mode is the verb the controller is collecting input for.
type Mode int

const (
	// ModeNone means no action is open.
	ModeNone Mode = iota
	// ModeComment captures comment text.
	ModeComment
	// ModeAssign captures an assignee query.
	ModeAssign
	// ModeLabels captures a comma-separated label list.
	ModeLabels
	// ModeCreate collects a new issue in a two-field overlay — summary line
	// plus Markdown description area; IssueKey carries the target project
	// key rather than an issue.
	ModeCreate
	// ModeWorklog captures a time string.
	ModeWorklog
	// ModeEdit captures a new summary.
	ModeEdit
	// ModeBulkAssign captures an assignee query to apply across a
	// multi-selection (resolved to one account id at apply time).
	ModeBulkAssign
	// ModeBulkComment captures comment text to post on every selected issue.
	ModeBulkComment
)

// String returns a lower-case label for the mode.
func (m Mode) String() string {
	switch m {
	case ModeComment:
		return "comment"
	case ModeAssign:
		return "assign"
	case ModeLabels:
		return "labels"
	case ModeCreate:
		return "create"
	case ModeWorklog:
		return "worklog"
	case ModeEdit:
		return "edit"
	case ModeBulkAssign:
		return "bulk assign"
	case ModeBulkComment:
		return "bulk comment"
	default:
		return "none"
	}
}

// isText reports whether a mode collects free text. Every non-create mode the
// controller now owns is a text mode; the transition and preset choice lists
// moved out to the section's dialog stack.
func (m Mode) isText() bool {
	return m == ModeComment || m == ModeAssign || m == ModeLabels || m == ModeWorklog ||
		m == ModeEdit || m == ModeBulkAssign || m == ModeBulkComment
}

// multiline reports whether a mode collects comment-style multiline text.
func (m Mode) multiline() bool { return m == ModeComment || m == ModeBulkComment || m == ModeCreate }

// Bulk reports whether the mode applies to a multi-selection.
func (m Mode) Bulk() bool {
	return m == ModeBulkAssign || m == ModeBulkComment
}

// Request is the result of a completed action, handed to the caller to execute.
type Request struct {
	Mode     Mode
	IssueKey string

	// Text is the free-text payload for the text modes.
	Text string

	// Summary is ModeCreate's first field; Text carries its description.
	Summary string

	// IssueType, Assignee, and Labels carry ModeCreate's remaining fields.
	// IssueType is the picked type name (always one of the create screen's
	// types). Assignee is the accountId behind an accepted assignee suggestion,
	// empty when none was accepted (the issue is created unassigned rather than
	// guessing at free text). Labels are the parsed, non-empty label tokens.
	IssueType string
	Assignee  string
	Labels    []string
}

// CreateConfig parameterizes the new-issue overlay. IssueTypes populates the
// type cycle field (DefaultType selects the starting option); the two Fetch
// funcs back the assignee and label autocompletes and run inside the form's
// fetch command — nil disables a field's suggestions but leaves it editable.
type CreateConfig struct {
	Project       string
	Projects      []string // selectable target projects; the pill starts on Project
	IssueTypes    []string
	DefaultType   string
	AssigneeFetch func(string) []form.Suggestion
	LabelFetch    func(string) []form.Suggestion
}

// The create form's field indices, fixed by OpenCreate's field order and read
// back in Request. Project leads (type and the rest depend on it).
const (
	createFieldProject = iota
	createFieldType
	createFieldSummary
	createFieldAssignee
	createFieldLabels
	createFieldDescription
)

// Outcome is what an Update asks the owner to do next. The controller closes
// itself on a cancel; a submit leaves it open so the owner reads the Request
// out through Submit.
type Outcome int

const (
	// OutcomeNone means the input was consumed and the action stays open.
	OutcomeNone Outcome = iota
	// OutcomeSubmit means the action completed — call Submit for the Request.
	OutcomeSubmit
	// OutcomeCancel means the user backed out; the controller already closed.
	OutcomeCancel
	// OutcomeEditor asks the owner to continue the draft (Draft) in the
	// external editor; the controller stays open until the owner closes it.
	OutcomeEditor
	// OutcomeChanged means the project selector stepped to a new value; the
	// owner refetches that project's issue types and pushes them back through
	// SetTypeOptions. The controller stays open.
	OutcomeChanged
)

// Controller collects input for one action against one issue. Every mode it
// owns now collects free text (or the two-field create) through the shared form
// component (focus ring, dirty-discard guard, inline hints); the transition and
// preset choice lists moved out to the section's dialog stack.
type Controller struct {
	mode     Mode
	issueKey string

	// form collects every text mode, from the one-line verbs to the
	// two-field create.
	form form.Model
}

// modalTextWidth is the inner width text inputs render at inside the action
// overlay (the modal box is capped at 66 columns with a border and padding).
const modalTextWidth = 60

// OpenText opens a free-text action with an optional initial value (e.g. the
// current summary for an edit). Comments get a multiline area — with ctrl+e
// as the external-editor escape hatch when one is configured — and everything
// else a single-line input.
func (c *Controller) OpenText(mode Mode, issueKey, initial string) {
	c.reset()
	c.mode = mode
	c.issueKey = issueKey
	spec := form.FieldSpec{Initial: initial, Placeholder: placeholderFor(mode)}
	cfg := form.Config{
		Title:  titleFor(mode, issueKey),
		Width:  modalTextWidth,
		Styles: formStyles(),
	}
	if mode.multiline() {
		spec.Multiline = true
		spec.Rows = 6
		// The hatch is single-issue only: the editor round-trip resumes by
		// issue key, which a bulk selection doesn't have.
		cfg.EditorHatch = mode == ModeComment && input.EditorCommand() != ""
	}
	// Labels and bulk assign accept empty text deliberately: clear all
	// labels, and unassign the selection (the bulk path confirms first).
	spec.Optional = mode == ModeLabels || mode == ModeBulkAssign
	if mode == ModeWorklog {
		// Catch an unparseable duration inline before the submit fans out, so
		// "2 hrs" surfaces under the field rather than failing the write. The
		// dispatch re-parses with the profile's real workday; validation only
		// needs the default to reject garbage.
		spec.Validate = func(s string) error {
			_, err := jira.ParseDuration(s, jira.DefaultWorkdaySeconds)
			return err
		}
	}
	cfg.Fields = []form.FieldSpec{spec}
	c.form = form.New(cfg)
}

func (c *Controller) reset() {
	c.mode = ModeNone
	c.issueKey = ""
	c.form = form.Model{}
}

// OpenCreate opens the new-issue overlay targeting cfg.Project: a type cycle
// field, the summary line, optional assignee and label fields with inline
// suggestions, and a Markdown description area. The type starts focused;
// tab moves between fields and ctrl+s submits from any of them.
func (c *Controller) OpenCreate(cfg CreateConfig) {
	c.reset()
	c.mode = ModeCreate
	c.issueKey = cfg.Project
	types := cfg.IssueTypes
	if len(types) == 0 {
		// Never leave the cycle field optionless — an empty Options list would
		// degrade it to a plain text input. Fall back to the default type alone.
		fallback := cfg.DefaultType
		if fallback == "" {
			fallback = "Task"
		}
		types = []string{fallback}
	}
	// The project pill leads the form: it names the create target and, when
	// stepped, drives a live refetch of the type list below it. It always holds
	// at least the current project, so its index is fixed whether or not other
	// projects are offered.
	projects := cfg.Projects
	if len(projects) == 0 {
		projects = []string{cfg.Project}
	}
	// Each field carries its name — the project/type selectors as pill captions,
	// the text fields as their box titles — with placeholders left to hint at the
	// expected input rather than repeat the label. The form is taller than a
	// small terminal now, but the dialog Shell scrolls to follow focus and pins
	// the foot row, so the hints and any discard confirmation stay visible.
	c.form = form.New(form.Config{
		Title:  "new issue",
		Width:  modalTextWidth,
		Styles: formStyles(),
		Fields: []form.FieldSpec{
			{Label: "project", Options: projects, Initial: cfg.Project, Notify: true},
			{Label: "type", Options: types, Initial: cfg.DefaultType},
			{Label: "summary", Placeholder: "a one-line summary"},
			{Label: "assignee", Placeholder: "type a name", Optional: true, Autocomplete: assigneeAutocomplete(cfg.AssigneeFetch)},
			{Label: "labels", Placeholder: "comma-separated", Optional: true, Autocomplete: labelAutocomplete(cfg.LabelFetch)},
			{Label: "description", Placeholder: "Markdown", Multiline: true, Rows: 3, Optional: true},
		},
	})
}

// assigneeAutocomplete completes the whole assignee field against fetch: a name
// carries spaces, so the query is the entire field (IsBoundary never fires)
// rather than the trailing word. Each suggestion's Detail is the accountId,
// recovered in Request without a re-resolve. Nil fetch disables suggestions.
func assigneeAutocomplete(fetch func(string) []form.Suggestion) *form.Autocomplete {
	if fetch == nil {
		return nil
	}
	return &form.Autocomplete{
		MinQuery:   3,
		IsBoundary: func(rune) bool { return false },
		Fetch:      fetch,
	}
}

// labelAutocomplete completes one comma-separated label at a time: commas and
// whitespace bound the token so each entry fetches and accepts alone. Nil fetch
// disables suggestions.
func labelAutocomplete(fetch func(string) []form.Suggestion) *form.Autocomplete {
	if fetch == nil {
		return nil
	}
	return &form.Autocomplete{
		MinQuery:   2,
		IsBoundary: func(r rune) bool { return r == ',' || unicode.IsSpace(r) },
		Fetch:      fetch,
	}
}

// titleFor is the overlay heading: the verb plus its target, so the modal
// says what it is about to do without a separate label row.
func titleFor(mode Mode, issueKey string) string {
	switch mode {
	case ModeBulkComment:
		return "comment on selection"
	case ModeBulkAssign:
		return "assign selection"
	case ModeComment:
		return "comment on " + issueKey
	case ModeAssign:
		return "assign " + issueKey
	case ModeLabels:
		return "labels for " + issueKey
	case ModeWorklog:
		return "log work on " + issueKey
	case ModeEdit:
		return "edit " + issueKey
	default:
		return strings.TrimSpace(mode.String() + " " + issueKey)
	}
}

// placeholderFor hints at each verb's expected input format.
func placeholderFor(mode Mode) string {
	switch mode {
	case ModeComment, ModeBulkComment:
		return "write a comment… (Markdown)"
	case ModeAssign, ModeBulkAssign:
		return "name or email — none clears"
	case ModeLabels:
		return "comma-separated — empty clears all"
	case ModeWorklog:
		return "e.g. 2h 30m"
	default:
		return ""
	}
}

// formStyles wires the shared theme into the form's injected styles, keeping
// the form package itself theme-agnostic.
func formStyles() form.Styles {
	return form.Styles{
		Title:              theme.DetailHeader,
		Label:              theme.HelpDesc,
		LabelFocused:       theme.HelpKey,
		Border:             lipgloss.NewStyle().Foreground(theme.Theme.Dim.GetForeground()),
		BorderFocused:      lipgloss.NewStyle().Foreground(theme.Theme.Yellow.GetForeground()),
		HintKey:            theme.HelpKey,
		HintText:           theme.HelpDesc,
		Question:           lipgloss.NewStyle().Foreground(theme.Theme.Yellow.GetForeground()).Bold(true),
		Suggestion:         theme.DetailValue,
		SuggestionSelected: lipgloss.NewStyle().Bold(true).Reverse(true),
		Error:              theme.StatusErr,
	}
}

// Active reports whether an action is open.
func (c *Controller) Active() bool { return c.mode != ModeNone }

// Mode returns the current mode.
func (c *Controller) Mode() Mode { return c.mode }

// Update routes a message into the open form and reports what completed. The
// form modes carry their own submit, cancel, guard, and editor-hatch contract.
func (c *Controller) Update(msg tea.Msg) (tea.Cmd, Outcome) {
	if c.mode == ModeNone {
		return nil, OutcomeNone
	}
	cmd, ev, _ := c.form.Update(msg)
	switch ev {
	case form.EventSubmit:
		return cmd, OutcomeSubmit
	case form.EventCancel:
		c.reset()
		return cmd, OutcomeCancel
	case form.EventEditor:
		return cmd, OutcomeEditor
	case form.EventChanged:
		return cmd, OutcomeChanged
	case form.EventNone:
	}
	return cmd, OutcomeNone
}

// Project returns the create form's selected target project (the project pill's
// value). Meaningful only in ModeCreate; empty otherwise.
func (c *Controller) Project() string {
	if c.mode != ModeCreate {
		return ""
	}
	return c.form.Value(createFieldProject)
}

// SetTypeOptions swaps the create form's issue-type list — the owner's response
// to an OutcomeChanged after refetching the newly selected project's types. An
// empty list is ignored so the field never degrades to a blank.
func (c *Controller) SetTypeOptions(types []string) {
	if c.mode != ModeCreate {
		return
	}
	c.form.SetOptions(createFieldType, types)
}

// Draft returns the open form's primary text — what an OutcomeEditor carries
// into the external editor so no keystroke is lost in the handoff.
func (c *Controller) Draft() string { return c.form.Value(0) }

// IssueKey returns the open action's target (the project key for ModeCreate).
func (c *Controller) IssueKey() string { return c.issueKey }

// Request reads the completed action's payload without closing the controller
// — the async submit lifecycle keeps the form open until the write resolves, so
// unlike a reset-on-success this leaves the draft in place. It reports false
// when the action is incomplete (empty required text, or a create without a
// summary); the form's own required-field gate already blocks that path, so a
// caller acting on OutcomeSubmit always sees true.
func (c *Controller) Request() (Request, bool) {
	switch {
	case c.mode == ModeCreate:
		summary := strings.TrimSpace(c.form.Value(createFieldSummary))
		if summary == "" {
			return Request{}, false
		}
		// The project pill is the create target — it may differ from the project
		// the overlay opened on if the user stepped it — falling back to the
		// opening project when the pill is somehow blank.
		project := c.form.Value(createFieldProject)
		if project == "" {
			project = c.issueKey
		}
		return Request{
			Mode:      ModeCreate,
			IssueKey:  project,
			Summary:   summary,
			Text:      c.form.Value(createFieldDescription),
			IssueType: c.form.Value(createFieldType),
			Assignee:  c.form.AcceptedDetail(createFieldAssignee),
			Labels:    xstrings.SplitCSV(c.form.Value(createFieldLabels)),
		}, true
	case c.mode.isText():
		text := c.form.Value(0)
		// Labels and bulk assign accept empty text deliberately: clear all
		// labels, and unassign the selection (the bulk path confirms first).
		if xstrings.IsBlank(text) && c.mode != ModeLabels && c.mode != ModeBulkAssign {
			return Request{}, false // empty comment/summary/etc is not a submit
		}
		return Request{Mode: c.mode, IssueKey: c.issueKey, Text: text}, true
	default:
		return Request{}, false
	}
}

// SetSubmitting freezes the form on an async submit, showing frame (a spinner
// glyph) plus "submitting…" until the write resolves.
func (c *Controller) SetSubmitting(frame string) { c.form.SetSubmitting(frame) }

// SetError clears the submitting state and surfaces msg as the form's
// inline error, leaving the draft intact for a retry.
func (c *Controller) SetError(msg string) { c.form.SetError(msg) }

// View renders the open action's titled form as overlay content, empty when no
// action is open (the zero-value form renders empty on its own).
func (c *Controller) View() string { return c.form.View() }

// Body is the scrollable part of the form (title + fields); Foot is its pinned
// chrome (the hint / submitting / discard-confirm row). A dialog that scrolls a
// tall create form draws the two separately so the foot never scrolls off.
func (c *Controller) Body() string { return c.form.Body() }

// Foot is the form's pinned foot row — see [Controller.Body].
func (c *Controller) Foot() string { return c.form.Foot() }

// FocusRegion forwards the form's focused-field position so a framing Shell can
// scroll to follow focus through a tall create form. It is meaningful only
// while an action is open.
func (c *Controller) FocusRegion() (top, height int, ok bool) { return c.form.FocusRegion() }
