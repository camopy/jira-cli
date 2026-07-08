# Release Notes


## [0.10.1](https://github.com/matcra587/jira-cli/releases/tag/v0.10.1) — 2026-07-08

### Fixed

- Locally-detected command errors now carry typed classifications instead of being guessed from their message text, which fixes five wrong exit codes. `auth login` with a `--backend` that differs from the profile's stored backend, and `issue edit --assignee me` on a profile with no saved account id, now exit 3 (validation, `flag_value_invalid`) instead of 1 (auth). `auth whoami` against a profile that has no base URL now exits 2 (`profile_incomplete`) instead of 1 (auth). `jira agent guide` with an unknown section and `jira search saved` with an unknown query name now exit 3 (validation, `arg_value_invalid`) instead of 2 (not found) — the lookups are local, not Jira resources. Every `--force` gate keeps its `validation_failed`/exit 3 contract, and a genuinely absent Jira user on watcher commands keeps `not_found`/exit 2.
- The command error classifier no longer guesses an error's category from its message text — a fragile heuristic that misrouted some validation errors to the wrong exit code. A bad `auth_type` (rejected on config load or via `config set`) and `auth login` with the conflicting `--secret-stdin` and `--credential-env` flags now exit 3 (validation) instead of 1 (auth), where before the word "auth" or the flag name "credential-env" pushed them onto the authentication path. Multi-key batch commands that partially fail now carry the exit code of their worst failure directly instead of re-deriving it from the summary line, so mixed-failure batches classify correctly. Any error the CLI does not explicitly classify now defaults to validation (exit 3) rather than being sorted by keyword.

## [0.10.0](https://github.com/matcra587/jira-cli/releases/tag/v0.10.0) — 2026-07-07

### Added

- `issue view` now surfaces the discovery data Jira already returns on the read instead of discarding it: `data.issue.transitions[]` lists the workflow moves valid from the current status, and `data.issue.editmeta.fields` lists the editable fields with their required flag, accepted operations, and allowed values — so an agent or script can plan an edit or transition from a single read. The human view gains a one-line transitions summary.
- The agent contract is now versioned: `jira agent schema` reports the revision at `data.schema_version` and the full `jira agent guide` output stamps the same value, so an agent can detect a breaking change instead of discovering it mid-workflow. The scheme (what a major, minor, or patch bump means for the contract) is documented in the guide's core_contract section.
- Two discovery gaps are closed. A new `jira cache resolutions` command lists the tenant's resolution names (they vary per instance), so a resolve transition that requires a `resolution` field can be planned instead of guessed. Cached statuses now carry `status_category` (`new`, `indeterminate`, or `done`) alongside `id` and `name`, and the guide documents that status ids repeat across workflows — map a name to its category from the cache, and resolve the id per issue via the transition list.
- The `--dry-run`/`--force` safety rails now cover local state mutations, not just Jira writes. `cache clear --dry-run` reports what a live clear would remove without touching any file, `config set --dry-run` runs the full key and value validation and reports the current and new value without writing config, `auth switch --dry-run` resolves the target profile without changing the default, and `auth logout --dry-run` names the credential a live logout would revoke without opening the secret backend. The destructive pair — `cache clear` and `auth logout` — now requires `--force` in agent, non-TTY, or `--no-input` context, matching the gate on destructive Jira mutations; headless scripts that clear caches or log out must add the flag. Interactive terminals proceed without prompts.

### Changed

- `jira update` no longer asks for a yes/no confirmation in an interactive terminal — running the command is the consent, and the self-replace is already checksum-verified and rollback-safe. `--dry-run` still previews without changing anything, and headless, agent, and `--no-input` runs still require `--force`.

## [0.9.2](https://github.com/matcra587/jira-cli/releases/tag/v0.9.2) — 2026-07-07

### Added

- The JSON envelope now carries Jira's own trace id: the `Atl-Traceid` / `X-ARequestId` response header is captured as `meta.upstream_request_id` and on each error entry, so a Jira-side failure can be quoted directly to Atlassian support. The existing `meta.request_id` is generated locally and has no server-side meaning.

### Fixed

- Human output now strips ANSI escape and control bytes from Jira-controlled text (summaries, descriptions, error messages), so a crafted issue or error can no longer inject color codes or corrupt your terminal.
- `jira agent schema` now binds the comment input schema to the `comment add`/`comment edit` leaves (they previously reported no input schema) and publishes output schemas for the read/list commands — comment, attachment, link, worklog, transition, boards, and cache — so agents can discover response shapes without trial calls.
- `issue attachment download --to` is now confined to the working directory: a `..` traversal or an absolute path outside it is rejected before any HTTP call, so an agent (or a crafted instruction) can no longer write downloads outside the tree it was launched in. A new `security_posture` section in `jira agent guide` documents the threat model.

