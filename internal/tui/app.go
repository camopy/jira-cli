// Package tui implements the persistent Jira dashboard.
//
// Architecture mirrors pagerduty-client/internal/tui:
//   - App is a value-typed Bubble Tea root model owning all sub-models,
//     overlays and the global key map.
//   - WindowSizeMsg cascades down a single source of truth (bodyH) to every
//     child so layout never drifts.
//   - Update routes overlay keys first (cascade), then global keys, then
//     per-view actions, then forwards unhandled keys to the active view.
//   - View composes header / body / footer zones and layers overlays.
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/tui/components"
	"github.com/matcra587/jira-cli/internal/tui/theme"
)

// view enumerates the top-level screens.
type view int

const (
	viewDashboard view = iota
	viewDetail
)

// IssueProvider supplies issues for the dashboard list.
// (Preserved from prior API for cmd/jira/tui.go integration.)
type IssueProvider interface {
	ListIssues(context.Context) ([]*jira.Issue, error)
}

// IssueProviderFunc adapts a function to IssueProvider.
type IssueProviderFunc func(context.Context) ([]*jira.Issue, error)

// ListIssues calls the wrapped function.
func (f IssueProviderFunc) ListIssues(ctx context.Context) ([]*jira.Issue, error) {
	return f(ctx)
}

// MutationService is the seam for issue mutations triggered from the TUI.
// (Preserved from prior API.)
type MutationService interface {
	UpdateIssue(context.Context, string, map[string]any) (*jira.Issue, error)
	CreateIssue(context.Context, *jira.IssueCreateRequest) (*jira.Issue, error)
	TransitionIssue(context.Context, string, *jira.TransitionRequest) error
	AddComment(context.Context, string, *jira.CommentAddRequest) (*jira.Comment, error)
	AddWorklog(context.Context, string, *jira.WorklogAddRequest) (*jira.Worklog, error)
	CloneIssue(context.Context, string, *jira.IssueCloneRequest) (*jira.Issue, error)
	MoveIssue(context.Context, string, *jira.IssueMoveRequest) (*jira.Issue, error)
	DeleteIssue(context.Context, string) error
}

// Options configures a TUI run.
type Options struct {
	IssueProvider   IssueProvider
	MutationService MutationService
	InitialError    string
	Profiles        []string
	ActiveProfile   string
	BaseURL         string
	RefreshEvery    time.Duration
	Theme           string
	// Identity used to resolve "me" / "team" filters in the issues tab.
	// Email is matched against assignee.email_address (Jira Cloud).
	// AccountID is matched against assignee.account_id when known.
	// TeamAccountIDs lists teammates whose issues count as "my team".
	Email          string
	AccountID      string
	TeamAccountIDs []string
}

// App is the root Bubble Tea model.
type App struct {
	ctx       context.Context //nolint:containedctx // value-typed model
	cancel    context.CancelFunc
	provider  IssueProvider
	mutations MutationService
	profiles  []string
	profile   string
	baseURL   string
	interval  time.Duration

	current   view
	tabs      []string
	activeTab int
	dashboard Dashboard
	detail    *issueDetail

	statusBar  components.StatusBar
	help       components.Help
	confirm    components.Confirm
	textInput  components.TextInput
	filterOpts components.FilterOptions

	spinner   spinner.Model
	loading   bool
	loadError string
	width     int
	height    int
	bodyH     int
	statusID  int
	paused    bool
	editing   bool

	// Identity for resolving "me" / "team" filters.
	email     string
	accountID string
	teamIDs   []string

	keys appKeyMap
}

// New constructs an App using default options.
func New(ctx context.Context) App { return NewWithOptions(ctx, Options{}) }

