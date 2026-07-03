---
description: >
  The output contract and its machinery: mode resolution, the JSON
  envelope, the clog printer path, error mapping and exit codes, streams,
  and the keyed-results pattern.
paths:
  - "internal/cli/**/*.go"
  - "cmd/jira/*.go"
---

# Output & Logging

The authoritative implementation rules for the output machinery. Sibling
files link here: [commands.md](commands.md) (command-side usage),
[style.md](style.md) (the lint bans backing this file),
[project.md](project.md) (architecture map). The user-facing contract itself
is specified in the embedded guide (`jira agent guide core_contract`) — do
not duplicate it here; change it there in the same PR.

## Libraries

All terminal output and logging goes through `github.com/gechr/clog` —
structured logging, spinners/progress, styled JSON/TOML printing. TTY
detection comes from `x/terminal`; ANSI strip/width/truncate from `x/ansi`;
flag metadata, themed help, and completions from `clib`, only through
`internal/cli/cmdutil`. Do not hand-roll any of those.

## Mode resolution

One selector: `--output` (`auto` | `human` | `json` | `compact`); the legacy
boolean flags (`--json`, `--plain`, `--raw`) are gone — never reintroduce
one. `Detect` (`internal/cli/detector.go`) resolves `auto` once: detected
agent env → `compact`, non-TTY stdout → `json`, TTY → human.

## The envelope

`{ok, meta{command, timestamp, request_id, exit_code?, pagination?}, data,
errors[], warnings[]}` — written by `cli.WriteEnvelope` only.

IMPORTANT: JSON envelopes must be byte-clean. Machine output goes to stdout
through `cli.WriteEnvelope`; never a raw `json.Encoder`. In `json`/`compact`
modes nothing else may write to stdout.

*   Envelopes render through the clog printer path (`internal/cli/json.go`):
    `JSONFlat` for machine modes, `JSONPretty` retinted to the active theme
    for human-mode JSON. The custom `errWriter` wrapper exists to capture
    broken-pipe/quota write failures that would otherwise be swallowed —
    the reason raw encoders are banned.
*   Error envelopes go to **stdout** (success and failure alike), even under
    `compact`; human diagnostics go to **stderr** through clog.
*   Warnings carry a taxonomy (`unknown_adf_node`, `lossy_adf_conversion`,
    `rate_limit_near`, …) — reuse existing codes before minting new ones.

## Human output

*   Root wiring (`internal/cli/root/root.go`): clog is pinned to **stderr**
    with `SetEnvPrefix("JIRA")` and color from `--color`; a context logger
    is seeded so command code uses `clog.Ctx(ctx)`.
*   Per-command human rendering goes through the plain renderer registered
    in `internal/cli/registry.go` (`plain_*.go`), built on fresh clog
    loggers with `SetOmitEmpty(true)` — see the `newPlainLogger` comment for
    why OmitEmpty and not OmitZero.
*   Success lines take their verb from `internal/cli/verbs.go`
    (`VerbFor("issue.list").PastPlural()`); register a verb with every new
    command.
*   Blocking Jira calls render a spinner/progress (`unwrappedBlockingCall`
    ruleguard rule); multi-key mutations use the fanout bar
    (`cmdutil/fanout.go`), suppressed off-TTY via `NonTTYSilent` and under
    `--debug`.

## Errors and exit codes

All user-visible failures map through `internal/cli/errors.go`:

*   `MapError` chains `errors.As` adapters for every typed error (CLIInput,
    Prompt, context cancel/deadline, Credential, `jira.APIError`, issuekey,
    board candidates, ambiguous-user). The substring classifier at the
    bottom is legacy fallback only — new errors get a typed adapter, never a
    substring match.
*   Stable snake_case codes with a `hint` that adds action beyond
    `message`; Jira HTTP statuses map to `jira_*` codes and hints in
    lockstep tables guarded by a test.
*   Exit codes are a published contract: `0` ok, `1` auth, `2` not-found,
    `3` validation, `4` rate-limit, `5` server, `6` canceled, `7` timeout.
    `main.go` owns process exit and maps `ErrCompletionHandled` → 0.

## Keyed results (multi-key commands)

Multi-key commands accept lists/ranges (`PROJ-1..10`), expand and chunk
transparently, and return ordered `data.results[]` with per-key errors that
never discard successful keys (`cmdutil/keyed_results.go`; failures sort by
exit severity, the top error drives the process exit code). Follow this
pattern for any new multi-key verb.

## Enforced vs advisory

CI fails on: raw pflag declarations outside cmdutil (`rawPflagDeclaration`),
unwrapped blocking Jira calls (`unwrappedBlockingCall`), and `log`/`log/slog`
imports (depguard) — full tables in [style.md](style.md). Envelope
byte-cleanliness and stream discipline are contract-test-enforced; the rest
of this file is convention — reviewed, not machine-guaranteed.
