## author_adf
Goal: Produce a lint-clean native ADF document from Markdown before any mutation touches Jira, and review existing ADF safely.
When: a create/edit/comment/worklog body is more than plain prose and you want conversion problems surfaced in isolation — not inside a full mutation payload at submit time.

**Decide**
- Authoring new rich text? Write Markdown, run `adf convert`, submit the emitted document via `--json-input`.
- Conversion fails strict? Fix the named Markdown line, or accept the documented downgrade with `--adf-best-effort` (then prefer `--output=json`: compact folds warnings into the payload).
- Reviewing existing ADF (a description or comment) before editing? `adf render` gives a Markdown projection — read-only, explicitly lossy; always edit and resubmit the original document, never the projection.
- Content Markdown cannot express at all (mentions, panels, status, cards)? Author native ADF directly — → `adf_reference`.

**Run**
- Author → convert → submit (agent):
  `jira adf convert --input notes.md --output=compact | jira issue comment add <ISSUE_KEY> --json-input - --no-input --output=json`
- Lint only: `jira adf convert --input notes.md --output=json` (exit 0 = will convert cleanly on submit)
- From stdin: `cat notes.md | jira adf convert --input - --output=json`
- Review existing: `jira adf render --input body.json --output=json`

**Save**
> Requires `--output=json`.
- `adf convert` `data` [object] — the ADF document itself (usable verbatim as a `--json-input` body or inside a `fields` envelope value).
- `adf render` `data.markdown` [string] — the lossy projection; `data.lossy` [bool] + `data.lossy_constructs[]` name what degraded.
- `warnings[]` — same taxonomy as mutations (`markdown_lossy_conversion` with source-mapped `path`, `unknown_adf_node`, …).

**Preconditions**
- Both subcommands are local-only: no profile, no network, safe under `JIRA_READ_ONLY`.
- Strict mode (default, mutation parity) exits `3` with `code=markdown_lossy_conversion` and the offending Markdown line/snippet on any lossy step.
- `--input` defaults to `-` (stdin); pass a file path otherwise.

**Behavior**
- `adf convert` runs the exact converter + normalizer + validator the mutation pipeline runs: a clean convert here cannot fail conversion on submit.
- Documented downgrades that stay non-lossy (strict still passes): images → alt-text links, quotes inside list items hoisted. Lossy (strict fails): decorative marks on inline code, constructs with no ADF path (raw HTML).
- `adf render` never mutates and its output is for human/agent review only.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| Exit 3, `code=markdown_lossy_conversion` | A construct degrades lossily | Fix the named line (→ `adf_reference` gotchas), or `--adf-best-effort` |
| Compact output is not a bare document | Best-effort run carried warnings (folded into compact payload) | Use `--output=json` and read `data` |
| Rendered Markdown missing content | `adf render` is lossy by design | Consult `data.lossy_constructs`; work from the original ADF |

**Next**
- Then: → `create_issue`, `edit_issue`, `add_comment`, or `log_work` with the converted document.
- Composes: → `adf_reference` (node shapes, gotchas, mark rules), → `safe_mutation`.
