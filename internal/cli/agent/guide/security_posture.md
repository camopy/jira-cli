## security_posture
Goal: State the threat model the CLI enforces — the agent is not a trusted operator — so automation authors know which inputs are validated, which paths are confined, and where the hard stops are.
When: reviewing what a prompt-injected or misbehaving agent could make the CLI do, or deciding which guardrail (read-only mode, path confinement, force gates) covers a workflow.

This is not a command — it is the cross-cutting contract behind the input and output guards every workflow inherits. The operating assumption: the process may be driven by an agent that read hostile instructions (a crafted issue description, a poisoned comment), so user-supplied identifiers, server-supplied text, and user-supplied filesystem paths are all validated at their boundaries rather than trusted.

**Decide**

# which guard covers my concern?
- Untrusted resource identifiers: issue keys go through a strict parser (traversal, percent-encoding, and embedded query params are rejected before any HTTP call), and every URL path segment is percent-encoded at the HTTP layer.
- Untrusted filesystem writes: `attachment download --to` is confined to the working directory — `..` traversal and absolute paths outside it exit `3` before any HTTP call; server-provided filenames are base-named and pass the same confinement. Downloads never stream to stdout.
- Untrusted server text: human-mode output routes Jira-controlled strings (summaries, descriptions, error messages) through a terminal sanitizer that strips ANSI escapes and control bytes; machine modes are protected by the JSON encoder.
- Untrusted mutations: read-only mode (`JIRA_READ_ONLY=true`, or per-profile `read_only = true`) blocks every state-changing HTTP method at the transport layer; destructive commands additionally require `--force` in headless / agent context. See → `safe_mutation`.

**Behavior**
- Guards fail fast and loud: a rejected input is a validation error (exit `3`) with a remediation message, never a silent rewrite of the input into something safe.
- Path confinement is a hard stop with no override flag; run the CLI from the directory the file belongs in.
- Read-only blocks and dry-run blocks live at the HTTP layer, so no command path can forget them; `--force` does not bypass read-only mode.
- The sanitizer preserves printable text (including non-ASCII); only escape sequences and control bytes are dropped.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| exit 3, "outside the working directory" | `--to` escaped the working tree | Use a path inside the working directory — see → `attach_file` |
| exit 3, invalid issue key | The KEY argument failed the strict key parser | Pass a bare `PROJ-123` style key; never URL-encode or embed query params |
| exit 3, `code=read_only` | Read-only mode blocked a mutation | Intentional guard; flip the profile or env only outside the untrusted context |

**Next**
- Composes: → `safe_mutation` (confirmation and dry-run discipline), → `core_contract` (read-only mode, exit codes, envelope contract).
