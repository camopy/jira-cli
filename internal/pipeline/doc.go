// Package pipeline implements the deterministic 5-stage validation
// pipeline that gates every Jira mutation submission. Stages run in
// fixed order; warnings collected by earlier stages survive a
// later-stage fatal so the envelope always carries the full context.
//
// Stages:
//
//  1. Parse / shape
//  2. ADF + compatibility (pkg/adf calls)
//  3. Field schema / screen validation (this file)
//  4. Customfield registry (pkg/jira/customfield)
//  5. Dry-run preview / live submit
//
// Strict aborts at the first FATAL stage. Best-effort drops or coerces
// where defined and continues.
package pipeline
