// Package editor spawns the user's external editor over a temporary Markdown
// file and reads the edited result back.
//
// Its defining constraint is that a non-blocking editor is a data-loss hazard:
// launchers like `code` or `subl` fork and return to the shell immediately by
// default, racing the caller's temp-file cleanup against the editor's save. Run
// refuses a known non-blocking editor unless its command line carries a wait
// flag, and EditMarkdown additionally treats a sub-second exit with unchanged
// content as that same failure. Editor resolution follows
// JIRA_EDITOR → configured → $EDITOR → $VISUAL → "vi".
package editor
