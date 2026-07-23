// Package cmdutil holds the cross-cutting helper layer shared by every
// jira-cli command: envelope writers, client/profile accessors, output-mode
// resolution, mutation gates, the credential-warning sink, and small generic
// value helpers. Command output flows through cmdutil to internal/cli and then
// to Cobra's io.Writer; every layer returns destination failures to the command.
// It is a strict leaf package — it depends on the shared internal/cli,
// internal/config, internal/jira, and internal/adf layers but never on cmd/jira
// or any internal/cli/<command> package.
package cmdutil
