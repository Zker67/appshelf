package discover

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"appshelf/internal/catalog"
	"appshelf/internal/source/github"
	"appshelf/internal/source/scoop"
	"appshelf/internal/source/winget"
)

func rel(tag string, pre bool) github.Release {
	return github.Release{Tag: tag, Name: "Release " + tag, Body: "notes of " + tag, Prerelease: pre, HTMLURL: "https://github.com/o/app/releases/tag/" + tag}
}

func tags(rs []github.Release) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Tag
	}
	return out
}

func TestPickReleases(t *testing.T) {
	rels := []github.Release{rel("v2.0.0-rc.1", true), rel("v1.3.0", false), rel("v1.2.1-beta", true), rel("v1.2.0", false), rel("v1.1.0", false), rel("v1.0.0", false)}
	for _, tc := range []struct {
		name, from, to string
		want           []string
		ok             bool
	}{
		{"from installed to latest", "1.1.0", "1.3.0", []string{"v1.3.0", "v1.2.0"}, true},
		{"latest when to is empty", "1.1.0", "", []string{"v1.3.0", "v1.2.0"}, true},
		{"prerelease offered is listed", "1.2.0", "2.0.0-rc.1", []string{"v2.0.0-rc.1", "v1.3.0"}, true},
		{"from unknown: to only", "", "1.2.0", []string{"v1.2.0"}, true},
		{"from not comparable: to only", "nightly", "1.3.0", []string{"v1.3.0"}, true},
		{"from not older: to only", "1.3.0", "1.3.0", []string{"v1.3.0"}, true},
		{"1.2 is 1.2.0", "1.0.0", "1.2", []string{"v1.2.0", "v1.1.0"}, true},
		{"rc is not the release", "1.0", "1.2.1", nil, false},
		{"no such tag", "1.0", "9.9", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, more, ok := pickReleases(rels, tc.from, tc.to)
			if ok != tc.ok || !slices.Equal(tags(got), tc.want) || more {
				t.Errorf("pickReleases(%q, %q) = %v, more %v, ok %v; want %v, ok %v", tc.from, tc.to, tags(got), more, ok, tc.want, tc.ok)
			}
		})
	}

	// Words between the numbers: matched loosely, but never a prerelease.
	git := []github.Release{rel("v2.56.0.windows.4", false), rel("v2.56.0-rc1.windows.1", true), rel("v2.56.0.windows.1", false), rel("v2.55.0.windows.2", false)}
	got, more, ok := pickReleases(git, "2.55.0.2", "2.56.0.4")
	if !ok || more || !slices.Equal(tags(got), []string{"v2.56.0.windows.4", "v2.56.0.windows.1"}) {
		t.Errorf("loose: %v, ok %v", tags(got), ok)
	}
	if _, _, ok := pickReleases(git, "", "2.56.0-rc1"); ok {
		t.Error("a prerelease matched loosely")
	}

	// More than ten: the newest ten, and more.
	var many []github.Release
	for i := 30; i >= 1; i-- {
		many = append(many, rel(fmt.Sprintf("v1.%d", i), false))
	}
	got, more, ok = pickReleases(many, "1.5", "1.30")
	if !ok || len(got) != maxNotes || got[0].Tag != "v1.30" || !more {
		t.Errorf("many: %v, more %v, ok %v", tags(got), more, ok)
	}
	// A full page that stops after from: older ones may be missing.
	got, more, _ = pickReleases(many[:30], "0.1", "1.3")
	if !more || len(got) != 3 {
		t.Errorf("page stops after from: %v, more %v", tags(got), more)
	}
}

func TestLooseVersion(t *testing.T) {
	for v, want := range map[string]string{
		"2.56.0.windows.4": "2.56.0.4",
		"2.56.0.4":         "2.56.0.4",
		"1.02.3":           "1.2.3",
		"2.0-rc1":          "",
		"1.2-beta.3":       "",
		"7":                "",
		"nightly":          "",
		"1.2.arch.3":       "1.2.3",
	} {
		if got := looseVersion(v); got != want {
			t.Errorf("looseVersion(%q) = %q, want %q", v, got, want)
		}
	}
}

func TestTagVersion(t *testing.T) {
	for tag, want := range map[string]string{
		"v1.2.3":         "1.2.3",
		"1.2":            "1.2",
		"app-v1.4.0":     "1.4.0",
		"release/2.0":    "2.0",
		"Bandizip-7.47":  "7.47",
		"nightly":        "nightly",
		"2026.10.4":      "2026.10.4",
		"refs/tags/v0.9": "0.9",
	} {
		if got := tagVersion(tag); got != want {
			t.Errorf("tagVersion(%q) = %q, want %q", tag, got, want)
		}
	}
}

