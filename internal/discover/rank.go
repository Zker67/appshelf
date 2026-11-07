// Package discover finds software to add: it ranks search results, reads
// the details of a package (winget's manifests, scoop's manifests, a GitHub
// repository) and proposes detection rules for GitHub apps.
//
// All of it is shown to the user and nothing more: whether an app is
// installed, and what installing it does, is for its source to say.
package discover

import (
	"slices"
	"strings"
	"unicode"
)

// Fold reduces text for loose matching: lower case, without spaces and
// punctuation, so that "Visual Studio Code", "visual-studio-code" and
// "VisualStudioCode" are the same.
func Fold(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Rank orders search results by how well they match the query: a name or
// ID equal to it first, then those starting with it, then the others, each
// group in the order of its source. names returns what to compare of a
// result: its name and its ID, or the last part of the ID.
func Rank[T any](query string, items []T, names func(T) []string) []T {
	q := Fold(query)
	if q == "" {
		return items
	}
	score := func(it T) int {
		best := 2
		for _, n := range names(it) {
			switch f := Fold(n); {
			case f == q:
				return 0
			case strings.HasPrefix(f, q):
				best = 1
			}
		}
		return best
	}
	out := slices.Clone(items)
	slices.SortStableFunc(out, func(a, b T) int { return score(a) - score(b) })
	return out
}

// LastPart returns what follows the last dot or slash of an ID: the name of
// a winget package without its publisher, of a scoop app without its
// bucket, of a repository without its owner.
func LastPart(id string) string {
	if i := strings.LastIndexAny(id, "./"); i >= 0 {
		return id[i+1:]
	}
	return id
}
