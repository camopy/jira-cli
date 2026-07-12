package cli

import (
	"strings"
	"unicode"
)

// SentenceCase upper-cases the first rune of s, leaving the rest untouched, so a
// lower-case verb phrase reads as user-facing UI text ("listed issues" ->
// "Listed issues") while an acronym already at the front survives ("JQL
// reference" stays). The verb registry stores lower-case forms for structured
// logs; the spinner, progress bar, and completion line apply this at render so
// the surfaces the user reads are Sentence-cased, matching git / cargo / docker
// and clog's own spinner examples.
func SentenceCase(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// OperationVerb carries an operation's verb in the three forms status output
// needs: a present-progressive gerund while the work runs ("caching"), a
// past-tense confirmation on success ("cached"), and an infinitive for failures
// ("failed to cache …"). Noun names the object acted on.
//
// Forms are lower case to match the house log style — clog's own lines and the
// plain completion messages are lower case. The message is a terse event
// string; the variable detail (issue key, elapsed time, failure reason) travels
// as structured fields (key=, time=, reason=), never baked into these phrases.
type OperationVerb struct {
	Gerund     string // "caching", "creating", "deleting"
	Past       string // "cached", "created", "deleted"
	Infinitive string // "cache", "create", "delete"
	Noun       string // "boards", "issue", "worklog"
}

// phrase joins a verb form with the noun, e.g. ("creating","issue") -> "creating
// issue". An empty noun returns the form alone.
func phrase(form, noun string) string {
	if noun == "" {
		return form
	}
	return form + " " + noun
}

// Gerundf returns the in-progress phrase, "creating issue".
func (v OperationVerb) Gerundf() string { return phrase(v.Gerund, v.Noun) }

// Pastf returns the success phrase, "created issue".
func (v OperationVerb) Pastf() string { return phrase(v.Past, v.Noun) }

// Failuref returns the failure phrase, "failed to create issue".
func (v OperationVerb) Failuref() string { return phrase("failed to "+v.Infinitive, v.Noun) }

// Conditionalf returns the dry-run preview phrase, "would create issue" —
// the past-tense confirmation would overstate a preview that submitted
// nothing.
func (v OperationVerb) Conditionalf() string { return phrase("would "+v.Infinitive, v.Noun) }

// Preview reworks the verb for a dry-run lifecycle: the operation is not
// performed, only previewed, so every phrase speaks about the preview
// itself — "previewing issue edit", "previewed issue edit", "failed to
// preview issue edit" — instead of claiming the mutation. The original
// noun and infinitive collapse into a compound noun, which reads cleanly
// for transitive verbs with plain nouns (edit/delete/clone/move/
// transition + issue). Particle infinitives ("comment on") and
// prepositional nouns ("issues to epic") would garble — rework the
// registry entry before adopting Preview for such an op.
func (v OperationVerb) Preview() OperationVerb {
	return OperationVerb{
		Gerund:     "previewing",
		Past:       "previewed",
		Infinitive: "preview",
		Noun:       strings.TrimSpace(v.Noun + " " + v.Infinitive),
	}
}

// NounPlural returns the object noun in plural form for batch phrases — naive
// pluralisation (append s unless already plural) that covers the operations'
// nouns: issue -> issues, transition -> transitions, web link -> web links.
func (v OperationVerb) NounPlural() string {
	if v.Noun == "" || strings.HasSuffix(v.Noun, "s") {
		return v.Noun
	}
	return v.Noun + "s"
}

// GerundPlural returns the in-progress batch phrase, "listing issues".
func (v OperationVerb) GerundPlural() string { return phrase(v.Gerund, v.NounPlural()) }

// PastPlural returns the success batch phrase, "viewed issues" — the plural
// counterpart of Pastf for multi-key summaries.
func (v OperationVerb) PastPlural() string { return phrase(v.Past, v.NounPlural()) }

// operationVerbs is the single source of truth for operation verbs, keyed by
// the command/envelope name. The spinner, the debug lifecycle, and the plain
// completion message (messageForCommand) all read their wording from here, so
// they can never disagree. Keys are sorted alphabetically; forms are lower case.
var operationVerbs = map[string]OperationVerb{
	"adf.convert":               {"converting", "converted", "convert", "markdown"},
	"adf.render":                {"rendering", "rendered", "render", "ADF"},
	"auth.login":                {"logging in", "logged in", "log in", ""},
	"auth.login.discover":       {"discovering", "discovered", "discover", "cloud ID"},
	"auth.logout":               {"logging out", "logged out", "log out", ""},
	"auth.status":               {"checking", "checked", "check", "auth"},
	"auth.switch":               {"switching", "switched", "switch", "profile"},
	"auth.whoami":               {"fetching", "fetched", "fetch", "account"},
	"boards.list":               {"listing", "listed", "list", "boards"},
	"cache.boards":              {"caching", "cached", "cache", "boards"},
	"cache.epics":               {"caching", "cached", "cache", "epics"},
	"cache.fields":              {"caching", "cached", "cache", "fields"},
	"cache.issuekeys":           {"listing", "listed", "list", "recent issue keys"},
	"cache.issuetypes":          {"caching", "cached", "cache", "issue types"},
	"cache.labels":              {"caching", "cached", "cache", "labels"},
	"cache.linktypes":           {"caching", "cached", "cache", "link types"},
	"cache.priorities":          {"caching", "cached", "cache", "priorities"},
	"cache.projects":            {"caching", "cached", "cache", "projects"},
	"cache.refresh":             {"refreshing", "refreshed", "refresh", "caches"},
	"cache.resolutions":         {"caching", "cached", "cache", "resolutions"},
	"cache.statuses":            {"caching", "cached", "cache", "statuses"},
	"epic.add":                  {"adding", "added", "add", "issues to epic"},
	"epic.board":                {"rendering", "rendered", "render", "epic board"},
	"epic.list":                 {"listing", "listed", "list", "epics"},
	"epic.remove":               {"removing", "removed", "remove", "issues from epic"},
	"issue.attachment.add":      {"uploading", "uploaded", "upload", "attachment"},
	"issue.attachment.delete":   {"deleting", "deleted", "delete", "attachment"},
	"issue.attachment.download": {"downloading", "downloaded", "download", "attachment"},
	"issue.attachment.list":     {"listing", "listed", "list", "attachments"},
	"issue.clone":               {"cloning", "cloned", "clone", "issue"},
	"issue.comment":             {"commenting on", "commented on", "comment on", "issue"},
	"issue.comment.add":         {"adding", "added", "add", "comment"},
	"issue.comment.delete":      {"deleting", "deleted", "delete", "comment"},
	"issue.comment.edit":        {"editing", "edited", "edit", "comment"},
	"issue.comment.list":        {"listing", "listed", "list", "comments"},
	"issue.create":              {"creating", "created", "create", "issue"},
	"issue.delete":              {"deleting", "deleted", "delete", "issue"},
	"issue.edit":                {"editing", "edited", "edit", "issue"},
	"issue.link":                {"linking", "linked", "link", "issues"},
	"issue.link.delete":         {"removing", "removed", "remove", "link"},
	"issue.link.list":           {"listing", "listed", "list", "links"},
	"issue.link.types":          {"fetching", "fetched", "fetch", "link types"},
	"issue.list":                {"listing", "listed", "list", "issues"},
	"issue.list.count":          {"counting", "counted", "count", "issues"},
	"issue.move":                {"moving", "moved", "move", "issue"},
	"issue.rank":                {"ranking", "ranked", "rank", "issues"},
	"issue.transition":          {"transitioning", "transitioned", "transition", "issue"},
	"issue.transitions":         {"listing", "listed", "list", "transitions"},
	"issue.view":                {"viewing", "viewed", "view", "issue"},
	"issue.watchers.add":        {"adding", "added", "add", "watcher"},
	"issue.watchers.list":       {"listing", "listed", "list", "watchers"},
	"issue.watchers.remove":     {"removing", "removed", "remove", "watcher"},
	"issue.weblink":             {"adding", "added", "add", "web link"},
	"jql.reference":             {"fetching", "fetched", "fetch", "JQL reference"},
	"me":                        {"fetching", "fetched", "fetch", "account"},
	"release.notes":             {"showing", "shown", "show", "release notes"},
	"schema":                    {"rendering", "rendered", "render", "schema"},
	"search.count":              {"counting", "counted", "count", "issues"},
	"search.jql":                {"searching", "searched", "search", "issues"},
	"search.saved":              {"searching", "searched", "search", "issues"},
	"update":                    {"updating", "updated", "update", "jira-cli"},
	"update.check":              {"checking", "checked", "check", "for updates"},
	"user.search":               {"searching", "searched", "search", "users"},
	"user.resolve":              {"resolving", "resolved", "resolve", "user"},
	"worklog.add":               {"adding", "added", "add", "worklog"},
	"worklog.list":              {"listing", "listed", "list", "worklogs"},
}

// mutatingOps marks the operations that write to Jira. Their completion
// lines log at LevelSuccess when the mutation really happened (not a
// dry-run preview, no failed keys), while reads stay at Info. Local state
// changes (config, cache, alias, auth) deliberately stay Info: the success
// level answers "did my Jira write land", and diluting it with local
// bookkeeping would blunt that signal.
var mutatingOps = map[string]bool{
	"epic.add":                true,
	"epic.remove":             true,
	"issue.attachment.add":    true,
	"issue.attachment.delete": true,
	"issue.clone":             true,
	"issue.comment":           true,
	"issue.comment.add":       true,
	"issue.comment.delete":    true,
	"issue.comment.edit":      true,
	"issue.create":            true,
	"issue.delete":            true,
	"issue.edit":              true,
	"issue.link":              true,
	"issue.link.delete":       true,
	"issue.move":              true,
	"issue.rank":              true,
	"issue.transition":        true,
	"issue.watchers.add":      true,
	"issue.watchers.remove":   true,
	"issue.weblink":           true,
	"worklog.add":             true,
}

// OpMutating reports whether an operation writes to Jira (see mutatingOps).
func OpMutating(op string) bool { return mutatingOps[op] }

// VerbFor returns the verb forms for an operation. An unknown operation falls
// back to a generic "processing" set so callers always get usable phrases.
func VerbFor(op string) OperationVerb {
	if v, ok := operationVerbs[op]; ok {
		return v
	}
	noun := op
	if i := strings.LastIndex(op, "."); i >= 0 {
		noun = op[i+1:]
	}
	return OperationVerb{Gerund: "processing", Past: "processed", Infinitive: "process", Noun: noun}
}
