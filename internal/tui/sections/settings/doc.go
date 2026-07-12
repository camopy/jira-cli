// Package settings is the dashboard's configuration view: it shows where the
// active config came from and what it resolved to, offers a manual reload, and
// watches the file (on the shared refresh heartbeat) so edits hot-apply
// without restarting the TUI. Theme and credentials still need a restart —
// the theme applies once per process and auth requires a new client.
package settings
