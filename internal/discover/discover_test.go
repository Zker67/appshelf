package discover

import (
	"context"
	"encoding/json/v2"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"appshelf/internal/catalog"
	"appshelf/internal/inventory"
	"appshelf/internal/source/github"
	"appshelf/internal/source/scoop"
	"appshelf/internal/source/winget"
)

func TestFold(t *testing.T) {
	for in, want := range map[string]string{
		"Visual Studio Code": "visualstudiocode",
		"visual-studio_code": "visualstudiocode",
		"  微信 WeChat ":       "微信wechat",
		"Notepad++":          "notepad",
	} {
		if got := Fold(in); got != want {
			t.Errorf("Fold(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRank(t *testing.T) {
	type hit struct{ id, name string }
	hits := []hit{
		{"Obsidian.Clipper", "Obsidian Web Clipper"},
		{"Foo.ObsidianTools", "Tools for obsidian"},
		{"Obsidian.Obsidian", "Obsidian"},
		{"Bar.Obsidian-Thing", "obsidian thing"},
	}
	names := func(h hit) []string { return []string{h.name, h.id, LastPart(h.id)} }
	got := Rank("obsidian", hits, names)
	var ids []string
	for _, h := range got {
		ids = append(ids, h.id)
	}
	want := []string{"Obsidian.Obsidian", "Obsidian.Clipper", "Foo.ObsidianTools", "Bar.Obsidian-Thing"}
	if !slices.Equal(ids, want) {
		t.Errorf("Rank = %v, want %v", ids, want)
	}
	// Loose: spaces and case do not matter.
	got = Rank("VISUAL studio", []hit{{"x.Other", "Other"}, {"Microsoft.VisualStudioCode", "Visual Studio Code"}}, names)
	if got[0].id != "Microsoft.VisualStudioCode" {
		t.Errorf("loose rank = %v", got)
	}
	if got := Rank("", hits, names); !slices.Equal(got, hits) {
		t.Error("an empty query changed the order")
	}
}

func TestLastPart(t *testing.T) {
	for in, want := range map[string]string{"Git.Git": "Git", "main/7zip": "7zip", "owner/repo": "repo", "plain": "plain", "A.B.C": "C"} {
		if got := LastPart(in); got != want {
			t.Errorf("LastPart(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestManifestDir(t *testing.T) {
	cases := map[[2]string]string{
		{"Microsoft.VisualStudioCode", "1.95.3"}:             "m/Microsoft/VisualStudioCode/1.95.3",
		{"Microsoft.VisualStudio.2022.Community", "17.12.0"}: "m/Microsoft/VisualStudio/2022/Community/17.12.0",
		{"7zip.7zip", "24.08"}:                               "7/7zip/7zip/24.08",
		{"Foo.Bar", "1.0 beta"}:                              "f/Foo/Bar/1.0%20beta",
	}
	for in, want := range cases {
		if got := ManifestDir(in[0], in[1]); got != want {
			t.Errorf("ManifestDir(%q, %q) = %q, want %q", in[0], in[1], got, want)
		}
	}
}

func TestParseManifest(t *testing.T) {
	m, err := ParseManifest([]byte("\xef\xbb\xbfPackageIdentifier: Foo.Bar\nPackageVersion: 1.10\nPackageName: Foo\nTags:\n- a\n- b\nDescription: |-\n  Line one\n  Line two\n"))
	if err != nil {
		t.Fatal(err)
	}
	if m.PackageVersion != "1.10" || m.PackageName != "Foo" || !slices.Equal(m.Tags, []string{"a", "b"}) || m.Description != "Line one\nLine two" {
		t.Errorf("manifest = %+v", m)
	}
	if _, err := ParseManifest([]byte("Tags: [unclosed")); err == nil {
		t.Error("bad YAML parsed")
	}
}

// manifestServer serves files by path and counts the requests.
func manifestServer(t *testing.T, files map[string]string) (*httptest.Server, *atomic.Int32) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		body, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &n
}

func TestWingetDetails(t *testing.T) {
	dir := "/manifests/f/Foo/Bar/2.0/"
	srv, requests := manifestServer(t, map[string]string{
		dir + "Foo.Bar.yaml":                    "PackageIdentifier: Foo.Bar\nPackageVersion: 2.0\nDefaultLocale: en-US\nManifestType: version\n",
		dir + "Foo.Bar.locale.en-US.yaml":       "PackageName: Foo Bar\nPublisher: Foo Inc.\nShortDescription: Does foo.\nDescription: Does foo, at length.\nPackageUrl: https://foo.example\nLicense: MIT\nTags: [foo]\nPackageVersion: 2.0\n",
		dir + "Foo.Bar.locale.zh-CN.yaml":       "ShortDescription: 做 foo。\nTags: [工具]\n",
		"/manifests/s/Solo/App/1/Solo.App.yaml": "ManifestType: singleton\nPackageName: Solo\nPublisher: One\nShortDescription: Alone.\nPackageVersion: 1\n",
	})
	latestCalls := 0
	d := NewDetails(Fetchers{WingetLatest: func(ctx context.Context, id string) (string, error) {
		latestCalls++
		return "2.0", nil
	}}, t.TempDir(), srv.URL+"/manifests")
	ctx := context.Background()

	det, err := d.Get(ctx, PackageRef{Type: catalog.Winget, ID: "Foo.Bar"})
	if err != nil {
		t.Fatal(err)
	}
	if det.Name != "Foo Bar" || det.Publisher != "Foo Inc." || det.Summary != "做 foo。" || det.Description != "Does foo, at length." ||
		det.Homepage != "https://foo.example" || det.License != "MIT" || det.Version != "2.0" || !slices.Equal(det.Tags, []string{"工具"}) {
		t.Errorf("details = %+v", det)
	}
	if latestCalls != 1 {
		t.Errorf("latest version read %d times", latestCalls)
	}

	// Cached: no more requests, also from a new reader of the same directory.
	before := requests.Load()
	if _, err := d.Get(ctx, PackageRef{Type: catalog.Winget, ID: "foo.bar", Version: "2.0"}); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != before {
		t.Error("cached details read again")
	}

	solo, err := d.Get(ctx, PackageRef{Type: catalog.Winget, ID: "Solo.App", Version: "1"})
	if err != nil || solo.Name != "Solo" || solo.Summary != "Alone." {
		t.Errorf("singleton = %+v, %v", solo, err)
	}

	if _, err := d.Get(ctx, PackageRef{Type: catalog.Winget, ID: "No.Such", Version: "1"}); err == nil || !strings.Contains(err.Error(), "没有这个版本") {
		t.Errorf("missing manifest: %v", err)
	}
}

func TestDetailsCacheExpires(t *testing.T) {
	calls := 0
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	d := NewDetails(Fetchers{ScoopInfo: func(ctx context.Context, bucket, name string) (scoop.Info, error) {
		calls++
		return scoop.Info{Name: name, Description: "d", Version: "1", Website: "https://x.example", License: "MIT"}, nil
	}}, t.TempDir(), "")
	d.now = func() time.Time { return now }
	ref := PackageRef{Type: catalog.Scoop, ID: "main/7zip"}
	for range 2 {
		if det, err := d.Get(context.Background(), ref); err != nil || det.Summary != "d" || det.Homepage != "https://x.example" {
			t.Fatalf("scoop details = %+v, %v", det, err)
		}
	}
	if calls != 1 {
		t.Errorf("read %d times within a day", calls)
	}
	now = now.Add(25 * time.Hour)
	if _, err := d.Get(context.Background(), ref); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("read %d times after a day", calls)
	}
	if _, err := d.Get(context.Background(), PackageRef{Type: catalog.Scoop, ID: "nobucket"}); err == nil {
		t.Error("scoop reference without bucket accepted")
	}
}

func TestGitHubDetails(t *testing.T) {
	repo := github.Repo{FullName: "o/app", Description: "An app", Stars: 42, Homepage: "https://app.example", HTMLURL: "https://github.com/o/app"}
	repo.Owner.Login = "o"
	relErr := error(&github.RateLimitError{})
	release := github.Release{}
	calls := 0
	d := NewDetails(Fetchers{
		GitHubRepo: func(ctx context.Context, r string) (github.Repo, error) { calls++; return repo, nil },
		GitHubRelease: func(ctx context.Context, app catalog.App) (github.Release, github.Asset, error) {
			if app.Source.Repo != "o/app" {
				t.Errorf("release of %q", app.Source.Repo)
			}
			return release, github.Asset{Name: "app-setup.exe"}, relErr
		},
	}, t.TempDir(), "")
	ctx := context.Background()
	ref := PackageRef{Type: catalog.GitHub, ID: "o/app"}

	// The quota used up: details without a release, not kept.
	det, err := d.Get(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if det.Stars != 42 || det.Homepage != "https://app.example" || det.PublisherURL != "https://github.com/o/app" || det.Release == nil || det.Release.AssetError == "" {
		t.Errorf("details = %+v %+v", det, det.Release)
	}
	release, relErr = github.Release{Tag: "v1.2.0", HTMLURL: "https://github.com/o/app/releases/v1.2.0"}, nil
	det, err = d.Get(ctx, ref)
	if err != nil || det.Version != "v1.2.0" || det.Release.Asset != "app-setup.exe" || det.Release.AssetError != "" {
		t.Errorf("details = %+v %+v, %v", det, det.Release, err)
	}
	if calls != 2 {
		t.Errorf("repository read %d times, want 2", calls)
	}
	// Kept now.
	if _, err := d.Get(ctx, ref); err != nil || calls != 2 {
		t.Errorf("kept details read again: %d, %v", calls, err)
	}
	// No installer to choose is a fact of the release.
	d2 := NewDetails(Fetchers{
		GitHubRepo: func(ctx context.Context, r string) (github.Repo, error) { return repo, nil },
		GitHubRelease: func(ctx context.Context, app catalog.App) (github.Release, github.Asset, error) {
			return github.Release{Tag: "v2"}, github.Asset{}, github.ErrNoInstaller
		},
	}, "", "")
	det, err = d2.Get(ctx, ref)
	if err != nil || det.Release.Tag != "v2" || det.Release.AssetError != github.ErrNoInstaller.Error() {
		t.Errorf("no installer: %+v, %v", det.Release, err)
	}
	// The repository unreadable: an error.
	d3 := NewDetails(Fetchers{GitHubRepo: func(ctx context.Context, r string) (github.Repo, error) {
		return github.Repo{}, errors.New("仓库不存在")
	}}, "", "")
	if _, err := d3.Get(ctx, ref); err == nil {
		t.Error("no error for a missing repository")
	}
}

// Stars and archived are left out where a package has none, as json/v2,
// which the bindings use, sends a zero tagged omitempty.
func TestDetailsJSONOmitsZero(t *testing.T) {
	for _, tc := range []struct {
		det  PackageDetails
		want []string
		not  []string
	}{
		{PackageDetails{Summary: "A browser"}, nil, []string{`"stars"`, `"archived"`}},
		{PackageDetails{Stars: 42, Archived: true}, []string{`"stars":42`, `"archived":true`}, nil},
	} {
		b, err := json.Marshal(tc.det)
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range tc.want {
			if !strings.Contains(string(b), w) {
				t.Errorf("%s lacks %s", b, w)
			}
		}
		for _, n := range tc.not {
			if strings.Contains(string(b), n) {
				t.Errorf("%s has %s", b, n)
			}
		}
	}
}

func TestSuggestRules(t *testing.T) {
	entries := []inventory.Entry{
		{Name: "LocalSend", DisplayName: "LocalSend", Version: "1.15.4", Publisher: "Tien Do Nam"},
		{Name: "Optimized Tool", DisplayName: "Optimized Tool"},
		{Name: "draw.io 24.7.5", DisplayName: "draw.io 24.7.5", Publisher: "JGraph"},
		{Name: "Other by owner", Publisher: "localsend"},
	}
	got := SuggestRules("localsend/localsend", entries)
	if len(got) != 3 || got[0].From != "LocalSend" || got[0].Rule != "^LocalSend$" || got[1].From != "Other by owner" || got[2].From != "" || got[2].Rule != "^localsend" {
		t.Errorf("localsend = %+v", got)
	}
	got = SuggestRules("jgraph/drawio-desktop", entries)
	if len(got) != 2 || got[0].From != "draw.io 24.7.5" || got[0].Rule != `^draw\.io v?\d[\w.+-]*$` || got[1].Rule != "^drawio" {
		t.Errorf("drawio = %+v", got)
	}
	got = SuggestRules("zed-industries/zed", entries)
	if len(got) != 1 || got[0].Rule != "^zed" {
		t.Errorf("zed = %+v", got)
	}
}

// TestDetailsReal reads the manifests of Visual Studio Code from winget's
// community repository. It needs the network and winget's module.
func TestDetailsReal(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	w := winget.New()
	if av := w.Check(ctx); !av.OK {
		t.Skipf("winget not available: %s", av.Reason)
	}
	d := NewDetails(Fetchers{WingetLatest: w.LatestVersion}, t.TempDir(), "")
	det, err := d.Get(ctx, PackageRef{Type: catalog.Winget, ID: "Microsoft.VisualStudioCode"})
	if err != nil {
		t.Skipf("manifests not readable here: %v", err)
	}
	t.Logf("%+v", det)
	if det.Publisher == "" || det.Homepage == "" || det.Summary == "" {
		t.Errorf("details lack publisher, homepage or summary: %+v", det)
	}
}
