// Package cmdutil holds the cross-cutting helper layer shared by every
// jira-cli command: envelope writers, keyed placement, client/profile access,
// output-mode resolution, mutation gates and small generic helpers. Commands
// pass their concrete internal/envelope Output unchanged for single-key data or
// each keyed result; cmdutil then routes it through internal/cli to Cobra's
// writer and returns destination failures. It never depends on cmd/jira or an
// internal/cli/<command> package.
package cmdutil
