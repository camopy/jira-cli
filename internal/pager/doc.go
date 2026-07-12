// Package pager shows a rendered document one screen at a time on the
// user's terminal. It is deliberately dumb about policy: callers decide
// WHETHER to page (TTY, output mode, agent detection, --no-pager); this
// package only decides HOW — an explicit JIRA_PAGER/PAGER command when the
// user configured one, otherwise a built-in viewport, so paging works on
// Windows where less is often absent.
package pager