func TestRepoOf(t *testing.T) {
	for link, want := range map[string]string{
		"https://github.com/o/app":                               "o/app",
		"https://github.com/o/app/releases/tag/v1.2.0":           "o/app",
		"https://www.github.com/o/app.git":                       "o/app",
		"https://github.com/o/my.app/releases/download/v1/x.exe": "o/my.app",
		"https://github.com/o":                                   "",
		"https://github.com/sponsors/o":                          "",
		"https://example.com/o/app":                              "",
		"":                                                       "",
	} {
		if got := RepoOf(link); got != want {
			t.Errorf("RepoOf(%q) = %q, want %q", link, got, want)
		}
	}
}

func TestGitHubReleaseNotes(t *testing.T) {
	calls := 0
	quota := false
	d := NewDetails(Fetchers{GitHubReleases: func(ctx context.Context, repo string) ([]github.Release, error) {
		calls++
		if quota {
			return nil, &github.RateLimitError{}
		}
		if repo != "o/app" {
			t.Errorf("releases of %q", repo)
		}
		return []github.Release{rel("v1.3.0", false), rel("v1.2.0", false), rel("v1.1.0", false)}, nil
	}}, "", "")
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	d.now = func() time.Time { return now }
	ctx := context.Background()
	ref := PackageRef{Type: catalog.GitHub, ID: "o/app", Version: "v1.3.0"}

	n, err := d.ReleaseNotes(ctx, ref, "v1.1.0")
	if err != nil {
		t.Fatal(err)
	}
	if n.From != "github" || n.Repo != "o/app" || len(n.Notes) != 2 || n.Notes[0].Version != "v1.3.0" || n.Notes[0].Body != "notes of v1.3.0" || n.Link == "" {
		t.Errorf("notes = %+v", n)
	}
	// Kept for an hour.
	if _, err := d.ReleaseNotes(ctx, ref, "v1.1.0"); err != nil || calls != 1 {
		t.Errorf("read again within the hour: %d calls, %v", calls, err)
	}
	now = now.Add(2 * time.Hour)
	quota = true
	if _, err := d.ReleaseNotes(ctx, ref, "v1.1.0"); err == nil {
		t.Error("no error with the quota used up")
	}

	// Tags that do not follow the versions: no notes, a reason, the link.
	quota = false
	n, err = d.ReleaseNotes(ctx, PackageRef{Type: catalog.GitHub, ID: "o/app", Version: "7.0"}, "6.0")
	if err != nil || len(n.Notes) != 0 || n.Reason == "" || n.Link == "" || n.Notes == nil {
		t.Errorf("mismatch: %+v, %v", n, err)
	}
}

func TestWingetReleaseNotes(t *testing.T) {
	dir := "/manifests/f/Foo/Bar/2.0/"
	gone := "/manifests/g/Gone/App/3.1/"
	inst := "/manifests/i/Inst/App/1.5/"
	srv, _ := manifestServer(t, map[string]string{
		// Notes in the manifest, homepage elsewhere, installers off GitHub.
		dir + "Foo.Bar.yaml":              "PackageVersion: 2.0\nDefaultLocale: en-US\nManifestType: version\n",
		dir + "Foo.Bar.locale.en-US.yaml": "PackageVersion: 2.0\nPackageUrl: https://foo.example\nReleaseNotes: |-\n  - Fixed a crash\n  - Faster start\nReleaseNotesUrl: https://foo.example/changes\n",
		dir + "Foo.Bar.installer.yaml":    "Installers:\n- InstallerUrl: https://foo.example/setup.exe\n",
		// The homepage is the repository.
		gone + "Gone.App.yaml":              "PackageVersion: 3.1\nDefaultLocale: en-US\nManifestType: version\n",
		gone + "Gone.App.locale.en-US.yaml": "PackageVersion: 3.1\nPackageUrl: https://github.com/o/app\n",
		// Only the installers name the repository.
		inst + "Inst.App.yaml":              "PackageVersion: 1.5\nDefaultLocale: en-US\nManifestType: version\n",
		inst + "Inst.App.locale.en-US.yaml": "PackageVersion: 1.5\nPackageUrl: https://inst.example\n",
		inst + "Inst.App.installer.yaml":    "Installers:\n- InstallerUrl: https://github.com/o/inst/releases/download/v1.5/setup.exe\n",
	})
	releases := map[string][]github.Release{
		"o/app":  {rel("v3.1", false), rel("v3.0", false)},
		"o/inst": {rel("v1.5", false), rel("v1.4", false), rel("v1.3", false)},
	}
	var ghErr error
	d := NewDetails(Fetchers{GitHubReleases: func(ctx context.Context, repo string) ([]github.Release, error) {
		return releases[repo], ghErr
	}}, "", srv.URL+"/manifests")
	ctx := context.Background()

	n, err := d.ReleaseNotes(ctx, PackageRef{Type: catalog.Winget, ID: "Foo.Bar", Version: "2.0"}, "1.9")
	if err != nil {
		t.Fatal(err)
	}
	if n.From != "winget" || len(n.Notes) != 1 || !strings.Contains(n.Notes[0].Body, "Faster start") || n.Notes[0].Version != "2.0" || n.Link != "https://foo.example/changes" {
		t.Errorf("manifest notes = %+v", n)
	}

	n, err = d.ReleaseNotes(ctx, PackageRef{Type: catalog.Winget, ID: "Gone.App", Version: "3.1"}, "2.9")
	if err != nil || n.From != "github" || n.Repo != "o/app" || len(n.Notes) != 2 {
		t.Errorf("homepage repository: %+v, %v", n, err)
	}

	n, err = d.ReleaseNotes(ctx, PackageRef{Type: catalog.Winget, ID: "Inst.App", Version: "1.5"}, "1.3")
	if err != nil || n.Repo != "o/inst" || len(n.Notes) != 2 || n.Notes[0].Version != "v1.5" {
		t.Errorf("installer repository: %+v, %v", n, err)
	}

	// GitHub failing: the reason says so, the releases page stays, and
	// nothing is kept.
	ghErr = errors.New("无法连接 GitHub")
	n, err = d.ReleaseNotes(ctx, PackageRef{Type: catalog.Winget, ID: "Inst.App", Version: "1.5"}, "1.0")
	if err != nil || len(n.Notes) != 0 || !strings.Contains(n.Reason, "无法连接") || n.Link != "https://github.com/o/inst/releases" {
		t.Errorf("GitHub failing: %+v, %v", n, err)
	}
	ghErr = nil
	if n, _ := d.ReleaseNotes(ctx, PackageRef{Type: catalog.Winget, ID: "Inst.App", Version: "1.5"}, "1.0"); len(n.Notes) != 3 {
		t.Errorf("failure kept: %+v", n)
	}

	if _, err := d.ReleaseNotes(ctx, PackageRef{Type: catalog.Winget, ID: "No.Such", Version: "1"}, ""); err == nil {
		t.Error("no error for a missing manifest")
	}
}

