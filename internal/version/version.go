// Package version compares the versions of apps as their sources write
// them: release tags such as "v1.2.3", registry versions such as
// "2.55.0.5", calendar versions such as "2026.11.07".
package version

import (
	"strconv"
	"strings"
)

// Normalize drops what is not part of the version: a "refs/tags/" prefix,
// a leading "v", and "+build" metadata.
func Normalize(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "refs/tags/")
	if len(s) > 1 && (s[0] == 'v' || s[0] == 'V') && s[1] >= '0' && s[1] <= '9' {
		s = s[1:]
	}
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	return s
}

// numbers returns the leading dotted numbers of a normalized version, as
// [2 56 0] for "2.56.0.windows.1", and whether it has any.
func numbers(s string) ([]int, bool) {
	var out []int
	for part := range strings.SplitSeq(s, ".") {
		end := 0
		for end < len(part) && part[end] >= '0' && part[end] <= '9' {
			end++
		}
		if end == 0 {
			break
		}
		n, err := strconv.Atoi(part[:end])
		if err != nil {
			break
		}
		out = append(out, n)
		if end < len(part) {
			break // "3-beta": the numbers stop at the suffix
		}
	}
	return out, len(out) > 0
}

// Compare compares two versions by their leading dotted numbers, missing
// parts counting as 0: -1 when a is older than b, 1 when newer, 0 when the
// same. ok is false when either has no leading number, as "nightly", and
// the two cannot be compared.
func Compare(a, b string) (cmp int, ok bool) {
	na, oka := numbers(Normalize(a))
	nb, okb := numbers(Normalize(b))
	if !oka || !okb {
		return 0, false
	}
	for i := range max(len(na), len(nb)) {
		var x, y int
		if i < len(na) {
			x = na[i]
		}
		if i < len(nb) {
			y = nb[i]
		}
		switch {
		case x < y:
			return -1, true
		case x > y:
			return 1, true
		}
	}
	return 0, true
}
