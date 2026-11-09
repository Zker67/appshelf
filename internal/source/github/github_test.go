package github

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"appshelf/internal/catalog"
	"appshelf/internal/detect"
	"appshelf/internal/source"
	"appshelf/internal/state"
)

func names(ns ...string) []Asset {
	out := make([]Asset, len(ns))
	for i, n := range ns {
		out[i] = Asset{Name: n}
	}
	return out
}

func TestArchOf(t *testing.T) {
	for name, want := range map[string]string{
		"gh_2.103.3_windows_amd64.msi":  "amd64",
		"gh_2.103.3_windows_386.msi":    "386",
		"gh_2.103.3_windows_arm64.msi":  "arm64",
		"App-Setup-x86_64.exe":          "amd64",
		"app-win32-setup.exe":           "386",
		"app-x86.exe":                   "386",
		"tool-aarch64.msi":              "arm64",
		"Setup.exe":                     "",
		"PowerToysSetup-0.85.0-x64.exe": "amd64",
		"box64-tool.exe":                "", // "x64" inside a word is not an architecture
	} {
		if got := archOf(name); got != want {
			t.Errorf("archOf(%q) = %q, want %q", name, got, want)
		}
	}
}

// TestChooseAsset uses the names of real releases.
func TestChooseAsset(t *testing.T) {
	gh := names(
		"gh_2.103.3_checksums.txt", "gh_2.103.3_linux_amd64.tar.gz", "gh_2.103.3_macOS_universal.pkg",
		"gh_2.103.3_windows_386.msi", "gh_2.103.3_windows_386.zip",
		"gh_2.103.3_windows_amd64.msi", "gh_2.103.3_windows_amd64.zip",
		"gh_2.103.3_windows_arm64.msi", "gh_2.103.3_windows_arm64.zip",
	)
	for machine, want := range map[string]string{
		"amd64": "gh_2.103.3_windows_amd64.msi",
		"arm64": "gh_2.103.3_windows_arm64.msi",
		"386":   "gh_2.103.3_windows_386.msi",
	} {
		a, err := ChooseAsset(gh, "", machine)
		if err != nil || a.Name != want {
			t.Errorf("%s: %q %v, want %q", machine, a.Name, err, want)
		}
	}

	// .msi before .exe, whatever the order.
	if a, _ := ChooseAsset(names("App-1.0-x64.exe", "App-1.0-x64.msi"), "", "amd64"); a.Name != "App-1.0-x64.msi" {
		t.Errorf("msi first: %q", a.Name)
	}
	// The machine's architecture before none said, and before x86.
	if a, _ := ChooseAsset(names("App-x86.exe", "App.exe", "App-x64.exe"), "", "amd64"); a.Name != "App-x64.exe" {
		t.Errorf("architecture: %q", a.Name)
	}
	// Two installers of one kind and architecture: none is chosen.
	ptoys := names("PowerToysSetup-0.85.0-x64.exe", "PowerToysUserSetup-0.85.0-x64.exe", "PowerToysSetup-0.85.0-arm64.exe")
	_, err := ChooseAsset(ptoys, "", "amd64")
	var amb *AmbiguousError
	if !errors.As(err, &amb) || len(amb.Names) != 2 {
		t.Errorf("ambiguous: %v", err)
	}
	// A pattern settles it.
	if a, err := ChooseAsset(ptoys, `^PowerToysUserSetup`, "amd64"); err != nil || a.Name != "PowerToysUserSetup-0.85.0-x64.exe" {
		t.Errorf("with a pattern: %q %v", a.Name, err)
	}
	// Debug builds and other systems' files are left out.
	if a, err := ChooseAsset(names("app-debug-x64.exe", "app-x64.exe", "app.dmg"), "", "amd64"); err != nil || a.Name != "app-x64.exe" {
		t.Errorf("debug: %q %v", a.Name, err)
	}
	if _, err := ChooseAsset(names("app.zip", "app.tar.gz"), "", "amd64"); !errors.Is(err, ErrNoInstaller) {
		t.Errorf("no installer: %v", err)
	}
	if _, err := ChooseAsset(names("app-arm64.msi"), "", "amd64"); !errors.Is(err, ErrNoInstaller) {
		t.Errorf("only another architecture: %v", err)
	}
	if _, err := ChooseAsset(gh, `nomatch`, "amd64"); err == nil {
		t.Error("a pattern matching nothing chose an asset")
	}
}

