package detect

import (
	"regexp"
	"strings"
)

// versionInName finds the versions in a program's name: dotted numbers, as
// "23.01" or "go1.22.0", maybe with a suffix as "-beta", or a v and a
// number, as "v5". A year, as in "Visual Studio 2022", is no version.
var versionInName = regexp.MustCompile(`(?i)v?\d+(\.\d+)+([-+_]?[a-z]*\d*)?|\bv\d+\b`)

// versionPattern stands for any version in a rule.
const versionPattern = `v?\d[\w.+-]*`

// SuggestRule proposes a detection rule, a regular expression of
// DisplayName, from the name of an installed program: the whole name, with
// its versions standing for any version, so that the rule finds the program
// once updated and nothing else.
//
//   - "7-Zip 23.01 (x64)" gives `^7-Zip v?\d[\w.+-]* \(x64\)$`;
//   - "Git" gives `^Git$`, which "GitHub Desktop" does not match.
func SuggestRule(displayName string) string {
	name := strings.TrimSpace(displayName)
	var b strings.Builder
	b.WriteString("^")
	last := 0
	for _, loc := range versionInName.FindAllStringIndex(name, -1) {
		b.WriteString(regexp.QuoteMeta(name[last:loc[0]]))
		b.WriteString(versionPattern)
		last = loc[1]
	}
	b.WriteString(regexp.QuoteMeta(name[last:]))
	b.WriteString("$")
	return b.String()
}

// RepoAppName is the name an app of a GitHub repository likely has: the
// repository's name, without a suffix as "-releases" or "-desktop".
func RepoAppName(repo string) string {
	name := repo
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		name = name[i+1:]
	}
	lower := strings.ToLower(name)
	for _, suffix := range []string{"-releases", "-release", "-desktop", "-app", "-windows", "-win"} {
		if strings.HasSuffix(lower, suffix) && len(name) > len(suffix) {
			return name[:len(name)-len(suffix)]
		}
	}
	return name
}

// SuggestRuleFromRepo proposes a detection rule from a GitHub repository's
// name, for an app not installed yet: programs are mostly named after their
// repository, as "LocalSend" of localsend/localsend.
func SuggestRuleFromRepo(repo string) string {
	return "^" + regexp.QuoteMeta(RepoAppName(repo))
}