// NewWithOptions builds an App wired to the supplied provider/services.
func NewWithOptions(ctx context.Context, opts Options) App {
	ctx, cancel := context.WithCancel(ctx)
	theme.Apply(theme.Resolve(opts.Theme))

	interval := opts.RefreshEvery
	if interval <= 0 {
		interval = 30 * time.Second
	}
	profiles := opts.Profiles
	active := opts.ActiveProfile
	if active == "" && len(profiles) > 0 {
		active = profiles[0]
	}
	if active == "" {
		active = "default"
	}
	if len(profiles) == 0 {
		profiles = []string{active}
	}

	tabs := []string{"issues", "epics", "search", "activity"}

	sp := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	sp.Style = lipgloss.NewStyle().Foreground(theme.ColorHeaderFg)

	const defaultWidth, defaultHeight = 100, 30
	a := App{
		ctx:        ctx,
		cancel:     cancel,
		provider:   opts.IssueProvider,
		mutations:  opts.MutationService,
		profiles:   profiles,
		profile:    active,
		baseURL:    opts.BaseURL,
		interval:   interval,
		current:    viewDashboard,
		tabs:       tabs,
		dashboard:  newDashboard(ctx, opts.BaseURL),
		statusBar:  components.StatusBar{Profile: active, Width: defaultWidth},
		textInput:  components.NewTextInput(),
		filterOpts: components.NewFilterOptions(),
		spinner:    sp,
		loadError:  opts.InitialError,
		width:      defaultWidth,
		height:     defaultHeight,
		bodyH:      defaultHeight - 4,
		email:      opts.Email,
		accountID:  opts.AccountID,
		teamIDs:    append([]string(nil), opts.TeamAccountIDs...),
		keys:       newAppKeyMap(),
	}
	a.statusBar.LastRefresh = time.Now()
	a.dashboard.width = defaultWidth
	a.dashboard.height = a.bodyH
	a.dashboard.issues.width = defaultWidth
	a.dashboard.issues.height = a.bodyH

	// Eager initial fetch so the first View() has content even when the
	// caller has not pumped Init's command queue (matters for tests and
	// for the welcome render before the polling tick fires).
	if opts.IssueProvider != nil && opts.InitialError == "" {
		if issues, err := opts.IssueProvider.ListIssues(ctx); err != nil {
			a.loadError = err.Error()
		} else {
			a.dashboard.SetIssues(issues)
			a.statusBar.Total = len(issues)
		}
	}
	return a
}

// Run starts the program with default options.
func Run(ctx context.Context) (tea.Model, error) { return RunWithOptions(ctx, Options{}) }

// RunWithOptions starts a Bubble Tea program with the given options.
func RunWithOptions(ctx context.Context, opts Options) (tea.Model, error) {
	return tea.NewProgram(NewWithOptions(ctx, opts), tea.WithContext(ctx)).Run()
}

// Init implements tea.Model.
func (a App) Init() tea.Cmd {
	cmds := []tea.Cmd{a.dashboard.Init()}
	if a.provider != nil {
		cmds = append(cmds, a.fetchIssuesCmd(), a.tickCmd(), a.spinner.Tick)
	}
	return tea.Batch(cmds...)
}