// fakeGitHub serves releases by repository and their assets.
type fakeGitHub struct {
	mu        sync.Mutex
	releases  map[string]Release
	files     map[string][]byte
	remaining int
	requests  int
}

func (f *fakeGitHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if data, ok := f.files[r.URL.Path]; ok {
		w.Write(data)
		return
	}
	f.requests++
	w.Header().Set("X-RateLimit-Limit", "60")
	w.Header().Set("X-RateLimit-Remaining", fmt.Sprint(f.remaining))
	w.Header().Set("X-RateLimit-Reset", fmt.Sprint(time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC).Unix()))
	if f.remaining == 0 {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	f.remaining--
	repo, ok := strings.CutSuffix(strings.TrimPrefix(r.URL.Path, "/repos/"), "/releases/latest")
	rel, found := f.releases[repo]
	if !ok || !found {
		http.NotFound(w, r)
		return
	}
	json.NewEncoder(w).Encode(rel)
}

func newTestSource(t *testing.T, f *fakeGitHub, entries []detect.Entry) (*Source, *httptest.Server, *state.Store) {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	store, err := state.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	s := New(Options{Store: store, TempDir: t.TempDir(), API: srv.URL})
	s.machine = "amd64"
	s.scan = func() ([]detect.Entry, error) { return entries, nil }
	return s, srv, store
}

func ghApp(id, repo, rule string) catalog.App {
	return catalog.App{ID: id, Name: repo, Source: catalog.Source{Type: catalog.GitHub, Repo: repo}, Detect: &catalog.Detect{DisplayName: rule}}
}

func TestStatuses(t *testing.T) {
	f := &fakeGitHub{remaining: 50, releases: map[string]Release{
		"o/new":       {Tag: "v2.0.0", Assets: names("new-2.0.0-x64.msi")},
		"o/old":       {Tag: "v2.0.0", Assets: names("old-2.0.0-x64.msi")},
		"o/current":   {Tag: "v2.0.0", Assets: names("cur-2.0.0-x64.msi")},
		"o/ambiguous": {Tag: "v1.0", Assets: names("a-setup-x64.exe", "a-user-x64.exe")},
		"o/odd":       {Tag: "nightly-20261107", Assets: names("odd.msi")},
		"o/recorded":  {Tag: "v3.1", Assets: names("rec.msi")},
	}}
	entries := []detect.Entry{
		{DisplayName: "Old App", DisplayVersion: "1.5.0"},
		{DisplayName: "Current App", DisplayVersion: "2.0.0.0"},
		{DisplayName: "Odd App", DisplayVersion: ""},
		{DisplayName: "Recorded App", DisplayVersion: "3.1.0.17"},
	}
	s, _, store := newTestSource(t, f, entries)
	store.PutGitHub("a_rec", state.GitHubRecord{Tag: "v3.1", AssetName: "rec.msi"})
	apps := []catalog.App{
		ghApp("a_new", "o/new", "^New App"),
		ghApp("a_old", "o/old", "^Old App"),
		ghApp("a_cur", "o/current", "^Current App"),
		ghApp("a_amb", "o/ambiguous", "^Ambiguous"),
		ghApp("a_odd", "o/odd", "^Odd App"),
		ghApp("a_rec", "o/recorded", "^Recorded App"),
		ghApp("a_404", "o/missing", "^Missing"),
	}
	got, err := s.Statuses(context.Background(), apps)
	if err != nil {
		t.Fatal(err)
	}
	check := func(id string, state source.State, ok func(source.Status) bool) {
		t.Helper()
		st := got[id]
		if st.State != state || (ok != nil && !ok(st)) {
			t.Errorf("%s: %+v", id, st)
		}
	}
	check("a_new", source.NotInstalled, func(st source.Status) bool { return st.Asset == "new-2.0.0-x64.msi" && st.Latest == "v2.0.0" })
	check("a_old", source.UpdateAvailable, func(st source.Status) bool { return st.Version == "1.5.0" && st.Latest == "v2.0.0" })
	check("a_cur", source.Installed, nil)
	check("a_amb", source.Unknown, func(st source.Status) bool { return strings.Contains(st.Note, "匹配规则") })
	check("a_odd", source.Installed, func(st source.Status) bool { return strings.Contains(st.Note, "无法比较版本") })
	// The registry says 3.1.0.17, the tag v3.1: AppShelf's record of v3.1
	// says it is current.
	check("a_rec", source.Installed, nil)
	check("a_404", source.NotInstalled, func(st source.Status) bool { return strings.Contains(st.Note, "仓库不存在") })

	if r := s.Rate(); !r.Known || r.Limit != 60 {
		t.Errorf("rate %+v", r)
	}

	// A record of an older release with a version that does not compare:
	// a newer release is an update.
	store.PutGitHub("a_odd", state.GitHubRecord{Tag: "nightly-20261101"})
	st, _ := s.Recheck(context.Background(), apps[4])
	if st.State != source.UpdateAvailable {
		t.Errorf("odd with an older record: %+v", st)
	}
}

