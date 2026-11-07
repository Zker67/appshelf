package detect

import (
	"regexp"
	"strings"
	"testing"

	"appshelf/internal/catalog"
)

func TestSuggestRule(t *testing.T) {
	cases := []struct {
		name, want string
		also       []string // other names the rule must match
		not        []string // names it must not match
	}{
		{"7-Zip 23.01 (x64)", `^7-Zip v?\d[\w.+-]* \(x64\)$`, []string{"7-Zip 24.08 (x64)"}, []string{"7-Zip-zstd 23.01 (x64)", "7-Zip 24.08 (x86)"}},
		{"Git", `^Git$`, nil, []string{"GitHub Desktop", "Git Extensions"}},
		{"Python 3.12.1 (64-bit)", `^Python v?\d[\w.+-]* \(64-bit\)$`, []string{"Python 3.13.0 (64-bit)"}, []string{"Python Launcher"}},
		{"Microsoft Visual C++ 2015-2022 Redistributable (x64) - 14.38.33135", `^Microsoft Visual C\+\+ 2015-2022 Redistributable \(x64\) - v?\d[\w.+-]*$`, []string{"Microsoft Visual C++ 2015-2022 Redistributable (x64) - 14.40.33810"}, []string{"Microsoft Visual C++ 2015-2022 Redistributable (x86) - 14.40.33810"}},
		{"Visual Studio Community 2022", `^Visual Studio Community 2022$`, nil, []string{"Visual Studio Community 2019"}},
		{"Go Programming Language amd64 go1.22.0", `^Go Programming Language amd64 gov?\d[\w.+-]*$`, []string{"Go Programming Language amd64 go1.23.4"}, nil},
		{"Foo v5", `^Foo v?\d[\w.+-]*$`, []string{"Foo v6"}, nil},
		{"Node.js", `^Node\.js$`, nil, []string{"NodeXjs"}},
		{"  Steam  ", `^Steam$`, nil, nil},
	}
	for _, c := range cases {
		got := SuggestRule(c.name)
		if got != c.want {
			t.Errorf("SuggestRule(%q) = %q, want %q", c.name, got, c.want)
			continue
		}
		re := regexp.MustCompile("(?i)" + got)
		for _, n := range append([]string{strings.TrimSpace(c.name)}, c.also...) {
			if !re.MatchString(n) {
				t.Errorf("rule %q does not match %q", got, n)
			}
		}
		for _, n := range c.not {
			if re.MatchString(n) {
				t.Errorf("rule %q matches %q", got, n)
			}
		}
		// The rule is valid in a catalog and finds the program it came from.
		r, err := Match(catalog.Detect{DisplayName: got}, []Entry{{DisplayName: strings.TrimSpace(c.name)}})
		if err != nil || !r.Installed {
			t.Errorf("Match(%q) = %+v, %v", got, r, err)
		}
	}
}

func TestSuggestRuleFromRepo(t *testing.T) {
	cases := map[string]string{
		"localsend/localsend":          `^localsend`,
		"obsidianmd/obsidian-releases": `^obsidian`,
		"flameshot-org/flameshot":      `^flameshot`,
		"owner/my.app":                 `^my\.app`,
		"owner/-releases":              `^-releases`,
		"jgraph/drawio-desktop":        `^drawio`,
		"zed-industries/zed":           `^zed`,
		"someone/Tool-Windows":         `^Tool`,
	}
	for repo, want := range cases {
		if got := SuggestRuleFromRepo(repo); got != want {
			t.Errorf("SuggestRuleFromRepo(%q) = %q, want %q", repo, got, want)
		}
	}
}