// Update implements tea.Model.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return a.handleResize(msg)
	case tea.KeyPressMsg:
		return a.updateKeyPress(msg)
	case spinner.TickMsg:
		if a.loading {
			var cmd tea.Cmd
			a.spinner, cmd = a.spinner.Update(msg)
			return a, cmd
		}
		return a, nil
	case tickMsg:
		if !a.paused && !a.editing && a.provider != nil {
			a.statusBar.LastRefresh = time.Time(msg)
			return a, tea.Batch(a.tickCmd(), a.fetchIssuesCmd())
		}
		if a.provider != nil {
			return a, a.tickCmd()
		}
		return a, nil
	case issuesLoadedMsg:
		a.loading = false
		if msg.err != nil {
			a.loadError = msg.err.Error()
			return a.flashResult(a.loadError, true)
		}
		a.loadError = ""
		a.dashboard.SetIssues(msg.issues)
		a.statusBar.Total = len(msg.issues)
		return a, nil
	case clearStatusMsg:
		if msg.id == a.statusID {
			a.statusBar.StatusMsg = ""
		}
		return a, nil
	case browserOpenedMsg:
		if msg.err != nil {
			return a.flashResult("Open browser failed: "+msg.err.Error(), true)
		}
		return a.flashResult("Opened: "+msg.url, false)
	case mutationDoneMsg:
		a.editing = false
		if msg.err != nil {
			return a.flashResult(msg.kind+" "+msg.issueKey+" failed: "+msg.err.Error(), true)
		}
		hint := strings.TrimSpace("submitted " + msg.kind + " " + msg.issueKey)
		af, fcmd := a.flashResult(hint, false)
		if a.provider != nil {
			return af, tea.Batch(fcmd, a.fetchIssuesCmd())
		}
		return af, fcmd
	case SubmitEditMsg:
		return a, a.submitEditCmd(msg)
	case SubmitCreateMsg:
		return a, a.submitCreateCmd(msg)
	case SubmitTransitionMsg:
		return a, a.submitTransitionCmd(msg)
	case SubmitCommentMsg:
		return a, a.submitCommentCmd(msg)
	case SubmitWorklogMsg:
		return a, a.submitWorklogCmd(msg)
	case SubmitCloneMsg:
		return a, a.submitCloneCmd(msg)
	case SubmitMoveMsg:
		return a, a.submitMoveCmd(msg)
	case SubmitDeleteMsg:
		return a, a.submitDeleteCmd(msg)
	case components.ConfirmResult:
		if msg.Confirmed && msg.OnYes != nil {
			return a, msg.OnYes
		}
		return a, nil
	case components.InputSubmitted:
		return a.handleInputSubmitted(msg)
	case components.InputCancelled:
		a.editing = false
		return a, nil
	case components.FilterAppliedMsg:
		if msg.Origin == "issues" {
			state := components.IssueFilterState{
				Assignment: msg.Selections["Assignment"],
				Status:     msg.Selections["Status"],
				Priority:   msg.Selections["Priority"],
			}
			a.dashboard.issues.SetFilterState(state, a.email, a.accountID, a.teamIDs)
		}
		return a, nil
	case components.FilterClosed:
		return a, nil
	case IssueSelected:
		d := newIssueDetail(msg.Issue)
		d.setSize(a.width, a.bodyH-1)
		d.syncContent()
		a.detail = &d
		a.current = viewDetail
		return a, nil
	}

	switch a.current {
	case viewDashboard:
		dm, cmd := a.dashboard.Update(msg)
		a.dashboard = dm.(Dashboard)
		return a, cmd
	case viewDetail:
		if a.detail != nil {
			dm, cmd := a.detail.Update(msg)
			d := dm.(issueDetail)
			a.detail = &d
			return a, cmd
		}
	}
	return a, nil
}

func (a App) handleResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	a.width = msg.Width
	a.height = msg.Height
	a.statusBar.Width = msg.Width
	const headerH = 2
	const footerH = 2
	a.bodyH = max(a.height-headerH-footerH, 1)
	child := tea.WindowSizeMsg{Width: msg.Width, Height: a.bodyH}
	dm, _ := a.dashboard.Update(child)
	a.dashboard = dm.(Dashboard)
	if a.detail != nil {
		dm2, _ := a.detail.Update(tea.WindowSizeMsg{Width: msg.Width, Height: a.bodyH - 1})
		d := dm2.(issueDetail)
		a.detail = &d
	}
	return a, nil
}