func TestRateLimit(t *testing.T) {
	f := &fakeGitHub{remaining: 0, releases: map[string]Release{"o/a": {Tag: "v1"}}}
	s, _, _ := newTestSource(t, f, nil)
	_, err := s.Latest(context.Background(), "o/a")
	var rl *RateLimitError
	if !errors.As(err, &rl) || rl.Reset.IsZero() {
		t.Errorf("got %v", err)
	}
}

func TestInstall(t *testing.T) {
	payload := []byte("pretend installer")
	sum := sha256.Sum256(payload)
	f := &fakeGitHub{remaining: 50, files: map[string][]byte{"/dl/app-1.0-x64.exe": payload, "/dl/app-1.0-x64.msi": payload}}
	s, srv, _ := newTestSource(t, f, nil)
	f.releases = map[string]Release{
		"o/exe":  {Tag: "v1.0", Assets: []Asset{{Name: "app-1.0-x64.exe", Size: int64(len(payload)), Digest: "sha256:" + hex.EncodeToString(sum[:]), URL: srv.URL + "/dl/app-1.0-x64.exe"}}},
		"o/msi":  {Tag: "v1.0", Assets: []Asset{{Name: "app-1.0-x64.msi", Size: int64(len(payload)), URL: srv.URL + "/dl/app-1.0-x64.msi"}}},
		"o/bad":  {Tag: "v1.0", Assets: []Asset{{Name: "app-1.0-x64.exe", Size: int64(len(payload)), Digest: "sha256:00", URL: srv.URL + "/dl/app-1.0-x64.exe"}}},
		"o/fail": {Tag: "v1.0", Assets: []Asset{{Name: "app-1.0-x64.exe", URL: srv.URL + "/dl/app-1.0-x64.exe"}}},
	}
	var ran []string
	s.shellRun = func(file, params, dir string) (uint32, error) {
		// The installer run is the file downloaded: the .exe itself, or the
		// .msi quoted after msiexec's /i.
		installer := file
		if strings.EqualFold(filepath.Base(file), "msiexec.exe") {
			_, rest, _ := strings.Cut(params, `"`)
			installer, _, _ = strings.Cut(rest, `"`)
		}
		data, err := os.ReadFile(installer)
		if err != nil || string(data) != string(payload) {
			return 1, fmt.Errorf("installer not there: %v", err)
		}
		ran = append(ran, filepath.Base(file)+" "+params)
		if strings.Contains(params, "/fail") {
			return 1603, nil
		}
		return 0, nil
	}

	exe := ghApp("a_exe", "o/exe", "^App")
	exe.Source.InstallerArgs = "/S"
	res, err := s.Install(context.Background(), exe, nil)
	if err != nil || res.GitHub == nil || res.GitHub.Tag != "v1.0" || res.GitHub.AssetName != "app-1.0-x64.exe" {
		t.Fatalf("exe: %+v %v", res, err)
	}
	if len(ran) != 1 || !strings.HasSuffix(ran[0], " /S") {
		t.Errorf("exe ran %q", ran)
	}

	prog := &progressLog{}
	if _, err := s.Install(context.Background(), ghApp("a_msi", "o/msi", "^App"), prog); err != nil {
		t.Fatalf("msi: %v", err)
	}
	if got := strings.Join(prog.stages, ","); got != "正在下载,正在校验,正在运行安装程序" {
		t.Errorf("stages = %s", got)
	}
	if prog.checked.Done != int64(len(payload)) || prog.checked.Total != int64(len(payload)) {
		t.Errorf("the download's bytes = %+v", prog.checked)
	}
	if len(ran) != 2 || !strings.HasPrefix(ran[1], "msiexec.exe /i ") || !strings.HasSuffix(ran[1], "/norestart /passive") {
		t.Errorf("msi ran %q", ran)
	}

	if _, err := s.Install(context.Background(), ghApp("a_bad", "o/bad", "^App"), nil); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Errorf("a wrong digest: %v", err)
	}
	if len(ran) != 2 {
		t.Errorf("an installer with a wrong digest ran: %q", ran)
	}

	failing := ghApp("a_fail", "o/fail", "^App")
	failing.Source.InstallerArgs = "/fail"
	if _, err := s.Install(context.Background(), failing, nil); err == nil || !strings.Contains(err.Error(), "1603") {
		t.Errorf("exit code 1603: %v", err)
	}

	left, _ := os.ReadDir(s.tempDir)
	if len(left) != 0 {
		t.Errorf("downloads left behind: %v", left)
	}
}