## [0.9.1](https://github.com/matcra587/jira-cli/releases/tag/v0.9.1) — 2026-07-07

### Added

- Running a command with --debug now prints the error's classification on the error line — its stable code and type, plus a retryable marker when the error is retryable — the same fields an agent reads from the JSON envelope.
- Rate-limited commands now say how long to wait: when Jira sends a Retry-After, human output adds a 'retry in Ns' line under the rate-limit hint.

## [0.9.0](https://github.com/matcra587/jira-cli/releases/tag/v0.9.0) — 2026-07-06

### Added

- Errors are now easier to act on: human output prints the next-step hint (and a did-you-mean suggestion for unknown flags or commands), the JSON envelope carries those in a new `suggestions` field, and hints across the CLI were rewritten in plain language.

### Fixed

- error envelopes now carry a complete, guarded taxonomy: read-only refusals emit code read_only, dry-run transport guards dry_run_blocked, wrong-shape ADF payloads adf_invalid (the raw json unmarshal text no longer leaks), issue-key expansion limits, ambiguous users and ambiguous boards their own codes, invalid --output values route through flag_value_invalid, every code ships a non-empty hint, a permanently deleted resource (HTTP 410) now exits 2 (not found) instead of 3, and exits 6 (canceled) / 7 (timeout) plus all emitted error fields are documented in the agent contract
- Several credential and 1Password error hints now point at the correct fix — including a 1Password failure on a build without 1Password support, and a login that can't reach Jira to verify, which previously suggested remediation that did not apply.

## [0.8.4](https://github.com/matcra587/jira-cli/releases/tag/v0.8.4) — 2026-07-06

### Fixed

- issue create/edit gain --verify: after a successful live write the issue is re-fetched and every requested field is diffed against what the server actually applied, so silently dropped labels, parents, assignees, priorities, versions, components, and custom fields surface as field_not_applied warnings with a data.verification block instead of a clean success

## [0.8.3](https://github.com/matcra587/jira-cli/releases/tag/v0.8.3) — 2026-07-06

### Fixed

- alias list human output now renders one name → expansion line per alias (natural-ordered) instead of the collapsed value={...} placeholder; the generic plain fallback also renders any string-keyed map per key, closing the class
- auth login no longer wipes existing profiles when configuring a new one without --config: the fresh-config probe now stats the resolved default path instead of the raw (empty) flag value, so adding a second profile preserves the first

## [0.8.2](https://github.com/matcra587/jira-cli/releases/tag/v0.8.2) — 2026-07-06

### Fixed

- auth login credential-rejection and cloudId discovery errors now name the HTTP status text (e.g. 401 Unauthorized) instead of the bare status code
- key- and name-like candidate lists now sort naturally (PROJ-2 before PROJ-10) — failed-key summaries in the TUI, board project keys, alias names, and JQL completion candidates

## [0.8.1](https://github.com/matcra587/jira-cli/releases/tag/v0.8.1) — 2026-07-06

### Fixed

- search jql/saved --fields now narrows the flat per-issue summary projection instead of silently switching the issues array to Jira's raw wire shape; --full remains the raw-record mode
- worklog add --started now accepts ISO-8601 and relative times (yesterday, 2h ago), normalizes them to the exact timestamp format Jira requires, and rejects unparseable values at --dry-run instead of on submit
- auth logout can now purge a credential whose profile was deleted from config: pass --base-url with the profile's old site and the orphaned keychain token is removed instead of failing with profile not found
- issue attachment download/delete now validate their issue-key argument (a traversal path or malformed key fails fast with exit 3), and negative --timeout, --max-retry-wait, and --limit values are rejected instead of silently falling back; 0 remains the documented default/disabled sentinel
- issue create/edit --json-input now accept flat-string values for object-valued system fields (project, issuetype, parent, priority, assignee, reporter, components, fixVersions, versions), lifting them to the wire shape Jira requires instead of passing --dry-run and failing on submit; explicit wire objects are untouched

## [0.8.0](https://github.com/matcra587/jira-cli/releases/tag/v0.8.0) — 2026-07-05

### Added

- Publish curated release notes on the docs site, assembled per release from change fragments.
- Read jira-cli's own changelog from the CLI with `jira release-notes` (alias `rn`): the notes are embedded in the binary and rendered as Markdown — the full history by default, or a single release with a version argument or `--latest`.

### Fixed

- Honor a named theme set in config (e.g. `dracula`, `nord`) in plain command output — tables and the `release-notes` view — instead of falling back to the dark palette; previously only the opt-in `auto` theme (or `JIRA_THEME`) styled non-TUI output.