func (a App) updateKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.textInput.Visible {
		tm, cmd := a.textInput.Update(msg)
		a.textInput = tm.(components.TextInput)
		return a, cmd
	}
	if a.confirm.Visible {
		cm, cmd := a.confirm.Update(msg)
		a.confirm = cm.(components.Confirm)
		return a, cmd
	}
	if a.filterOpts.Visible {
		fm, cmd := a.filterOpts.Update(msg)
		a.filterOpts = fm.(components.FilterOptions)
		return a, cmd
	}
	if a.help.Visible {
		hm, cmd := a.help.Update(msg)
		a.help = hm.(components.Help)
		return a, cmd
	}

	if a.current == viewDashboard && a.dashboard.FilterActive() {
		dm, cmd := a.dashboard.Update(msg)
		a.dashboard = dm.(Dashboard)
		return a, cmd
	}

	km := a.keys

	if key.Matches(msg, km.Back) && a.current == viewDetail {
		a.current = viewDashboard
		a.detail = nil
		return a, nil
	}

	if a.current == viewDashboard {
		if idx := tabIndexFromKey(msg.String()); idx >= 0 && idx < len(a.tabs) {
			a.activeTab = idx
			return a, nil
		}
		switch {
		case key.Matches(msg, km.Tab):
			a.activeTab = (a.activeTab + 1) % len(a.tabs)
			return a, nil
		case key.Matches(msg, km.ShiftTab):
			a.activeTab = (a.activeTab - 1 + len(a.tabs)) % len(a.tabs)
			return a, nil
		}
	}

	switch {
	case key.Matches(msg, km.Quit):
		a.cancel()
		return a, tea.Quit
	case key.Matches(msg, km.Help):
		a.help.Visible = true
		a.help.CurrentView = a.currentViewName()
		return a, nil
	case key.Matches(msg, km.TogglePause):
		a.paused = !a.paused
		a.statusBar.Paused = a.paused
		if !a.paused && a.provider != nil {
			return a, a.tickCmd()
		}
		return a, nil
	case key.Matches(msg, km.Refresh):
		if a.editing {
			return a.flashResult("refresh paused while editing", true)
		}
		if a.provider == nil {
			return a, nil
		}
		return a, a.fetchIssuesCmd()
	case key.Matches(msg, km.Profile):
		a.cycleProfile()
		return a, nil
	case key.Matches(msg, km.FilterOpts):
		if a.current == viewDashboard && a.activeTabID() == "issues" {
			a.filterOpts = a.filterOpts.ShowWithRows("issues", components.IssueFilterRows(a.dashboard.issues.filterState))
		}
		return a, nil
	}

	if a.current == viewDashboard && a.activeTabID() == "issues" {
		return a.dashboardActionKey(msg)
	}
	if a.current == viewDetail {
		return a.detailActionKey(msg)
	}
	if a.current == viewDashboard {
		dm, cmd := a.dashboard.Update(msg)
		a.dashboard = dm.(Dashboard)
		return a, cmd
	}
	return a, nil
}

func (a App) dashboardActionKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	km := a.keys
	switch {
	case key.Matches(msg, km.Open):
		if iss := a.dashboard.SelectedIssue(); iss != nil {
			return a, func() tea.Msg { return IssueSelected{Issue: *iss} }
		}
		return a, nil
	case key.Matches(msg, km.OpenBrowse):
		return a.openInBrowser()
	case key.Matches(msg, km.Edit):
		return a.openInputForSelected("edit-summary", "New summary for ", "")
	case key.Matches(msg, km.AssignMe):
		return a.assignSelectedToMe()
	case key.Matches(msg, km.Transition):
		return a.openInputForSelected("transition", "Transition ID for ", "")
	case key.Matches(msg, km.Comment):
		return a.openInputForSelected("comment", "Comment for ", "")
	case key.Matches(msg, km.Worklog):
		return a.openInputForSelected("worklog", "Time spent for ", "1h")
	case key.Matches(msg, km.Create):
		a.editing = true
		a.textInput = a.textInput.Show("create", "", "New issue summary:", "")
		return a, nil
	case key.Matches(msg, km.Delete):
		iss := a.dashboard.SelectedIssue()
		if iss == nil || iss.Key == nil {
			return a.flashResult("no issue selected", true)
		}
		k := *iss.Key
		a.confirm = a.confirm.Show("Delete issue", "Permanently delete "+k+"?", func() tea.Msg {
			return SubmitDeleteMsg{IssueKey: k, Confirm: true}
		})
		return a, nil
	}
	dm, cmd := a.dashboard.Update(msg)
	a.dashboard = dm.(Dashboard)
	return a, cmd
}

