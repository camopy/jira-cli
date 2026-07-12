// Package startup parses jira's global flags and first subcommand out of the
// raw argv before cobra runs. The root alias-expansion preflight needs the
// requested config/profile and the first command token before the command
// tree is built, so this logic operates on []string directly — no cobra.
package startup