## [0.7.7](https://github.com/matcra587/jira-cli/releases/tag/v0.7.7) — 2026-07-05

### Changed

- Detect grapheme-clustering terminals through emulator markers (Ghostty, WezTerm, foot, Contour, Windows Terminal); kitty now measures width by wcwidth, correcting its column drift.

### Fixed

- Keep issue-table columns aligned when a summary contains an inline-code span.

### Dependencies

- Update gechr/clib to v0.5.9
- Update gechr/clive to v0.2.6
- Update gechr/clog to v0.11.15
- Update gechr/x to v0.2.8

## [0.7.6](https://github.com/matcra587/jira-cli/releases/tag/v0.7.6) — 2026-07-04

### Added

- Author Jira checklists from Markdown task lists (`- [ ]` / `- [x]`); the ADF support matrix now covers every node and mark of the pinned Atlassian schema.
- Teach a foreign flag carried over from another Jira CLI, pointing at this CLI's equivalent instead of rejecting it bare.

### Fixed

- Align issue tables on grapheme-clustering terminals (Ghostty, WezTerm, foot) when a summary contains emoji.
- Pad status pills to a uniform width down the column.
- Show an unassigned issue as `unassigned` rather than a raw `map[]`.

### Dependencies

- Update gechr/primer to v0.1.1
- Update gechr/x to v0.2.4
- Update bubbletea to v2.0.8
- Update lipgloss to v2.0.5
- Update google/uuid to v1.6.0

## [0.7.5](https://github.com/matcra587/jira-cli/releases/tag/v0.7.5) — 2026-07-03

### Added

- Accept native Jira REST payloads on `create`, `edit`, `transition`, and `link` via `--json-input`.
- Render bulk operations as a live per-key block.

### Changed

- Validate `--dry-run` against the server and report only the checks it actually ran.
- Emit pagination as a single canonical block with resumable cursors.

### Fixed

- Resolve numeric transition targets against the live transition list under `--dry-run`.


## [0.7.4](https://github.com/matcra587/jira-cli/releases/tag/v0.7.4) — 2026-07-03

### Added

- Carry a comment and field edits atomically with a transition's status change.
- Resolve names and emails to Jira account identities with `user search`.


## [0.7.3](https://github.com/matcra587/jira-cli/releases/tag/v0.7.3) — 2026-07-03

### Added

- Read Markdown bodies from a file or stdin via `--markdown-file`, unified under a single `--markdown` flag.
- Accept both `--json-input` payload shapes on `create` and `edit`.

### Changed

- Normalize pasted Jira wiki markup before Markdown conversion.
- Abort strict ADF mode on content loss only, not on decoration or placement.
- Treat the wire and alias spellings of the project field as one.


## [0.7.2](https://github.com/matcra587/jira-cli/releases/tag/v0.7.2) — 2026-07-03

### Added

- Author ADF ahead of submission with standalone `adf convert` and `adf render` commands.
- Return native ADF bodies from `comment list` for lossless reuse.
- Convert GFM tables, blockquotes, and images (as alt-text links) during Markdown conversion, with source-mapped diagnostics.


## [0.7.1](https://github.com/matcra587/jira-cli/releases/tag/v0.7.1) — 2026-07-03

### Fixed

- Fail closed on an unknown or incomplete `--profile`.


## [0.7.0](https://github.com/matcra587/jira-cli/releases/tag/v0.7.0) — 2026-07-03

### Added

- Update the CLI in place with channel-aware self-update.
- Print a bare version for humans, with a `--detailed` build block.

### Dependencies

- Update gechr/clib to v0.5.8
- Update gechr/clive to v0.2.4
- Update gechr/clog to v0.11.12
- Update gechr/primer to v0.0.16
- Update gechr/x to v0.1.14

## [0.6.4](https://github.com/matcra587/jira-cli/releases/tag/v0.6.4) — 2026-06-30

### Added

- Edit an issue description with `--description-markdown`.
- Install on Windows via a published Scoop manifest.


## [0.6.2](https://github.com/matcra587/jira-cli/releases/tag/v0.6.2) — 2026-06-18

### Added

- Auto-detect and support scoped (granular) API tokens.

### Changed

- Resolve config, cache, and query directories OS-natively.

### Fixed

- Handle ADF that Jira rejects with `INVALID_INPUT`.

### Dependencies

- Update gechr/clib to v0.5.4
- Update gechr/clog to v0.11.2
- Update gechr/x to v0.1.4
- Update glamour to v2.0.1
- Update koanf to v2.3.5
- Update lipgloss to v2.0.4
- Update golang.org/x/sync to v0.21.0

