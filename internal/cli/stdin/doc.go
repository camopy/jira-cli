// Package stdin is the ONLY place in the source tree allowed to read
// os.Stdin. Three opt-in surfaces exist:
//
//   - JSONInput(path) is the only way command-payload JSON enters the
//     CLI; pass "-" to mean stdin.
//   - TextInput(path) is the only way non-JSON text payloads (Markdown
//     for `adf convert`) enter the CLI; pass "-" to mean stdin.
//   - SecretStdin() is the only way credential material enters the CLI
//     via stdin; kept separate so a JSON parser never accidentally eats
//     a secret.
//
// Tests/contract/no_input_no_stdin_test.go and
// tests/guardrails/stdin_discipline_test.go enforce that no other
// package directly references os.Stdin.
package stdin
