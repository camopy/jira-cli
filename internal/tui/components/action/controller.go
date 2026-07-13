package action

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/gechr/x/ptr"
	xstrings "github.com/gechr/x/strings"
	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/tui/components/form"
	"github.com/matcra587/jira-cli/internal/tui/components/input"
	"github.com/matcra587/jira-cli/internal/tui/components/picker"
	"github.com/matcra587/jira-cli/internal/tui/theme"
)

// Mode is the verb the controller is collecting input for.
type Mode int

const (
	// ModeNone means no action is open.
	ModeNone Mode = iota
	// ModeTransition picks a workflow transition from a choice list.
	ModeTransition
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
	// ModeBulkTransition picks a workflow transition to apply across a
	// multi-selection. The choices come from one representative issue; the
	// match is by name, with the per-issue id resolved at apply time.
	ModeBulkTransition
	// ModeBulkAssign captures an assignee query to apply across a
	// multi-selection (resolved to one account id at apply time).
	ModeBulkAssign
	// ModeBulkComment captures comment text to post on every selected issue.
	ModeBulkComment
	// ModePreset picks a saved query from a choice list; the selection's
	// value (the JQL) comes back in Request.Text. The owning section drives
	// this mode itself — results.submitAction never sees it.
	ModePreset
)

// String returns a lower-case label for the mode.
func (m Mode) String() string {
	switch m {
	case ModeTransition:
		return "transition"
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
	case ModeBulkTransition:
		return "bulk transition"
	case ModeBulkAssign:
		return "bulk assign"
	case ModeBulkComment:
		return "bulk comment"
	case ModePreset:
		return "preset"
	default:
		return "none"
	}
}

// isText reports whether a mode collects free text (vs. a choice list).
func (m Mode) isText() bool {
	return m == ModeComment || m == ModeAssign || m == ModeLabels || m == ModeWorklog ||
		m == ModeEdit || m == ModeBulkAssign || m == ModeBulkComment
}

// isPick reports whether a mode selects from a choice picker.
func (m Mode) isPick() bool {
	return m == ModeTransition || m == ModeBulkTransition || m == ModePreset
}

// multiline reports whether a mode collects comment-style multiline text.
func (m Mode) multiline() bool { return m == ModeComment || m == ModeBulkComment || m == ModeCreate }

// Bulk reports whether the mode applies to a multi-selection.
func (m Mode) Bulk() bool {
	return m == ModeBulkTransition || m == ModeBulkAssign || m == ModeBulkComment
}

// Request is the result of a completed action, handed to the caller to execute.
type Request struct {
	Mode     Mode
	IssueKey string

	// TransitionID/TransitionName are set for ModeTransition. The ID is what
	// Jira needs; the name is what the user saw, for optimistic display.
	TransitionID   string
	TransitionName string

	// Text is the free-text payload for the text modes.
	Text string

	// Summary is ModeCreate's first field; Text carries its description.
	Summary string
}

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
)

// Controller collects input for one action against one issue. Text entry
// rides the shared form component (focus ring, dirty-discard guard, inline
// hints); transition and preset choices ride the filterable picker.
type Controller struct {
	mode     Mode
	issueKey string

	// Transition choices (ModeTransition): a filterable picker — name shown,
	// id submitted.
	pick picker.Model

	// form collects every text mode, from the one-line verbs to the
	// two-field create.
	form form.Model
}

// modalTextWidth is the inner width text inputs render at inside the action
// overlay (the modal box is capped at 66 columns with a border and padding).
const modalTextWidth = 60

// OpenTransition opens the transition picker for an issue with its valid
// transitions. Only the transitions Jira allows for this issue are passed in,
// so the user can never pick an invalid one.
func (c *Controller) OpenTransition(issueKey string, transitions []*jira.Transition) {
	c.openPick(ModeTransition, issueKey, "Transition to:", transitions)
}

// OpenBulkTransition opens the same picker for a multi-selection. The choices
// come from one representative marked issue; issues whose workflow doesn't
// offer the picked name report a per-issue failure at apply time.
func (c *Controller) OpenBulkTransition(transitions []*jira.Transition) {
	c.openPick(ModeBulkTransition, "", "Transition selection to:", transitions)
}

// OpenPreset opens a choice list of saved queries. Item values carry the JQL
// and come back whole in Request.Text on submit.
func (c *Controller) OpenPreset(items []picker.Item) {
	c.reset()
	c.mode = ModePreset
	c.pick = picker.New("Run saved query:", items)
}

func (c *Controller) openPick(mode Mode, issueKey, title string, transitions []*jira.Transition) {
	c.reset()
	c.mode = mode
	c.issueKey = issueKey
	items := make([]picker.Item, 0, len(transitions))
	for _, t := range transitions {
		if t == nil {
			continue
		}
		items = append(items, picker.Item{Label: ptr.Deref(t.Name), Value: ptr.Deref(t.ID)})
	}
	c.pick = picker.New(title, items)
}

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
	cfg.Fields = []form.FieldSpec{spec}
	c.form = form.New(cfg)
}

