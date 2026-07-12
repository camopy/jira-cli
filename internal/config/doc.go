// Package config is the layered configuration store for jira-cli: a koanf v2
// loader over defaults, a TOML file, and a JIRA_* environment overlay (last
// wins), plus the profile model and the credential backends profiles point at.
//
// Profiles are metadata only — name, base URL, email, and which secret backend
// holds the token. Credentials never live here: they are kept in the OS keyring,
// 1Password, or a JIRA_TOKEN_* environment variable, and must never reach the
// TOML file, a log, or an envelope. The store types (KeyringStore,
// OnePasswordStore, EnvCredentialStore) read and write those secrets; the loader
// only records where each profile's secret is found.
//
// Decode is strict: an unknown key is rejected rather than dropped, so a rename
// without a migration path fails loudly instead of silently discarding a setting
// on the next Save. Load never writes; LoadOrInit bootstraps a missing file and
// deliberately skips the env overlay, so a transient env value is never persisted.
// Every write goes through an atomic, symlink-following temp-file+rename.
package config
