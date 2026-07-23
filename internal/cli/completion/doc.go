// Package completion hosts the `jira completion` command and the dynamic
// shell-completion emitters. completionEmitters maps every predictor name to
// the function that emits its candidates, so a declared predictor with no
// emitter fails the guard test rather than shipping a silently-empty completion.
// Dynamic candidates use root's command writer and retain the first write
// failure across Clib's void callback.
package completion