// assignSelectedToMe submits an edit that sets the assignee to the active
// user. Requires a Jira Cloud accountId (or Server/DC username) — the TUI
// auto-resolves this via /myself at startup, so this only flashes an error
// when /myself failed AND the profile has no account_id configured.
//
// Email-based assignment is intentionally NOT supported: Jira Cloud silently
// ignores it and reports success, which is the worst possible UX.
func (a App) assignSelectedToMe() (tea.Model, tea.Cmd) {
	iss := a.dashboard.SelectedIssue()
	if iss == nil || iss.Key == nil {
		return a.flashResult("no issue selected", true)
	}
	if a.accountID == "" {
		return a.flashResult("no account_id resolved — run `jira auth whoami --save` and retry", true)
	}
	key := *iss.Key
	assignee := map[string]string{"accountId": a.accountID}
	return a, func() tea.Msg {
		return SubmitEditMsg{IssueKey: key, Fields: map[string]any{"assignee": assignee}}
	}
}

func (a App) openInputForSelected(action, promptPrefix, placeholder string) (tea.Model, tea.Cmd) {
	iss := a.dashboard.SelectedIssue()
	if iss == nil || iss.Key == nil {
		return a.flashResult("no issue selected", true)
	}
	a.editing = true
	a.textInput = a.textInput.Show(action, *iss.Key, promptPrefix+*iss.Key+":", placeholder)
	return a, nil
}

func (a App) detailActionKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	km := a.keys
	if a.detail == nil {
		return a, nil
	}
	switch {
	case key.Matches(msg, km.OpenBrowse), key.Matches(msg, km.Open):
		return a.openInBrowser()
	case key.Matches(msg, km.Edit):
		k := ""
		if a.detail.issue.Key != nil {
			k = *a.detail.issue.Key
		}
		a.editing = true
		a.textInput = a.textInput.Show("edit-summary", k, "New summary for "+k+":", "")
		return a, nil
	}
	dm, cmd := a.detail.Update(msg)
	d := dm.(issueDetail)
	a.detail = &d
	return a, cmd
}

func (a App) handleInputSubmitted(msg components.InputSubmitted) (tea.Model, tea.Cmd) {
	a.editing = false
	value := strings.TrimSpace(msg.Value)
	if value == "" {
		return a.flashResult("aborted: empty input", true)
	}
	switch msg.Action {
	case "edit-summary":
		return a, func() tea.Msg {
			return SubmitEditMsg{IssueKey: msg.IssueKey, Fields: map[string]any{"summary": value}}
		}
	case "transition":
		return a, func() tea.Msg {
			return SubmitTransitionMsg{IssueKey: msg.IssueKey, TransitionID: value}
		}
	case "comment":
		// The editor.Editor in the TextInput component built the ADF
		// document end-to-end as the user typed. There is no
		// convert-on-submit step here — we forward the document the
		// component already produced.
		if msg.Document == nil {
			return a.flashResult("comment input missing ADF document", true)
		}
		doc := *msg.Document
		return a, func() tea.Msg {
			return SubmitCommentMsg{IssueKey: msg.IssueKey, Body: doc}
		}
	case "worklog":
		secs, err := jira.ParseDuration(value, 28800)
		if err != nil {
			return a.flashResult("worklog parse failed: "+err.Error(), true)
		}
		return a, func() tea.Msg {
			return SubmitWorklogMsg{IssueKey: msg.IssueKey, TimeSpentSeconds: secs}
		}
	case "create":
		return a, func() tea.Msg {
			return SubmitCreateMsg{Request: &jira.IssueCreateRequest{Summary: value}}
		}
	}
	return a, nil
}

