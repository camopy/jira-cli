## self_update
Goal: Bring the jira binary itself to the latest release, or learn which installer command does.
When: the installed version is behind the latest release and the workflow (or a human) wants the newer binary — this is a local binary operation and never touches Jira.

**Decide**
- Just checking? `--dry-run` reports the channel, current and latest versions, and whether an update is available, without changing anything.
- Operating in agent / non-TTY / `--no-input` mode and want the live update? `--force` is mandatory — replacing the running binary is a local mutation.
- Binary installed by Scoop, mise, or `go install`? The command changes nothing and reports the installer command to run instead (see Behavior).

**Run**
- Check only (agent): `jira update --dry-run --output=json`
- Live update (agent): `jira update --force --output=json`
- Interactive: `jira update` (no confirmation prompt — running the command is the consent; progress renders on stderr)

**Save**
> Requires `--output=json`.
- `data.channel` [string] — detected install channel: `brew`, `github-archive`, `scoop`, `mise`, or `go-install`.
- `data.current` / `data.latest` [string] — installed and newest release versions (`latest` absent on managed channels).
- `data.update_available` [bool] — whether `latest` is strictly newer (self-updating channels).
- `data.updated` [bool] — whether the binary was actually replaced this run.
- `data.managed` [bool] + `data.hint` [string] — present when the installer owns updates; `hint` is the exact command to run.

**Preconditions**
- Live self-updates require `--force` in agent / non-TTY / `--no-input` mode; omitting it exits `3` with `validation_error`.
- A from-source build (`go build`) has no update channel: the command fails with reinstall guidance instead of guessing.
- Archive downloads are verified against GoReleaser's `checksums.txt`; release cosign signatures are not verified by the updater. Homebrew updates run through `brew` itself.

**Behavior**
- Channel detection reads the binary itself: a Homebrew prefix path means `brew upgrade` runs through the tap; a Scoop or mise path means that installer owns the binary (`scoop update jira` / `mise up github:matcra587/jira-cli`); a module-proxy version in build info means `go install` owns it; a GoReleaser build stamp outside any installer tree means the release archive is self-replaced in place (rollback-safe).
- Managed channels (`scoop`, `mise`, `go-install`) always exit `0` without touching the binary — treat `data.hint` as the follow-up command.
- Progress and the old→new result line render on stderr; the stdout envelope stays clean JSON.
- `--dry-run` performs a release lookup over the network but writes nothing.
- Separately from this command, interactive TTY sessions get a passive once-per-24h "jira is outdated" stderr hint served from a local cache. It never appears in agent or piped contexts (detection disables it entirely), and `JIRA_NO_UPDATE_CHECK=1` turns it off.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| `validation_error` (exit `3`) requesting `--force` | Agent / non-TTY live update without `--force` | Add `--force`, or use `--dry-run` to check only. |
| `cannot determine the install channel` | From-source build | Reinstall via Homebrew, Scoop, mise, a release archive, or `go install`; from-source users rebuild instead. |
| `check for updates: ...` network error | Release lookup unreachable (offline, proxy, rate limit) | Retry with connectivity; a `GITHUB_TOKEN` raises the API rate limit on the archive channel. |
| Binary replaced but old version still runs | Another copy earlier on `PATH` | `where jira` / `which -a jira` and remove or reorder the stale copy. |

**Next**
- Then: `jira version --output=json` to confirm the new binary reports the expected release.
- Composes: → `core_contract` for envelope/exit-code semantics.