func TestScoopReleaseNotes(t *testing.T) {
	site := "https://github.com/o/app"
	d := NewDetails(Fetchers{
		ScoopInfo: func(ctx context.Context, bucket, name string) (scoop.Info, error) {
			return scoop.Info{Name: name, Version: "3.1", Website: site}, nil
		},
		GitHubReleases: func(ctx context.Context, repo string) ([]github.Release, error) {
			return []github.Release{rel("v3.1", false), rel("v3.0", false)}, nil
		},
	}, "", "")
	ctx := context.Background()
	n, err := d.ReleaseNotes(ctx, PackageRef{Type: catalog.Scoop, ID: "extras/app"}, "3.0")
	if err != nil || n.From != "github" || len(n.Notes) != 1 || n.Notes[0].Version != "v3.1" {
		t.Errorf("scoop from GitHub: %+v, %v", n, err)
	}
	site = "https://app.example"
	d2 := NewDetails(Fetchers{ScoopInfo: func(ctx context.Context, bucket, name string) (scoop.Info, error) {
		return scoop.Info{Name: name, Version: "3.1", Website: site}, nil
	}}, "", "")
	n, err = d2.ReleaseNotes(ctx, PackageRef{Type: catalog.Scoop, ID: "extras/app"}, "3.0")
	if err != nil || len(n.Notes) != 0 || n.Reason == "" {
		t.Errorf("scoop without a repository: %+v, %v", n, err)
	}
}

// TestReleaseNotesReal reads the release notes of packages from winget's
// repository and GitHub: one whose installers are on GitHub, one whose
// notes are only a link. It reads only.
func TestReleaseNotesReal(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	w := winget.New()
	if av := w.Check(ctx); !av.OK {
		t.Skipf("winget not available: %s", av.Reason)
	}
	gh := github.New(github.Options{Token: os.Getenv("GITHUB_TOKEN")})
	d := NewDetails(Fetchers{WingetLatest: w.LatestVersion, GitHubReleases: gh.Releases}, t.TempDir(), "")
	for _, id := range []string{"Git.Git", "Microsoft.VisualStudioCode", "BurntSushi.ripgrep.MSVC"} {
		n, err := d.ReleaseNotes(ctx, PackageRef{Type: catalog.Winget, ID: id}, "")
		if err != nil {
			t.Logf("%s: %v", id, err)
			continue
		}
		var vs []string
		for _, note := range n.Notes {
			vs = append(vs, fmt.Sprintf("%s(%d)", note.Version, len(note.Body)))
		}
		t.Logf("%s: from %q repo %q link %q notes %v reason %q", id, n.From, n.Repo, n.Link, vs, n.Reason)
	}
}
