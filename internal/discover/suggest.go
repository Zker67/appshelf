package discover

import (
	"strings"
	"unicode"

	"appshelf/internal/detect"
	"appshelf/internal/inventory"
)

// RuleSuggestion is a detection rule proposed for a GitHub app.
type RuleSuggestion struct {
	Rule string `json:"rule"`
	// From is the installed program the rule comes from, or empty for the
	// rule made of the repository's name.
	From    string `json:"from,omitempty"`
	Version string `json:"version,omitempty"`
}

// maxSuggestions limits the installed programs proposed.
const maxSuggestions = 5

// SuggestRules proposes detection rules for an app of a GitHub repository:
// first from the installed programs named after the repository, or whose
// publisher is its owner, then from the repository's name, for an app not
// installed yet.
func SuggestRules(repo string, entries []inventory.Entry) []RuleSuggestion {
	owner, _, _ := strings.Cut(repo, "/")
	want := Fold(detect.RepoAppName(repo))
	fowner := Fold(owner)
	var out []RuleSuggestion
	seen := map[string]bool{}
	for _, e := range entries {
		if len(out) == maxSuggestions {
			break
		}
		n := e.RuleName()
		hit := (want != "" && namedAfter(n, want)) ||
			(len(fowner) > 2 && Fold(e.Publisher) == fowner)
		if !hit {
			continue
		}
		rule := detect.SuggestRule(n)
		if seen[rule] {
			continue
		}
		seen[rule] = true
		out = append(out, RuleSuggestion{Rule: rule, From: n, Version: e.Version})
	}
	return append(out, RuleSuggestion{Rule: detect.SuggestRuleFromRepo(repo)})
}

// namedAfter reports whether a program's name starts with want, or one of
// its words does, comparing folded text: "draw.io 24.7" is named after
// "drawio", "Optimized Tool" is not after "zed".
func namedAfter(name, want string) bool {
	if strings.HasPrefix(Fold(name), want) {
		return true
	}
	words := strings.FieldsFunc(name, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	for _, w := range words {
		if strings.HasPrefix(Fold(w), want) {
			return true
		}
	}
	return false
}
