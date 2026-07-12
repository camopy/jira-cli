// Package action provides a single controller that collects input for the Jira
// verbs (transition, comment, assign, labels, worklog, edit) through one
// Mode-dispatched component, instead of a bespoke flow per action.
// The controller is pure state: it gathers input and produces a Request; the
// caller turns the Request into a Jira mutation and applies it optimistically.
package action
