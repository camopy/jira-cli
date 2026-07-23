// Package root assembles the CLI's top-level cobra command: the persistent
// flags, help renderer, PersistentPreRunE (config, logging, output-mode
// resolution), and the subcommand tree, plus the completion-preflight seam and
// Execute. Execute reports command failures through the configured output
// boundary, retains a reporting failure as a secondary cause and never retries
// a command whose result could not be written. Dynamic completion candidates
// use the root writer; Clib's upstream --print-completion flag remains the one
// deliberate process-stdout boundary. It is the composition root every command
// package registers into.
package root