// View implements tea.Model.
func (a App) View() tea.View {
	if a.width == 0 {
		return tea.NewView("")
	}
	header := a.headerView()
	headerBorder := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		BorderForeground(theme.Theme.Dim.GetForeground()).
		Width(a.width)
	header = headerBorder.Render(header)

	a.statusBar.Hint = components.HintContext{
		View:         a.currentViewName(),
		FilterActive: a.current == viewDashboard && a.dashboard.FilterActive(),
		Paused:       a.paused,
		Editing:      a.editing,
	}
	footer := a.statusBar.View().Content

	var body string
	switch a.current {
	case viewDashboard:
		switch a.activeTabID() {
		case "issues":
			body = a.dashboard.View().Content
		default:
			body = lipgloss.Place(a.width, a.bodyH, lipgloss.Center, lipgloss.Center,
				theme.DetailDim.Render("Tab \""+a.activeTabID()+"\" is not yet wired"))
		}
	case viewDetail:
		if a.detail != nil {
			body = a.detail.View().Content
		}
	}
	if a.loadError != "" && a.current == viewDashboard {
		body = theme.StatusErr.Render("⚠  "+a.loadError) + "\n" + body
	}
	if a.loading {
		body = lipgloss.NewStyle().Faint(true).Render(body)
		body = overlayCenter(body, components.RenderOverlay(a.spinner.View()+"  Loading…", 0), a.width, a.bodyH)
	}
	if a.current != viewDetail {
		body = lipgloss.NewStyle().Width(a.width).Height(a.bodyH).MaxHeight(a.bodyH).Render(body)
	}
	if a.textInput.Visible {
		body = overlayCenter(body, a.textInput.View().Content, a.width, a.bodyH)
	} else if a.confirm.Visible {
		body = overlayCenter(body, a.confirm.View().Content, a.width, a.bodyH)
	} else if a.filterOpts.Visible {
		body = overlayCenter(body, a.filterOpts.View().Content, a.width, a.bodyH)
	} else if a.help.Visible {
		body = overlayCenter(body, a.help.View().Content, a.width, a.bodyH)
	}

	out := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
	v := tea.NewView(out)
	v.AltScreen = true
	return v
}

func (a App) headerView() string {
	if a.current == viewDetail && a.detail != nil {
		return a.detail.HeaderContent()
	}
	return a.tabBar()
}

func (a App) tabBar() string {
	active := lipgloss.NewStyle().Bold(true).Foreground(theme.ColorTitleFg).Underline(true).Padding(0, 1)
	inactive := lipgloss.NewStyle().Foreground(theme.ColorHeaderFg).Faint(true).Padding(0, 1)
	parts := make([]string, 0, len(a.tabs))
	for i, t := range a.tabs {
		label := titleCase(t)
		if i == a.activeTab {
			parts = append(parts, active.Render(label))
		} else {
			parts = append(parts, inactive.Render(label))
		}
	}
	return strings.Join(parts, " ")
}

func titleCase(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func (a App) tickCmd() tea.Cmd {
	interval := a.interval
	parent := a.ctx
	return func() tea.Msg {
		t := time.NewTimer(interval)
		defer t.Stop()
		select {
		case <-parent.Done():
			return tickMsg(time.Now())
		case <-t.C:
			return tickMsg(time.Now())
		}
	}
}

func (a App) fetchIssuesCmd() tea.Cmd {
	if a.provider == nil {
		return nil
	}
	provider := a.provider
	parent := a.ctx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 30*time.Second)
		defer cancel()
		issues, err := provider.ListIssues(ctx)
		return issuesLoadedMsg{issues: issues, err: err}
	}
}

func (a App) flashResult(text string, isErr bool) (tea.Model, tea.Cmd) {
	a.statusID++
	id := a.statusID
	if isErr {
		a.statusBar.StatusMsg = theme.StatusErr.Render(text)
	} else {
		a.statusBar.StatusMsg = theme.StatusOK.Render(text)
	}
	delay := 4 * time.Second
	return a, func() tea.Msg {
		t := time.NewTimer(delay)
		defer t.Stop()
		<-t.C
		return clearStatusMsg{id: id}
	}
}

func (a App) currentViewName() string {
	if a.current == viewDetail {
		return "detail"
	}
	switch a.activeTabID() {
	case "epics":
		return "epics"
	case "search":
		return "search"
	case "activity":
		return "activity"
	default:
		return "dashboard"
	}
}

func (a App) activeTabID() string {
	if a.activeTab >= 0 && a.activeTab < len(a.tabs) {
		return a.tabs[a.activeTab]
	}
	return "issues"
}

func tabIndexFromKey(k string) int {
	if len(k) == 1 && k[0] >= '1' && k[0] <= '9' {
		return int(k[0] - '1')
	}
	return -1
}

