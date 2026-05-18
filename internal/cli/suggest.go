package cli

import (
	"sort"
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