func (c *Controller) reset() {
	c.mode = ModeNone
	c.issueKey = ""
	c.pick = picker.Model{}
	c.form = form.Model{}
}

// OpenCreate opens the two-field new-issue overlay targeting project. The
// summary line starts focused; tab (or enter on the summary) moves to the
// description area, and ctrl+s submits from either field.
func (c *Controller) OpenCreate(project string) {
	c.reset()
	c.mode = ModeCreate
	c.issueKey = project
	c.form = form.New(form.Config{
		Title:  "create in " + project,
		Width:  modalTextWidth,
		Styles: formStyles(),
		Fields: []form.FieldSpec{
			{Placeholder: "one-line summary"},
			{Placeholder: "description (optional, Markdown)…", Multiline: true, Rows: 5, Optional: true},
		},
	})
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
		HintKey:            theme.HelpKey,
		HintText:           theme.HelpDesc,
		Question:           lipgloss.NewStyle().Foreground(theme.Theme.Yellow.GetForeground()).Bold(true),
		Suggestion:         theme.DetailValue,
		SuggestionSelected: lipgloss.NewStyle().Bold(true).Reverse(true),
	}
}

// Active reports whether an action is open.
func (c *Controller) Active() bool { return c.mode != ModeNone }

// Mode returns the current mode.
func (c *Controller) Mode() Mode { return c.mode }

// Cancel closes the controller without producing a request.
func (c *Controller) Cancel() { c.reset() }

// Update routes a message into the active control and reports what completed.
// The pickers handle esc/enter here (cancel and submit); the form modes carry
// their own submit, cancel, guard, and editor-hatch contract.
func (c *Controller) Update(msg tea.Msg) (tea.Cmd, Outcome) {
	switch {
	case c.mode == ModeNone:
		return nil, OutcomeNone
	case c.mode.isPick():
		if key, ok := msg.(tea.KeyPressMsg); ok {
			switch key.String() {
			case "esc":
				c.reset()
				return nil, OutcomeCancel
			case "enter":
				return nil, OutcomeSubmit
			}
		}
		return c.pick.Update(msg), OutcomeNone
	default:
		cmd, ev, _ := c.form.Update(msg)
		switch ev {
		case form.EventSubmit:
			return cmd, OutcomeSubmit
		case form.EventCancel:
			c.reset()
			return cmd, OutcomeCancel
		case form.EventEditor:
			return cmd, OutcomeEditor
		case form.EventNone:
		}
		return cmd, OutcomeNone
	}
}

// Draft returns the open form's primary text — what an OutcomeEditor carries
// into the external editor so no keystroke is lost in the handoff.
func (c *Controller) Draft() string { return c.form.Value(0) }

// IssueKey returns the open action's target (the project key for ModeCreate).
func (c *Controller) IssueKey() string { return c.issueKey }

// Submit produces the Request and closes the controller. It reports false when
// the action is incomplete (no transitions, empty required text, or a create
// without a summary), leaving the controller open so the user can finish.
func (c *Controller) Submit() (Request, bool) {
	switch {
	case c.mode == ModeCreate:
		summary := strings.TrimSpace(c.form.Value(0))
		if summary == "" {
			return Request{}, false
		}
		req := Request{Mode: ModeCreate, IssueKey: c.issueKey, Summary: summary, Text: c.form.Value(1)}
		c.reset()
		return req, true
	case c.mode.isPick():
		sel, ok := c.pick.Selected()
		if !ok {
			return Request{}, false
		}
		if c.mode == ModePreset {
			req := Request{Mode: ModePreset, Text: sel.Value}
			c.reset()
			return req, true
		}
		req := Request{
			Mode:           c.mode,
			IssueKey:       c.issueKey,
			TransitionID:   sel.Value,
			TransitionName: sel.Label,
		}
		if c.mode == ModeBulkTransition {
			// The id belongs to the representative issue only; bulk applies
			// by name with per-issue ids resolved at apply time. Zeroing it
			// keeps a mode-unaware caller from posting the wrong id.
			req.TransitionID = ""
		}
		c.reset()
		return req, true
	case c.mode.isText():
		text := c.form.Value(0)
		// Labels and bulk assign accept empty text deliberately: clear all
		// labels, and unassign the selection (the bulk path confirms first).
		if xstrings.IsBlank(text) && c.mode != ModeLabels && c.mode != ModeBulkAssign {
			return Request{}, false // empty comment/summary/etc is not a submit
		}
		req := Request{Mode: c.mode, IssueKey: c.issueKey, Text: text}
		c.reset()
		return req, true
	default:
		return Request{}, false
	}
}

// View renders the open action as overlay content: a choice list for a
// transition, or the titled form otherwise. Empty when no action is open.
func (c *Controller) View() string {
	switch {
	case c.mode == ModeNone:
		return ""
	case c.mode.isPick():
		return c.pick.View()
	default:
		return c.form.View()
	}
}