func (a *App) cycleProfile() {
	if len(a.profiles) <= 1 {
		return
	}
	idx := 0
	for i, p := range a.profiles {
		if p == a.profile {
			idx = i
			break
		}
	}
	a.profile = a.profiles[(idx+1)%len(a.profiles)]
	a.statusBar.Profile = a.profile
}

func (a App) openInBrowser() (tea.Model, tea.Cmd) {
	var k string
	switch a.current {
	case viewDetail:
		if a.detail != nil && a.detail.issue.Key != nil {
			k = *a.detail.issue.Key
		}
	default:
		if iss := a.dashboard.SelectedIssue(); iss != nil && iss.Key != nil {
			k = *iss.Key
		}
	}
	if k == "" {
		return a.flashResult("no issue selected", true)
	}
	url := strings.TrimRight(a.baseURL, "/") + "/browse/" + k
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return a.flashResult("profile has no base URL", true)
	}
	return a, openBrowserCmd(a.ctx, url)
}

// overlayCenter splices `overlay` into the center of `base` and preserves
// the base content to the LEFT and RIGHT of the overlay rectangle.
//
// ANSI-aware: xansi.Truncate / xansi.TruncateLeft slice the base lines at
// cell boundaries without breaking SGR escape sequences. The pdc version
// only kept the prefix (`prefix + overlay`) which clipped the right side
// of any wider base content like our multi-column issue list.
func overlayCenter(base, overlay string, w, h int) string {
	ow := lipgloss.Width(overlay)
	oh := lipgloss.Height(overlay)

	x := (w - ow) / 2
	y := (h - oh) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	lines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")

	for i, ol := range overlayLines {
		row := y + i
		if row >= len(lines) {
			break
		}
		line := lines[row]
		for lipgloss.Width(line) < x+ow {
			line += " "
		}
		prefix := xansi.Truncate(line, x, "")
		suffix := xansi.TruncateLeft(line, x+ow, "")
		lines[row] = prefix + ol + suffix
	}

	var sb strings.Builder
	for i, l := range lines {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(l)
	}
	return sb.String()
}

func (a App) requireMutations() (MutationService, error) {
	if a.mutations == nil {
		return nil, fmt.Errorf("mutation service is not configured")
	}
	return a.mutations, nil
}

// keyOrSelected resolves an issue key, falling back to the dashboard
// cursor's selection. Mutation submitters that arrive without an explicit
// key (typically from tests or simple key paths) get the selected one.
func (a App) keyOrSelected(key string) string {
	if key != "" {
		return key
	}
	if iss := a.dashboard.SelectedIssue(); iss != nil && iss.Key != nil {
		return *iss.Key
	}
	return ""
}

func (a App) submitEditCmd(msg SubmitEditMsg) tea.Cmd {
	key := a.keyOrSelected(msg.IssueKey)
	return func() tea.Msg {
		svc, err := a.requireMutations()
		if err != nil {
			return mutationDoneMsg{kind: "edit", issueKey: key, err: err}
		}
		_, err = svc.UpdateIssue(a.ctx, key, msg.Fields)
		return mutationDoneMsg{kind: "edit", issueKey: key, err: err}
	}
}

func (a App) submitCreateCmd(msg SubmitCreateMsg) tea.Cmd {
	return func() tea.Msg {
		svc, err := a.requireMutations()
		if err != nil {
			return mutationDoneMsg{kind: "create", err: err}
		}
		issue, err := svc.CreateIssue(a.ctx, msg.Request)
		k := ""
		if issue != nil && issue.Key != nil {
			k = *issue.Key
		}
		return mutationDoneMsg{kind: "create", issueKey: k, err: err}
	}
}

func (a App) submitTransitionCmd(msg SubmitTransitionMsg) tea.Cmd {
	key := a.keyOrSelected(msg.IssueKey)
	return func() tea.Msg {
		svc, err := a.requireMutations()
		if err != nil {
			return mutationDoneMsg{kind: "transition", issueKey: key, err: err}
		}
		err = svc.TransitionIssue(a.ctx, key, &jira.TransitionRequest{ID: msg.TransitionID, Fields: msg.Fields})
		return mutationDoneMsg{kind: "transition", issueKey: key, err: err}
	}
}

