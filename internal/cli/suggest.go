package cli

import (
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/agnivade/levenshtein"
)

// suggestionMaxDistance is the largest Levenshtein edit distance at which
// a candidate is still offered as a "did you mean" suggestion. Fixed at 2
// to match Cobra's own command-suggestion default: a wider threshold
// turns genuinely distinct names into false suggestions the user learns
// to distrust.
const suggestionMaxDistance = 2

// suggestionMinNameLen is the shortest candidate eligible for a
// suggestion. Names below this length sit within edit distance 2 of one
// another, so suggesting them is noise rather than help.
const suggestionMinNameLen = 4

// suggestionLimit caps how many candidates a single failure offers.
const suggestionLimit = 2

// Suggest returns up to suggestionLimit candidates within
// suggestionMaxDistance edit distance of input, closest first. Candidates
// shorter than suggestionMinNameLen are skipped. The returned names are
// the raw candidate strings — the caller adds any "--" prefix.
//
// This deliberately stays off xstrings.Closest: Suggestions is a multi-valued
// field fed by Cobra's own SuggestionsFor on the unknown-command path, and
// this helper keeps unknown-flag suggestions on the same contract — plain
// Levenshtein at Cobra's fixed distance-2 default, up to two candidates.
// Closest returns a single best match under a length-proportional Damerau
// threshold, which would change the error payload shape and break that parity.
func Suggest(input string, candidates []string) []string {
	if input == "" {
		return nil
	}
	type scored struct {
		name string
		dist int
	}
	matches := make([]scored, 0, len(candidates))
	for _, c := range candidates {
		if c == input || utf8.RuneCountInString(c) < suggestionMinNameLen {
			continue
		}
		d := levenshtein.ComputeDistance(input, c)
		if d <= suggestionMaxDistance {
			matches = append(matches, scored{name: c, dist: d})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].dist < matches[j].dist
	})
	out := make([]string, 0, suggestionLimit)
	for _, m := range matches {
		out = append(out, m.name)
		if len(out) == suggestionLimit {
			break
		}
	}
	return out
}

// foreignFlagEquivalents maps a flag borrowed from another Jira CLI to
// this CLI's equivalents, offered as suggestions when the unknown flag
// has no near-miss match. Only flags with NO equivalent anywhere in this
// CLI belong here — a flag that exists on some commands (raw, columns,
// web) would produce a misleading suggestion, and a flag the failing
// command accepts never reaches flag_unknown at all.
var foreignFlagEquivalents = map[string][]string{
	"plain":       {"--output=human", "--output=json"},
	"gjq":         {"--jq"},
	"template":    {"--output=json"},
	"no-headers":  {"--output=json"},
	"no-truncate": {"--output=json"},
	"paginate":    {"--limit", "--all", "--cursor"},
}

// ForeignFlagSuggestions resolves a flag name against the foreign-CLI
// table, tolerating leading dashes and case drift in how the parser
// reported it. It returns a fresh slice, or nil for a flag with no known
// equivalent.
func ForeignFlagSuggestions(flag string) []string {
	return slices.Clone(foreignFlagEquivalents[normalizeFlagName(flag)])
}

// isForeignFlag reports whether the flag is in the foreign-CLI table.
func isForeignFlag(flag string) bool {
	_, ok := foreignFlagEquivalents[normalizeFlagName(flag)]
	return ok
}

// normalizeFlagName strips leading dashes and case drift from a flag name
// as the parser reported it.
func normalizeFlagName(flag string) string {
	return strings.ToLower(strings.TrimLeft(strings.TrimSpace(flag), "-"))
}
