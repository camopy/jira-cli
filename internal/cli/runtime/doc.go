// Package runtime is the dependency boundary between the binary shell in
// cmd/jira and the command implementations. It carries the IO streams a
// command invocation needs that are otherwise process-global: stdout,
// stderr, and stdin.
//
// Runtime is constructed once per process by main (and once per test by
// the internal/cli/root test helper). It is deliberately small: it owns inputs,
// not behavior. It does NOT store a context.Context — main owns the root
// context via signal.NotifyContext and threads it through ExecuteContext.
// Runtime never calls os.Exit and exposes no broad service-locator
// surface; command-domain logic lives in the command packages.
//
// TTY detection is derived from the stdout stream (an *os.File in
// production) rather than carried as a separate field — see
// runtimeStdoutIsTTY in internal/cli/root. Environment, config-path, timeout,
// clock, and request-ID inputs are intentionally absent: their consumers
// are not yet wired through the runtime, and an option that does not
// reach its consumer is a correctness trap. They return to this boundary
// when each one is wired to its consumer.
package runtime
