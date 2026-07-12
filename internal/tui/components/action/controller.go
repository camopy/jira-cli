package action

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/gechr/x/ptr"
	xstrings "github.com/gechr/x/strings"
	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/tui/components/input"
	"github.com/matcra587/jira-cli/internal/tui/components/picker"
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

// Controller collects input for one action against one issue. Text entry
// rides the shared input substrate: a real single-line input for the quick
// verbs and a textarea for comments when no external editor is configured.
type Controller struct {
	mode     Mode
	issueKey string

	// Transition choices (ModeTransition): a filterable picker — name shown,
	// id submitted.
	pick picker.Model

	// line collects the single-line text modes; area collects ModeComment.
	line input.Line
	area input.Area

	// summary is ModeCreate's first field (the area carries the description);
	// descFocused tracks which of the two owns the keyboard.
	summary     input.Line
	descFocused bool
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
// current summary for an edit). Comments get a multiline area; everything else
// a single-line input.
func (c *Controller) OpenText(mode Mode, issueKey, initial string) {
	c.reset()
	c.mode = mode
	c.issueKey = issueKey
	if mode.multiline() {
		c.area = input.NewArea("write a comment…", modalTextWidth, 6)
		c.area.SetValue(initial)
		return
	}
	c.line = input.NewLine("", "")
	c.line.SetWidth(modalTextWidth)
	c.line.SetValue(initial)
}

func (c *Controller) reset() {
	c.mode = ModeNone
	c.issueKey = ""
	c.pick = picker.Model{}
	c.line = input.Line{}
	c.area = input.Area{}
	c.summary = input.Line{}
	c.descFocused = false
}

// OpenCreate opens the two-field new-issue overlay targeting project. The
// summary line starts focused; tab (or enter on the summary) moves to the
// description area, and ctrl+s submits from either field.
func (c *Controller) OpenCreate(project string) {
	c.reset()
	c.mode = ModeCreate
	c.issueKey = project
	c.summary = input.NewLine("", "one-line summary")
	c.summary.SetWidth(modalTextWidth)
	c.area = input.NewArea("description (optional, Markdown)…", modalTextWidth, 5)
	c.area.Blur()
}

// toggleCreateFocus moves keyboard ownership between the create form's two
// fields, keeping exactly one cursor visible.
func (c *Controller) toggleCreateFocus() {
	c.descFocused = !c.descFocused
	if c.descFocused {
		c.summary.Blur()
		c.area.Focus()
		return
	}
	c.area.Blur()
	c.summary.Focus()
}

// Active reports whether an action is open.
func (c *Controller) Active() bool { return c.mode != ModeNone }

// Mode returns the current mode.
func (c *Controller) Mode() Mode { return c.mode }

// Cancel closes the controller without producing a request.
func (c *Controller) Cancel() { c.reset() }

// Update routes a message into the active control: the transition picker
// (up/down navigate, typing filters) or the text input (cursor movement,
// editing, paste).
func (c *Controller) Update(msg tea.Msg) tea.Cmd {
	switch {
	case c.mode == ModeCreate:
		if key, ok := msg.(tea.KeyPressMsg); ok {
			switch key.String() {
			case "tab", "shift+tab":
				c.toggleCreateFocus()
				return nil
			case "enter":
				// Enter on the one-line summary advances to the description
				// (a newline is meaningless there); in the area it stays a
				// newline via the textarea below.
				if !c.descFocused {
					c.toggleCreateFocus()
					return nil
				}
			}
		}
		if c.descFocused {
			return c.area.Update(msg)
		}
		return c.summary.Update(msg)
	case c.mode.isPick():
		return c.pick.Update(msg)
	case c.mode.multiline():
		return c.area.Update(msg)
	case c.mode.isText():
		return c.line.Update(msg)
	}
	return nil
}

// Text returns the current text-mode content.
func (c *Controller) Text() string {
	if c.mode.multiline() {
		return c.area.Value()
	}
	return c.line.Value()
}

// Multiline reports that the open action collects multiline text, so enter
// inserts a newline rather than submitting.
func (c *Controller) Multiline() bool { return c.mode.multiline() }

// Submit produces the Request and closes the controller. It reports false when
// the action is incomplete (no transitions, empty required text, or a create
// without a summary), leaving the controller open so the user can finish.
func (c *Controller) Submit() (Request, bool) {
	switch {
	case c.mode == ModeCreate:
		summary := strings.TrimSpace(c.summary.Value())
		if summary == "" {
			return Request{}, false
		}
		req := Request{Mode: ModeCreate, IssueKey: c.issueKey, Summary: summary, Text: c.area.Value()}
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
		text := c.Text()
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
// transition, or a labeled text prompt otherwise. Empty when no action is open.
func (c *Controller) View() string {
	switch {
	case c.mode.isPick():
		return c.pick.View()
	case c.mode == ModeCreate:
		return "create in " + c.issueKey + " (tab switches, ctrl+s submits):\n" +
			c.summary.View() + "\n" + c.area.View()
	case c.mode.multiline():
		return c.mode.String() + " (ctrl+s to submit):\n" + c.area.View()
	case c.mode.isText():
		return c.mode.String() + ": " + c.line.View()
	default:
		return ""
	}
}
