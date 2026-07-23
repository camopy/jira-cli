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
is specified in the embedded guide (`jira agent guide core-contract`) — do
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

## Write ownership

The output path is command → `cmdutil` → `cli` → Cobra's `io.Writer`.
Commands use `RunE` and return every `cmdutil.Write*Envelope` result.
`cmdutil` chooses the machine or human renderer and passes
`cmd.OutOrStdout()`/`cmd.ErrOrStderr()` down. `cli` owns rendering and tracks
the first destination failure across Clog's void finalisers. Never write
command output through `os.Stdout` or `os.Stderr`.

Dynamic completion candidates use the same injected root writer and report a
failed candidate write after Clib dispatch. Clib's upstream
`--print-completion` action remains the sole deliberate process-stdout
boundary.

Destination failures are `output_write_failed`, `type=io`, exit 8 and
non-retryable. A Jira mutation may already have completed, so never retry it
solely because its result could not be written. If a command error and its
error renderer both fail, root keeps the command error primary and joins the
write failure as secondary context; it does not try the failed stream again.

## Envelope data conventions (contract v2)

*   **Issue identity is always an object.** Any issue-scoped `data.issue`
    (and link create's `inward_issue`/`outward_issue`) is a
    `cmdutil.IssueRef` — `{id?, key, self?}` — never a bare key string, and
    never an ad-hoc `data.key`. `IssueRef.String()` renders the key for
    human/plain output; richer objects (`issue view`, create's POST echo)
    satisfy the same minimum by carrying `key` at the same place.
*   **Every mutation carries `dry_run` on both paths** — `true` on preview,
    `false` on the live write — including no-profile and `--no-readback`
    variants. A live path never drops fields its dry-run counterpart
    carries (identity included).
*   **A new or changed `data` field must land in `outputSchemas()`
    (`internal/cli/schema/schema.go`) in the same change** — conditional
    fields too, with a description saying when they appear. The
    conformance guardrail
    (`tests/contract/schema_conformance_test.go`) validates emitted
    envelopes against the published schemas and fails on an emitted
    field the schema does not declare; extend its case table when a new
    op is hermetically producible (dry-run, local-only, or a simple
    stub). The identity invariant itself is pinned by
    `tests/contract/issue_ref_shape_test.go`.
*   **Typed Output structs are the law, not a direction.** Every operation
    has an exported Output struct in `internal/envelope`, registered beside
    its definition; the published schema derives from the struct
    (`envelope.SchemaOf`), prose rides the registration's doc overrides,
    and builders emit the struct on every path. Enforced in layers: the
    `untypedEnvelopeData` ruleguard rule bans map literals at envelope
    write sites; `tests/guardrails/typed_outputs_test.go` requires a
    registration for every verb-registry operation AND every op string a
    source scan finds at a `cmdutil.Write*Envelope` call; the conformance
    contract test validates emitted envelopes against the published
    schemas. Genuinely shapeless payloads register `envelope.Dynamic` with
    a reason; a builder that must keep map emission (issue.list's
    `--detail` polymorphism) keeps its registered struct as the schema
    template and is a reviewed, commented exception.

*   Envelopes render through the clog printer path (`internal/cli/json.go`):
    `JSONFlat` for machine modes, `JSONPretty` retinted to the active theme
    for human-mode JSON. The shared command-local write tracker captures
    broken-pipe/quota failures and nil short writes that Clog finalisers would
    otherwise hide — the reason raw encoders are banned.
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
*   **A dry-run speaks in preview tense on every human surface** — it must
    never claim the mutation happened. Completion lines take the verb
    registry's conditional form (`VerbFor(op).Conditionalf()`, selected
    automatically when the rendered payload carries `dry_run=true`); a
    dry-run path that still spins or fans out (local pipeline validation,
    `--validate-remote` reads) uses the preview variants
    (`cmdutil.SpinPreview`, `cmdutil.FanOutKeysProgressPreview`) so
    spinner labels, the fanout footer, and the `--debug` lifecycles say
    "previewing issue edit", never "editing issue". The one exception: a
    dry-run step that performs a genuine read keeps `Spin` with the read
    op (the transition preview spins `issue.transitions`), because that
    read really happens. `OperationVerb.Preview()` documents which
    registry entries compose cleanly before a new op adopts it.

## Errors and exit codes

All user-visible failures map through `internal/cli/errors.go`:

*   `MapError` chains `errors.As` adapters for every typed error (CLIInput,
    Prompt, context cancel/deadline, Credential, `jira.APIError`, issuekey,
    board candidates, ambiguous-user). The terminal fallback classifies any
    untyped error as validation (exit 3) without inspecting its message —
    every non-validation class (auth/not_found/rate_limit/server) is typed
    at its source via an adapter or `errtax.Coded`. Never match `err.Error()`
    substrings to pick a class; type the error where it is built.
*   **Never synthesize a fake HTTP status on a hand-built `APIError` to borrow
    a transport class.** A miss the CLI computes locally — a `--type` value
    absent from the fetched issuetypes list, a name not in a cached set — is
    **validation (exit 3)**, typed at its source with a dedicated error
    (`jira.IssueTypeUnknownError`), not a `&APIError{StatusCode: 404}` that
    would drag it into not-found (exit 2). Real 404s come only from the
    transport (`client.go`). Guarded by
    `TestNoSyntheticAPIErrorStatusInServices` in `tests/guardrails`.
*   Stable snake_case codes with a `hint` that adds action beyond
    `message`; Jira HTTP statuses map to `jira_*` codes and hints in
    lockstep tables guarded by a test.
*   **Writing a hint.** A hint is the next action in the plain words a person
    would use — not a recap of `message`. Say what to do, not what failed. It
    serves both a human (it renders as clog's 💡 line under the error on
    stderr) and an agent (the envelope `hint`), so avoid envelope-only jargon
    like `candidates[]`; when the fix needs a value the caller doesn't have
    yet, name the command that surfaces it (`jira user search`,
    `jira boards list`). Every hint is static — defined in the
    `internal/errtax` registry keyed by code — and every code carries a
    non-empty one (the taxonomy guard enforces it). A hint never interpolates
    a runtime value: if a specific matters (the offending value, a "did you
    mean" suggestion), it belongs in the error `message` or a structured
    envelope field (`field`, `suggestions[]`), never baked into the hint. If
    two error types want different hints, give them different codes rather
    than one code with per-instance text. A **catch-all or multi-resource
    code** must keep its hint resource-neutral: `jira_not_found` maps every
    Jira 404 (issue, board, attachment, user, comment, link, project), so its
    hint says "the identifier", never "the issue key" — naming one resource
    misleads on all the others; put the resource specifics in the `message`.
    Both bans are machine-enforced in `internal/errtax` —
    `TestHintsAvoidEnvelopeJargon` (jargon) and
    `TestSharedNotFoundHintIsResourceNeutral` (the shared-404 hint). Watch the
    reuse trap in the other direction too: `arg_value_invalid`'s hint sends the
    caller to `--help`, so only use it where `--help` actually lists the
    accepted values. A lookup whose valid values live elsewhere — a saved-query
    name in `queries_path`, an issue type on a project's create screen — needs
    its own code (`saved_query_unknown`, `issue_type_unknown`) with a hint that
    points at the real source, and offers the values via `suggestions[]`.
*   Exit codes are a published contract: `0` ok, `1` auth, `2` not-found,
    `3` validation, `4` rate-limit, `5` server, `6` canceled, `7` timeout,
    `8` local output failure. `main.go` owns process exit and maps
    `ErrCompletionHandled` → 0.

## Keyed results (multi-key commands)

Multi-key commands accept lists/ranges (`PROJ-1..10`), expand and chunk
transparently, and return ordered `data.results[]` with per-key errors that
never discard successful keys (`cmdutil/keyed_results.go`; failures sort by
exit severity, the top error drives the process exit code). Follow this
pattern for any new multi-key verb.

## Enforced vs advisory

CI fails on: raw pflag declarations outside cmdutil (`rawPflagDeclaration`),
unwrapped blocking Jira calls (`unwrappedBlockingCall`), `log`/`log/slog`
imports (depguard), direct process-stream references in command packages,
discarded output-helper results and `Run` handlers that cannot return errors.
The last three are path-aware AST guardrails rather than ruleguard rules;
`errcheck` separately enforces returned-error handling. Full lint tables are
in [style.md](style.md).
