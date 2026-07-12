// Package root assembles the CLI's top-level cobra command: the persistent
// flags, help renderer, PersistentPreRunE (config, logging, output-mode
// resolution), and the subcommand tree, plus the completion-preflight seam and
// Execute. It is the composition root every command package registers into.
package root