func (a App) submitCommentCmd(msg SubmitCommentMsg) tea.Cmd {
	key := a.keyOrSelected(msg.IssueKey)
	return func() tea.Msg {
		svc, err := a.requireMutations()
		if err != nil {
			return mutationDoneMsg{kind: "comment", issueKey: key, err: err}
		}
		_, err = svc.AddComment(a.ctx, key, &jira.CommentAddRequest{Body: msg.Body})
		return mutationDoneMsg{kind: "comment", issueKey: key, err: err}
	}
}

func (a App) submitWorklogCmd(msg SubmitWorklogMsg) tea.Cmd {
	key := a.keyOrSelected(msg.IssueKey)
	return func() tea.Msg {
		svc, err := a.requireMutations()
		if err != nil {
			return mutationDoneMsg{kind: "worklog", issueKey: key, err: err}
		}
		_, err = svc.AddWorklog(a.ctx, key, &jira.WorklogAddRequest{
			TimeSpentSeconds: msg.TimeSpentSeconds,
			Started:          msg.Started,
			Comment:          msg.Comment,
		})
		return mutationDoneMsg{kind: "worklog", issueKey: key, err: err}
	}
}

func (a App) submitCloneCmd(msg SubmitCloneMsg) tea.Cmd {
	key := a.keyOrSelected(msg.IssueKey)
	return func() tea.Msg {
		svc, err := a.requireMutations()
		if err != nil {
			return mutationDoneMsg{kind: "clone", issueKey: key, err: err}
		}
		_, err = svc.CloneIssue(a.ctx, key, msg.Request)
		return mutationDoneMsg{kind: "clone", issueKey: key, err: err}
	}
}

func (a App) submitMoveCmd(msg SubmitMoveMsg) tea.Cmd {
	key := a.keyOrSelected(msg.IssueKey)
	return func() tea.Msg {
		svc, err := a.requireMutations()
		if err != nil {
			return mutationDoneMsg{kind: "move", issueKey: key, err: err}
		}
		_, err = svc.MoveIssue(a.ctx, key, msg.Request)
		return mutationDoneMsg{kind: "move", issueKey: key, err: err}
	}
}

func (a App) submitDeleteCmd(msg SubmitDeleteMsg) tea.Cmd {
	key := a.keyOrSelected(msg.IssueKey)
	return func() tea.Msg {
		svc, err := a.requireMutations()
		if err != nil {
			return mutationDoneMsg{kind: "delete", issueKey: key, err: err}
		}
		if !msg.Confirm {
			return mutationDoneMsg{kind: "delete", issueKey: key, err: fmt.Errorf("delete requires confirmation")}
		}
		err = svc.DeleteIssue(a.ctx, key)
		return mutationDoneMsg{kind: "delete", issueKey: key, err: err}
	}
}

// --- Public test/inspection accessors (preserved for integration tests) ---

// ActiveTab returns the id of the currently active dashboard tab.
func (a App) ActiveTab() string { return a.activeTabID() }

// ActiveProfile returns the currently selected profile name.
func (a App) ActiveProfile() string { return a.profile }

// HelpVisible reports whether the help overlay is currently shown.
func (a App) HelpVisible() bool { return a.help.Visible }

// Cursor exposes the list cursor for legacy tests; returns 0 in the new
// design (cursor is internal to the issuesList view).
func (a App) Cursor() int { return 0 }

// Filter returns the active filter string from the issue list.
func (a App) Filter() string { return a.dashboard.issues.FilterValue() }

// SetFilter pre-populates the issue list filter; used by tests.
func (a *App) SetFilter(value string) {
	a.dashboard.issues.filterInput.SetValue(value)
}

// LastAction returns the current view name as a coarse intent label
// (kept for legacy callers; the prior ad-hoc string field is gone).
func (a App) LastAction() string { return a.currentViewName() }
