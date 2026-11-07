package version

import "testing"

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"v1.2.3":           "1.2.3",
		"refs/tags/v2.0":   "2.0",
		"1.0.0+build.7":    "1.0.0",
		" V3.1 ":           "3.1",
		"version-1":        "version-1",
		"v":                "v",
		"0.9.1+2026101702": "0.9.1",
		"2.56.0.windows.1": "2.56.0.windows.1",
		"desktop-v1.2.0":   "desktop-v1.2.0",
		"2026.11.07":       "2026.11.07",
		"vnext":            "vnext",
	} {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		cmp  int
		ok   bool
	}{
		{"v1.2.3", "1.2.3", 0, true},
		{"1.2", "1.2.0", 0, true},
		{"1.10.0", "1.9.9", 1, true},
		{"2.55.0.5", "v2.56.0.windows.1", -1, true},
		{"v2.103.3", "2.103.3", 0, true},
		{"2026.11.07", "2026.10.30", 1, true},
		{"1.2.3-beta", "1.2.3", 0, true},
		{"0.9.3+2026110401", "0.9.1+2026101702", 1, true},
		{"nightly", "1.0", 0, false},
		{"", "1.0", 0, false},
	}
	for _, tc := range cases {
		cmp, ok := Compare(tc.a, tc.b)
		if cmp != tc.cmp || ok != tc.ok {
			t.Errorf("Compare(%q, %q) = %d, %v; want %d, %v", tc.a, tc.b, cmp, ok, tc.cmp, tc.ok)
		}
	}
}

func TestMajorUpgrade(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"2.9.1", "3.0", true},
		{"v1.2", "v2.0.0", true},
		{"1.2", "1.3", false},
		{"3.0", "2.9", false},
		{"2025.10.1", "2026.1.0", false}, // a calendar version
		{"", "2.0", false},
		{"nightly", "2.0", false},
	}
	for _, c := range cases {
		if got := MajorUpgrade(c.a, c.b); got != c.want {
			t.Errorf("MajorUpgrade(%q, %q) = %v", c.a, c.b, got)
		}
	}
}
