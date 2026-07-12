## rank_issues
Goal: Reorder backlog issues headlessly — the drag-to-reorder of the web UI's backlog as a single command.
When: a backlog's Rank order needs changing without the web UI — sprint prep, dependency ordering, or an agent sequencing work.

**Decide**

- Anchor and direction: `--before ANCHOR` places the issues immediately above the anchor, `--after ANCHOR` immediately below. Exactly one is required; they are mutually exclusive.
- The issues keep the order you pass them in. Key lists and ranges (`<PROJECT_KEY>-1:10`) expand like every multi-key command.
- More than 50 keys are chunked transparently (the endpoint's cap); each later chunk anchors after the last key of the one before it, so the requested order survives end-to-end. Chunks after a failure do not run, and the error names how many issues already ranked — those persist; resume with the remainder.

**Run**

    jira issue rank <PROJECT_KEY>-7 <PROJECT_KEY>-9 --before <PROJECT_KEY>-3 --no-input --output=json
    jira issue rank <PROJECT_KEY>-20:24 --after <PROJECT_KEY>-50 --no-input --output=json

- `--dry-run` previews `data.order`, the anchor, and the chunk count locally; it never contacts Jira.
- Verify the result: `jira issue list --jql "project = <PROJECT_KEY> ORDER BY Rank ASC" --output=json`.

**Read**

- `data.order` [array, required] — the submitted issue order; `data.anchor` and `data.position` echo the placement; `data.chunks` counts the requests used.

**Failure modes**

| Signal | Meaning | Move |
| ------ | ------- | ---- |
| Exit `3`, `rank_rejected` | Jira refused the rank — the project has no Jira Software board (no rank field), or the anchor cannot be ranked against | Confirm the project has a board (→ `discover_board`) and the anchor key exists on it |
| Exit `3`, `rank_partial` | The endpoint's 207: some issues ranked, the ones named in `errors[].message` did not | Fix the named issues, re-run with just them; the others already landed |
| Exit `3` at flag parse | Both or neither of `--before`/`--after` | Pass exactly one anchor |

Ranking is board-scoped Jira Software functionality; classic and team-managed projects both support it through this command.