## [0.6.0](https://github.com/matcra587/jira-cli/releases/tag/v0.6.0) — 2026-06-10

### Changed

- Rebuild the TUI as a section-based dashboard, with context-aware JQL autocomplete drawn from the instance's own metadata.

### Dependencies

- Update bubbletea to v2.0.7
- Update glamour to v2.0.0

## [0.5.1](https://github.com/matcra587/jira-cli/releases/tag/v0.5.1) — 2026-06-09

### Added

- Warn on a successful response that is near the rate limit.


## [0.5.0](https://github.com/matcra587/jira-cli/releases/tag/v0.5.0) — 2026-06-08

### Added

- Retry 429/503 responses within a bounded budget, honoring `Retry-After`, opt-in via `--max-retry-wait` and explained under `--debug`.


## [0.4.1](https://github.com/matcra587/jira-cli/releases/tag/v0.4.1) — 2026-06-08

### Added

- Warm every cached resource in one pass with `cache refresh`, backed by a per-resource TTL ladder and versioned entries that self-invalidate on a shape change.


## [0.4.0](https://github.com/matcra587/jira-cli/releases/tag/v0.4.0) — 2026-06-08

### Added

- Adapt output to the terminal background with an opt-in `auto` theme, honored across output and the dashboard.
- Show progress feedback for blocking and fan-out operations.
- Accept `-o` as shorthand for `--output`.

### Changed

- Colour issue-list status pills and the priority column.

### Fixed

- Write machine-mode error envelopes to stdout with a clean message.

### Dependencies

- Update gechr/clib to v0.5.2
- Update gechr/clog to v0.10.2
- Update gechr/x to v0.0.10

## [0.3.3](https://github.com/matcra587/jira-cli/releases/tag/v0.3.3) — 2026-06-03

### Added

- Complete `--status`, `--priority`, `--assignee`, and `--columns` from cached metadata in the shell.
- Validate JQL against Jira's parser (`jql validate`), list instance JQL metadata (`jql reference`), count approximate matches (`--count`), and page `search jql` with `--all`/`--limit`/`--unbounded`.

### Security

- Rebuild with Go 1.26.4 to clear two standard-library advisories on the HTTPS path to Jira: GO-2026-5039 (`net/textproto`) and GO-2026-5037 (`crypto/x509`).


## [0.3.2](https://github.com/matcra587/jira-cli/releases/tag/v0.3.2) — 2026-06-01

### Added

- Filter `issue mine` with the full list surface, plus `--updated`/`--created`/`--resolved` date filters.


## [0.3.1](https://github.com/matcra587/jira-cli/releases/tag/v0.3.1) — 2026-06-01

### Fixed

- Route config and credential writes through a symlink-aware atomic writer.


## [0.3.0](https://github.com/matcra587/jira-cli/releases/tag/v0.3.0) — 2026-05-31

### Added

- Open issues and queries in the browser.
- Transition by status name or positional target.
- Filter status with JQL comparators and negation; select and order columns with `--columns` and `--tsv`; create issues with convenience flags.

### Changed

- Colour issue statuses by workflow category.
- Require a 1Password vault and item on the headless login path, and validate the profile name at login.

### Fixed

- Apply `--order-by` to a custom `--jql` query.
- Write config through a symlinked file instead of clobbering it.

### Dependencies

- Update gechr/clib to v0.4.15
- Update gechr/clog to v0.9.8
- Update gechr/x to v0.0.8

## [0.2.0](https://github.com/matcra587/jira-cli/releases/tag/v0.2.0) — 2026-05-29

### Added

- Fan out multi-key operations with bounded parallelism.

### Dependencies

- Update golang.org/x/sync to v0.20.0

## [0.1.2](https://github.com/matcra587/jira-cli/releases/tag/v0.1.2) — 2026-05-28

### Added

- Accept issue-key ranges (`PROJ-1..PROJ-5`).


## [0.1.1](https://github.com/matcra587/jira-cli/releases/tag/v0.1.1) — 2026-05-27

### Fixed

- Render agent human output as styled JSON.


## [0.1.0](https://github.com/matcra587/jira-cli/releases/tag/v0.1.0) — 2026-05-27

Initial release — a Jira Cloud CLI built for developer and agent workflows.

### Added

- Log in to Jira Cloud with an API token, credentials keyed per site.
- Read, create, edit, and mutate issues, with an honest `--dry-run`.
- Emit machine output modes with agent detection, typed command-line parse failures, and an actionable hint on every Jira failure.
- Validate and preserve submitted ADF, and validate custom fields against the Jira screen schema.
- Page and query with truthful JQL, backed by a site-isolated metadata cache.
