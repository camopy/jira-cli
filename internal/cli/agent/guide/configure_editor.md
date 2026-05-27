## configure_editor
Goal: Pin the editor used by the kubectl-style `jira issue edit KEY` flow so TTY edits land in a tool that blocks until you save, instead of forking and losing your change.
When: a human user reports that `jira issue edit KEY` either backgrounds or loses changes on save — agents themselves should use `edit_issue` and never trigger this flow.

**Decide**
- Used only for the bare `jira issue edit KEY` form (no field flags). Agents must pass `--summary`, `--assignee`, or `--json-input` instead → see `edit_issue`.
- Resolution chain (highest precedence first):
  1. `JIRA_EDITOR` env var
  2. Per-profile `editor` field in TOML
  3. Global `editor` field in TOML
  4. `EDITOR` env var
  5. `VISUAL` env var
  6. `vi` (last-resort fallback)
- Editors that fork-and-return (e.g. `code` without `--wait`) are refused at spawn time with a one-line fix — silent strikethrough-and-data-loss is gone.

**Run**
- Per-invocation override: `JIRA_EDITOR='code --wait' jira issue edit KAN-1`
- Global default in `~/.config/jira-cli/config.toml`:
  ```toml
  editor = "code -w"
  ```
- Per-profile override (wins over the global on that profile):
  ```toml
  [[profiles]]
    name = "default"
    editor = "vim"
  ```

**Preconditions**
- Editor command must block until the file is closed (`--wait` / `-w` on VS Code; `vim` / `nvim` block by default). A forking editor is refused at spawn with `set EDITOR='code --wait'`-style remediation.
- Only relevant on a TTY — the bare `jira issue edit KEY` form is refused in agent / non-TTY context with exit 3.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| Spawn-time refusal naming `--wait` | Editor command forks and returns immediately | Set `EDITOR='code --wait'` (or `JIRA_EDITOR='code --wait'`) and retry |
| Exit 3, "issue edit requires an interactive terminal for the editor flow" | Running under an agent / non-TTY context | Pass `--summary`, `--assignee`, or `--json-input` instead → `edit_issue` |
| Edits open in `vi` unexpectedly | No editor configured at any precedence level | Set `JIRA_EDITOR`, `editor` in TOML, or `EDITOR`/`VISUAL` — pick the slot that matches your workflow |

**Next**
- Then: → `edit_issue` (single-shot field edits or bulk `--json-input` for agent / non-TTY contexts)
- Composes: → `auth_setup` (editor config is per-profile so it lives next to the credential metadata).