// progressLog is a log that keeps the progress reported.
type progressLog struct {
	strings.Builder
	stages  []string
	checked source.Progress // when the download ended
}

func (l *progressLog) ReportProgress(p source.Progress) {
	if n := len(l.stages); n == 0 || l.stages[n-1] != p.Stage {
		l.stages = append(l.stages, p.Stage)
	}
	if p.Stage == "正在校验" {
		l.checked = p
	}
}

// TestLatestReal reads a real release from GitHub, without a token.
func TestLatestReal(t *testing.T) {
	store, _ := state.Open(filepath.Join(t.TempDir(), "state.json"))
	s := New(Options{Store: store, TempDir: t.TempDir(), Token: os.Getenv("GITHUB_TOKEN")})
	s.machine = "amd64"
	rel, asset, err := s.Describe(context.Background(), ghApp("a_gh", "cli/cli", "^GitHub CLI"))
	var rl *RateLimitError
	if errors.As(err, &rl) {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(asset.Name, "_windows_amd64.msi") || !strings.HasPrefix(asset.Digest, "sha256:") || asset.Size == 0 {
		t.Errorf("%s: chose %+v", rel.Tag, asset)
	}
	if r := s.Rate(); !r.Known {
		t.Error("no rate reported")
	}
}

// TestInstallRealOptIn installs a real release, only with
// APPSHELF_INSTALL_TEST=1, APPSHELF_TEST_GITHUB_REPO and
// APPSHELF_TEST_GITHUB_DETECT (a DisplayName rule), chosen by the user.
func TestInstallRealOptIn(t *testing.T) {
	repo, rule := os.Getenv("APPSHELF_TEST_GITHUB_REPO"), os.Getenv("APPSHELF_TEST_GITHUB_DETECT")
	if os.Getenv("APPSHELF_INSTALL_TEST") != "1" || repo == "" || rule == "" {
		t.Skip("set APPSHELF_INSTALL_TEST=1, APPSHELF_TEST_GITHUB_REPO and APPSHELF_TEST_GITHUB_DETECT to install a release")
	}
	store, _ := state.Open(filepath.Join(t.TempDir(), "state.json"))
	s := New(Options{Store: store, TempDir: t.TempDir(), Token: os.Getenv("GITHUB_TOKEN")})
	app := ghApp("a_real", repo, rule)
	app.Source.InstallerArgs = os.Getenv("APPSHELF_TEST_GITHUB_ARGS")
	if _, err := s.Install(context.Background(), app, os.Stdout); err != nil {
		t.Fatal(err)
	}
	st, _ := s.Recheck(context.Background(), app)
	if !st.IsInstalled() {
		t.Errorf("after installing: %+v", st)
	}
}

// TestUseToken checks a token against /rate_limit, whose quotas it keeps,
// and carries only an accepted token.
func TestUseToken(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.URL.Path+" "+r.Header.Get("Authorization"))
		mu.Unlock()
		if r.URL.Path == "/rate_limit" && r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/rate_limit" {
			fmt.Fprint(w, `{"resources":{"core":{"limit":5000,"remaining":4990,"reset":1800000000},"search":{"limit":30,"remaining":30,"reset":1800000060}}}`)
			return
		}
		fmt.Fprint(w, `{"full_name":"o/r"}`)
	}))
	t.Cleanup(srv.Close)
	s := New(Options{TempDir: t.TempDir(), API: srv.URL, Token: "env"})
	if !s.HasToken() {
		t.Fatal("the token of the options is not used")
	}

	if err := s.UseToken(context.Background(), "bad"); !errors.Is(err, ErrBadToken) {
		t.Fatalf("bad token = %v", err)
	}
	if _, err := s.Repository(context.Background(), "o/r"); err != nil {
		t.Fatal(err)
	}
	if err := s.UseToken(context.Background(), "good"); err != nil {
		t.Fatal(err)
	}
	if r := s.Rate(); !r.Known || r.Limit != 5000 || r.Remaining != 4990 {
		t.Errorf("core quota = %+v", r)
	}
	if r := s.SearchRate(); !r.Known || r.Limit != 30 {
		t.Errorf("search quota = %+v", r)
	}
	if _, err := s.Repository(context.Background(), "o/r"); err != nil {
		t.Fatal(err)
	}
	s.SetToken("")
	if s.HasToken() || s.Rate().Known {
		t.Error("clearing kept the token or its quota")
	}
	if _, err := s.Repository(context.Background(), "o/r"); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	want := []string{"/rate_limit Bearer bad", "/repos/o/r Bearer env", "/rate_limit Bearer good", "/repos/o/r Bearer good", "/repos/o/r "}
	if !slices.Equal(seen, want) {
		t.Errorf("requests = %q, want %q", seen, want)
	}
}

// TestUseTokenReal asks the real GitHub about a token that is not one: it
// is refused, and the source keeps sending none.
func TestUseTokenReal(t *testing.T) {
	s := New(Options{TempDir: t.TempDir()})
	err := s.UseToken(context.Background(), "ghp_AppShelfTestNotAToken0000000000000000")
	if err != nil && strings.Contains(err.Error(), "无法连接") {
		t.Skipf("GitHub unreachable: %v", err)
	}
	if !errors.Is(err, ErrBadToken) {
		t.Fatalf("UseToken = %v, want ErrBadToken", err)
	}
	if s.HasToken() {
		t.Error("a refused token is carried")
	}
}
