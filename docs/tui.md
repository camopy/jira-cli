---
title: The TUI
description: The persistent full-screen dashboard — tabs, lenses, triage verbs, bulk actions, the JQL workbench, and its config.
icon: material/view-dashboard-outline
---

# :desktop_computer: The TUI

!!! warning "Alpha"
    The dashboard is in **alpha**: actively developed, keybindings and
    configuration may still change between releases, and rough edges are
    expected. The headless CLI commands remain the stable surface for
    scripts and agents. Bug reports are very welcome.

`jira tui` (or `jira -i`) opens a persistent, full-screen dashboard for
working a Jira queue without leaving the terminal: tabbed views over JQL
queries, an always-visible issue preview, single-key triage verbs with
optimistic updates, multi-select bulk actions, and a JQL search
workbench. It is built on Bubble Tea.

```sh
jira tui
jira -i
```

Everything below is configurable in `config.toml` under `[tui]` — see
[Configuration](#configuration) for the full key reference and a complete
example, and [Configuration docs](config.md) for where the file lives.

## Layout

```text
 Issues (6)   Team (12)   Search   Settings          jira v1.4.0
 ───────────────────────────────────────────────────────────────
 [Mine]  Sprint   Updated   Reported    assignee = currentUser()…
 2 selected · t/a/c act on all
   TYPE  KEY      SUMMARY                    STATUS   ASSIGNEE  AGE
 ✓ 🐛    JCT-12   Fix login redirect         To Do    Matt       2h
 ● ⭐    JCT-31   Add SSO support            In Prog  Ann        5m
   ...                                                │ preview
 ───────────────────────────────────────────────────────────────
 profile · project · board     enter open  t transition  a assign …
```

From top to bottom:

| Region | What it does |
|--------|--------------|
| Tab bar | One tab per section, with live result counts. `tab`/`shift+tab` or a mouse click switches. The brand/version sits right-aligned |
| Header row | Per-section: the lens chips on Issues, the JQL box on Search, the query title on configured sections |
| Status line | Transient feedback: filter input, action toasts, errors, loading spinner, facet chip, selection count, pagination state |
| List | The issue rows. The leading marker column shows `✓` (selected), a green `●` (new since last refresh) or a yellow `●` (updated) |
| Preview | The selected issue's detail beside or below the list; scrolls independently |
| Footer | Active profile/project/board plus a live hint line of the current view's key bindings |

The TUI is fully mouse-aware: click a tab to switch, a lens chip to
change query, a row to select it (click again to open it), the detail
sub-tab pills to switch, and scroll whichever pane is under the pointer.

## Tabs and sections

The built-in sections: **Issues** (the triage home), **Epics** (a strip of
open epics — `[`/`]` cycles, the list shows the active epic's children),
**Search** (the JQL workbench), **Board** (your profile's `default_board`
as its own tab — it lights up once `jira cache boards` has run) and
**Settings** (resolved config, always last).
You can add your own sections — each one a saved JQL query
with its own tab and result count:

```toml
[[tui.sections]]
title = "Team board"
jql = "project = JCT AND statusCategory != Done ORDER BY updated DESC"

[[tui.sections]]
title = "Needs review"
jql = "status = 'In Review' ORDER BY updated DESC"
```

`tui.tabs` controls which tabs show and their order; `tui.default_tab`
names the landing view:

```toml
[tui]
tabs = ["issues", "Team board", "search"]
default_tab = "Team board"
```

The resolution rules:

*   A name matches a built-in section ID first (`issues`, `epics`,
    `search`, `board`, `settings`), then a configured section's title
    (case-insensitive) — so a query titled "Search" can never hijack
    `default_tab = "search"`.
*   Configured sections always show; defining one is the opt-in and
    deleting it is how you hide it. Naming one in `tabs` only positions
    it explicitly; the rest slot in after Issues.
*   Settings always rides last unless `tabs` placed it somewhere.
*   `default_tab` is honored only when that tab is visible.

## Lenses

Lenses are the quick-filter chips on the Issues tab — one keypress flips
the whole queue to a different query. The built-ins are **Mine** (your
open work, the landing query), **Updated** (last 7 days) and
**Reported**. Cycle with `]` / `[` or click a chip; the active lens's
JQL shows faint beside the chips.

Replace them with your own:

```toml
[[tui.lenses]]
title = "Team"
jql = "project = JCT AND statusCategory != Done ORDER BY updated DESC"

[[tui.lenses]]
title = "Blocked"
jql = "status = Blocked ORDER BY updated DESC"

[tui]
default_lens = "Team"
```

*   Configured lenses **replace** the built-in set; omit `[[tui.lenses]]`
    entirely to keep the defaults.
*   `default_lens` names the landing lens by title (case-insensitive);
    absent or unmatched lands on the first lens.
*   Entries missing a `title` or `jql` are skipped.
*   The lens set is also the saved-query list on the Search tab
    (`ctrl+p` and `]` cycling), so one config drives both.

!!! tip "Lenses vs sections"
    A **section** is a separate tab with its own result count — use it
    for a queue you glance at. A **lens** re-points the Issues tab — use
    it for queries you flip between while triaging. They share the same
    `title`/`jql` shape.

## Filtering the list

Three narrowing tools compose on every list view:

*   **`/` — text filter.** Live fuzzy narrowing over key and summary;
    `enter` keeps it, `esc` clears it.
*   **`f` — facets.** A type-to-filter picker over the distinct
    statuses, assignees and labels in the loaded set (with counts,
    including an "Unassigned" bucket). Pick one to narrow to it; reopen
    and pick the `✕ clear` entry to drop it. The active facet shows as a
    chip in the status line and composes with the text filter.
*   **Change markers.** A background refresh marks rows it added (green
    `●`) or changed (yellow `●`). Navigating onto a row clears its
    marker — deliberate viewing, not refresh churn, is what counts.

## Reading an issue

The preview pane always shows the selected row. `enter` opens the full
detail view: description and comments rendered as themed markdown, with
**Overview** and **Comments** sub-tabs (`tab`/`shift+tab` or click the
pills). It scrolls with `↑`/`↓`/the wheel, `o` opens the issue in the
browser, and `esc`/`enter` returns to the list. Issue keys render as
terminal hyperlinks where supported.

### Recent issues — the jumplist

`ctrl+o` opens a picker over every issue you've opened this session,
most recent first, across all tabs. Type to filter, `enter` to jump:
issues still in the current list select-and-open in place; anything else
opens directly by key. Capped at the 50 most recent.

## Acting on issues

Single-key verbs drive the triage loop. Where the change is visible in
the row (status, summary, assignee) it applies **optimistically** — the
row updates immediately, the write reconciles in the background, and a
failure rolls the row back with an error toast.

| Key | Action | Notes |
|-----|--------|-------|
| `n` | New issue | Two-field overlay: summary line + Markdown description (`tab` switches fields, `ctrl+s` submits). Lands in `default_project` (or the selected issue's project) with `default_issue_type` (Task when unset) |
| `t` | Transition | Type-to-filter picker over the workflow transitions Jira allows for this issue |
| `a` | Assign | Type a name or email — resolved against Jira; `none`/`unassigned` (or empty) clears |
| `A` | Assign to me | One keypress, no prompt |
| `c` | Comment | In-modal textarea (`ctrl+s` submits, `enter` is a newline); with `$JIRA_EDITOR`/`$EDITOR` set, `ctrl+e` continues the draft in the external editor. Markdown converts to ADF |
| `e` | Edit summary | Prefilled with the current summary |
| `l` | Labels | Prefilled with the current labels, comma-separated — what you submit replaces the whole list, so an emptied field clears them |
| `w` | Log work | Accepts `2h`, `30m`, `1d` etc. (`workday_seconds` scales `d`) |
| `o` | Open in browser | |
| `y` / `Y` | Copy issue key / URL | Via OSC 52, so it works over SSH |

Modal prompts share one input substrate: real cursor movement, word-wise
editing and bracketed paste everywhere. `esc` cancels — and when you have
typed something, it asks `discard input? (y)es (n)o` first, so a stray
`esc` can never eat a half-written comment.

## Bulk actions

Select multiple issues and the verbs escalate to the whole selection:

| Key | Selection |
|-----|-----------|
| `space` | Toggle the current row (and set the range anchor) |
| `x` | Select from the anchor through the cursor |
| `X` | Invert the visible selection |
| `ctrl+a` | Select all visible rows — or clear them all when everything is already selected |

Selections survive filter changes (mark, narrow, mark more, act on the
union) and a refresh prunes marks for issues that left the result set.

With a selection active, `t`, `a` and `c` become **bulk transition**,
**bulk assign** and **bulk comment**. Every bulk write first shows a
confirmation that restates the verb, the count and the target keys —

```text
Transition 3 issues (JCT-1, JCT-2, JCT-7) to "Done"? (y/N)
```

— defaulting to **No**; any key other than `y` cancels and keeps the
selection. The batch then runs through a bounded worker pool, and
per-issue failures are reported by key instead of aborting the rest:

```text
2 → Done · failed: JCT-7
```

Bulk transition picks from the first selected issue's transitions and
applies by *name*, resolving each issue's own transition id at apply
time — an issue whose workflow doesn't offer that name fails
individually. Bulk assign resolves the user query once before any write.

## The Search tab

A free-form JQL workbench. `enter` (or `2`, or a click on the box)
starts editing; `enter` validates the query through Jira's parser —
syntax errors surface inline instead of as a failed search — and runs
it. Results get the full shared toolkit: filter, facets, verbs, bulk
mode, preview.

*   **Autocomplete.** The editor completes the token you're typing,
    context-aware, from your Jira instance's own JQL metadata (the same
    API behind the web UI's query bar): field names — including custom
    fields — at a field position, that field's operators after it, and
    its live values after the operator (after `status =` your actual
    statuses ghost in; a multi-word value like `In Progress` completes
    quoted when nothing — or an opening quote — has been typed yet, and
    after `in`/`not in` values complete parenthesised). JQL functions
    and `AND`/`OR`/`ORDER BY` complete in their places, and your saved
    queries complete as whole lines at the start. Everyday names
    (`project`, `assignee`, `currentUser()`, …) rank ahead of plugin
    custom fields, and an empty editor offers only the saved queries —
    token suggestions start once you type. The completion shows faint
    as you type; `tab` accepts it. Offline or before the metadata
    loads, it falls back to the saved queries and common openings.
*   **Presets.** `ctrl+p` opens a type-to-filter dropdown over the saved
    queries — matching on the name *or* the JQL text — and `enter`
    commits and runs the pick. `]` / `[` cycle through them directly.
*   Scroll to the bottom of a large result set and the next page fetches
    automatically.

## The preview pane

| Key | Action |
|-----|--------|
| `p` | Cycle the dock: right → left → bottom → hidden |
| `+` / `-` | Grow / shrink the preview's share of the split (5% steps) |
| `z` | Zoom the list to the full body (toggle; the split is restored on the next press) |
| `ctrl+u` / `ctrl+d` | Scroll the preview while it's visible |

Configure the resting state:

```toml
[tui]
preview = "right"      # right | left | bottom | hidden | auto
preview_size = 40      # percent of the split, 20–80
```

`auto` puts it right on wide terminals and bottom on narrow ones, and
side docks hide automatically when the window is too narrow for a
usable split.

## Refresh

Every section refetches on a shared interval (`tui.refresh_interval`
seconds, default 60; `0` disables). `r` refreshes the current view now;
`R` pauses/resumes the automatic cycle. Regaining terminal focus also
triggers a refresh, so the dashboard is current when you switch back to
it. Background reloads never interrupt: a refresh is skipped while
you're typing in a filter or modal, and a refetch re-finds your selected
issue by key even if the ordering changed.

## Keybindings

Every binding is rebindable under `[tui.keys]`: the value is a list of
keys, the first one becomes the help label, and removing an entry
restores the default. Unknown action names surface as a footer error
rather than breaking the dashboard.

```toml
[tui.keys]
transition = ["T"]
jumplist = ["ctrl+r"]
quit = ["q"]
```

| Action name | Default | Action name | Default |
|-------------|---------|-------------|---------|
| `up` / `down` | `↑`/`k`, `↓`/`j` | `transition` | `t` |
| `top` / `bottom` | `g`/`home`, `G`/`end` | `comment` | `c` |
| `page_up` / `page_down` | `ctrl+u`, `ctrl+d` | `assign` | `a` |
| `next_section` / `prev_section` | `tab`, `shift+tab` | `assign_me` | `A` |
| `open` | `enter` | `worklog` | `w` |
| `back` | `esc` | `edit` | `e` |
| `refresh` | `r` | `open_browse` | `o` |
| `toggle_pause` | `R` | `copy_key` / `copy_url` | `y`, `Y` |
| `filter` | `/` | `select` | `space` |
| `facet` | `f` | `select_all` | `ctrl+a` |
| `jumplist` | `ctrl+o` | `select_invert` | `X` |
| `search` | `2` | `select_range` | `x` |
| `presets` | `ctrl+p` | `toggle_preview` | `p` |
| `next_lens` / `prev_lens` | `]`, `[` | `grow_preview` / `shrink_preview` | `+`/`=`, `-` |
| `help` | `?` | `zoom` | `z` |
| `quit` | `q`, `ctrl+c` | | |

`?` opens a help sheet with the current bindings, grouped by purpose —
the authoritative in-app reference after any rebinding.

## Settings and hot-reload

The **Settings** tab shows the resolved configuration and the file it
came from. The TUI checks that file's mtime on the refresh heartbeat
and hot-applies edits to `[tui]` keys — lenses, sections, tabs,
keybindings, preview, refresh interval — without restarting. A reload
that changes a lens's JQL also refetches, so the rows always match the
header. Press `r` on the Settings tab to reload immediately (the only
path when auto-refresh is disabled).

Theme (`[theme]`) and credentials changes still need a restart.

## Theming

The dashboard follows `[theme]` from your config (see
[Configuration](config.md#themes)) — including the markdown rendering in
the detail view, which derives its glamour style from the same palette
so issue bodies match the chrome. ANSI palette themes render with your
terminal's own colours.

## Configuration

All keys live under `[tui]`:

| Key | Purpose | Default |
|-----|---------|---------|
| `refresh_interval` | Auto-refresh cadence in seconds; `0` disables | `60` |
| `tabs` | Which tabs show, in order (built-in IDs or section titles) | built-ins |
| `default_tab` | The landing tab (must also be visible) | `issues` |
| `sections` | `[[tui.sections]]` — your own JQL tabs (`title` + `jql`) | none |
| `lenses` | `[[tui.lenses]]` — replace the Issues quick-filters (`title` + `jql`) | built-ins |
| `default_lens` | Landing lens by title, case-insensitive | first lens |
| `preview` | Preview dock: `right`, `left`, `bottom`, `hidden`, `auto` | `auto` |
| `preview_size` | Preview share of the split, percent (20–80) | `50` |
| `keys` | `[tui.keys]` — rebind any action by name | defaults above |

A complete example:

```toml
[tui]
refresh_interval = 30
default_tab = "issues"
default_lens = "Team"
preview = "right"
preview_size = 40

[[tui.lenses]]
title = "Team"
jql = "project = JCT AND statusCategory != Done ORDER BY updated DESC"

[[tui.lenses]]
title = "Blocked"
jql = "status = Blocked ORDER BY updated DESC"

[[tui.sections]]
title = "Needs review"
jql = "status = 'In Review' ORDER BY updated DESC"

[tui.keys]
transition = ["T"]
quit = ["q", "ctrl+c"]
```

## See also

*   [Configuration](config.md): where `config.toml` lives, profiles, themes.
*   [JQL](jql.md): the query language behind lenses, sections and search.
*   [`search saved`](search.md#search-saved): the CLI-side saved queries (`.jql` files) — separate from TUI lenses today.
*   [Worklog](worklog.md): the duration formats `w` accepts.
