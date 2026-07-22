---
slug: annotate-issues
title: Add context to an issue
description: Comments, attachments, web links, watchers, issue links, and worklogs — what each records and how its direction works.
when_to_use: Commenting on an issue, attaching files or URLs, linking two issues, managing watchers, or logging time.
commands: [jira issue comment add, jira issue attachment add, jira issue weblink, jira issue watchers add, jira issue link, jira worklog add]
order: 7
---

## Decide

Match the record to what you're adding:

*   A note for humans → comment. Body via `--markdown` or, for exact ADF,
    `--json-input`. `--visibility-role` / `--visibility-group` restrict who
    sees it.
*   A file → attachment. A URL → weblink.
*   A relationship between two issues → issue link. Direction matters:
    `KEY` is the inward issue, `--to` the outward one, and the type name
    decides what that means. Check names with `jira issue link types`
    before linking, not after.
*   Someone who should follow the issue → watcher. `--user` takes
    `accountId:<id>` (the fast path), `me`, or a bare name/email (resolved
    via a live lookup).
*   Time spent → worklog, with a duration like `2h30m`.

## Run

```sh
jira issue comment add PROJ-123 --markdown "Deployed to staging." --no-input

jira issue attachment add PROJ-123 ./crash.log --no-input
jira issue weblink PROJ-123 --url https://ci.example.com/run/42 --title "CI run" --no-input

jira issue link types                                  # confirm direction first
jira issue link PROJ-123 --to PROJ-200 --type Blocks --no-input

jira issue watchers add PROJ-123 --user accountId:5b10a2844c20165700ede21g --no-input
jira worklog add PROJ-123 --time-spent 2h30m --markdown "Debugging" --no-input
```

## Save

*   Comment and worklog ids — edits and deletes need them later.
*   `comment list` returns oldest-first: the latest comment is the last
    entry of the last page, not the first.
*   The link's `inward_issue` / `outward_issue` objects — proof the
    direction landed the way you meant.

## Preconditions

The issue key exists and the profile can edit it. Watcher adds need an
`account_id` (`jira user search`). Every command here takes `--dry-run`.

## Recover

*   Link reads backwards → delete it by id and recreate with `KEY` and
    `--to` swapped, or use the opposite type name:
    `jira issue link delete PROJ-123 9001 --no-input --force`.
    A link *with a comment* is only expressible via `--json-input` (the
    native REST body).
*   Comment or attachment deletes are destructive: headless runs need
    `--force` on top of `--no-input`.
*   Worklog duration surprises → `1d` resolves via the profile's
    `workday_seconds` (default 28,800 = 8h), not 24h; use explicit
    hours/minutes when in doubt.
*   Attachment downloads never overwrite an existing file, and `--to` is
    confined to the working directory — absolute paths and `..` exit 3.

## Next

*   `safe-mutation` — preview and gate rules for all of these.
*   `write-rich-text` — comment bodies beyond what Markdown covers.
