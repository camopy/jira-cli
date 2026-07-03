---
description: >
  The cobra command pattern for jira-cli: NewCommand wiring, cmdutil.Add*
  flag metadata, gates (dry-run/read-only/ADF mode), envelope output, help
  text house style, and command-level review points and gotchas.
paths:
  - "internal/cli/**/*.go"
---

# Commands

The pattern for every cobra command. For the output contract see
[output.md](output.md); for config resolution see [config.md](config.md);
for the architecture map see [project.md](project.md).

## Pattern

```go
func NewCommand() *cobra.Command {
    cmd := &cobra.Command{
        Use:     "add ISSUE_KEY... EPIC_KEY",
        Short:   "Add an issue to an epic",
        Long:    "Assign one or more issues to an epic. ...",  // see Help text
        Example: `$ jira epic add PROJ-123 PROJ-100`,
        GroupID: "resources",                 // agent|configuration|resources|dashboard
        Args:    cobra.MinimumNArgs(2),       // arity via Args validator, never len(args) in RunE
        RunE: func(cmd *cobra.Command, args []string) error {
            client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
            // ok=false → no profile: degrade helpfully (empty result + intended
            // query shape) where the command can, instead of hard-failing.
            ...
            return cmdutil.WriteEnvelope(cmd, "epic.add", data)
        },
    }
    cmdutil.AddDryRunFlag(cmd)                // semantic bundlers first
    return cmd
}
```

*   Success output goes through `cmdutil.WriteEnvelope(cmd, "domain.verb", data)`:
    machine modes emit the envelope; human mode routes to the plain renderer
    registered in `internal/cli/registry.go`, with the success line's verb from
    `internal/cli/verbs.go` (`VerbFor("issue.list").PastPlural()` → "Listed
    issues"). A new command registers its verb and (if it has rich human output)
    its renderer.
*   Declare arity with a cobra `Args` validator (`cobra.NoArgs`,
    `cobra.MinimumNArgs(2)`, …); cobra rejects bad arity before `RunE`.
*   Blocking Jira calls run behind a clog spinner/progress — the
    `unwrappedBlockingCall` ruleguard rule fails CI otherwise. Multi-key work
    goes through the fanout executor (`cmdutil/fanout.go`) and returns
    keyed results (see [output.md](output.md)).

## Flags: metadata is mandatory

Raw `pflag` declarations are banned outside `cmdutil` (ruleguard
`rawPflagDeclaration`). Declare flags with the `cmdutil.Add*` helpers
(`AddStringVar`, `AddBoolP`, …), which register the flag and attach its
`clib.FlagExtra` in one call:

```go
cmdutil.AddStringVar(cmd.Flags(), &opts.Query, "query", "",
    "Filter results by query", clib.FlagExtra{
        Group:       "Filters",           // must exist in help.go flagGroupRank
        Terse:       "query filter",      // short label for completions
        Placeholder: "text",
        Complete:    "predictor=query",   // dynamic completion (completion.go)
    })
```

*   Prefer the semantic bundlers for recurring clusters: `AddDryRunFlag`,
    `ExtendPaginationFlags`, `AddIssueColumnFlags`, …; reach for generic
    helpers only for one-offs.
*   Enum flags carry `Enum`/`EnumTerse`/`EnumDefault` so help renders the
    values and completion offers them.
*   A `Group` not listed in `flagGroupRank` (`cmdutil/help.go`) renders near
    the bottom as "obviously unranked" — add new groups to the rank map
    deliberately.
*   A new dynamic predictor must be wired in `internal/cli/completion.go`;
    a guard test asserts every declared predictor is handled.

## Gates

Resolve gates through `cmdutil`, never by reading env/config ad hoc:

*   **Dry-run** — `--dry-run` is per-command (via `AddDryRunFlag`), local-only,
    and never contacts Jira. `dryRunRequested` (gates.go) tolerates the flag's
    absence and fails SAFE (a guard that cannot read its flag assumes dry-run
    is ON). It is also threaded into the Jira client so the service layer
    refuses mutating requests as a safety net.
*   **Read-only** — `cmdutil.ReadOnlyEnabled`: `JIRA_READ_ONLY` env wins on the
    OFF→ON direction only; enforcement lives at the HTTP transport (see
    [security.md](security.md)).
*   **ADF mode** — `cmdutil.ADFModeFor(cmd, mutation)`: flags > env
    (`JIRA_ADF_STRICT`) > path default; mutations default strict, reads
    best-effort; conflicting inputs resolve to the safe default.
*   **Headless** — mutations require `--no-input` in agent/non-TTY context;
    destructive commands additionally require `--force`. Do not weaken either.
*   **Retry budget** — `cmdutil.MaxRetryWaitFor`: `--max-retry-wait` (root
    persistent flag, read from root's flagset because inherited flags may not
    be merged yet) > `JIRA_MAX_RETRY_WAIT` > 30s default; always capped by the
    `--timeout` context deadline.

## Help text (Short / Long / Example)

Help renders through clib (`cmdutil.NewHelpRenderer`); every command defines
`Short`, `Long`, and `Example`. House style, by field:

*   **`Long` → double-quoted string**, concatenated with `+` across source
    lines at word boundaries. clib wraps at render time — do not hand-wrap
    prose; use `\n\n` for real paragraph breaks. Backtick special tokens
    directly in the string: sibling commands as the full tree
    (`` `jira epic list` ``), the command's own flags bare (`` `--dry-run` ``),
    env vars and paths (`` `JIRA_READ_ONLY` ``). Agent-relevant behavior
    (what dry-run does, what happens with no profile) belongs in `Long` —
    agents read it via the schema.
*   **`Example` → raw backtick string** of literal shell lines, each command
    `$`-prefixed, with `#` comment lines introducing variants:

    ```go
    Example: `$ jira epic add PROJ-123 PROJ-100

    # Preview the assignment without contacting Jira
    $ jira epic add PROJ-123 PROJ-100 --dry-run`,
    ```

*   **Flag usage strings stay plain prose** — no backticks. Placeholders and
    enum values come from `FlagExtra` metadata (`Placeholder`, `Enum`), never
    repeated inside the usage text: clib already renders
    `--query <text>  [table, json, yaml]`.

## Review points (not lint-enforced)

*   Verb registered in `verbs.go`; renderer in `registry.go` for rich output.
*   Guide section + schema updated in the same PR as any behavior change.
*   Contract test covers the envelope; guardrail test for any new invariant.
*   Dry-run checked before expensive work and before early returns.
*   New flag group added to `flagGroupRank`.

## Gotchas

*   **`completion` bypasses `PersistentPreRunE`** (root.go) — completion code
    paths cannot rely on config/logging setup having run.
*   **Completion never writes to stdout** except the raw candidates; candidates
    are sanitized against tab/newline injection before emission.
*   **Stop spinners before anything interactive** (editor, huh forms).
*   **Bare `jira`** is contract-aware: TTY renders help, non-TTY/agent emits
    the JSON schema (root.go) — don't add root-level behavior that breaks this.
*   **`jira issue edit KEY` with no field flags** opens `$EDITOR`; agent/non-TTY
    context refuses with exit 3 and a remediation hint — preserve that guard
    when touching the editor flow.
