package inventory

import (
	"context"
	"errors"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"appshelf/internal/catalog"
	"appshelf/internal/detect"
	"appshelf/internal/source/scoop"
	"appshelf/internal/source/winget"
	"appshelf/internal/usage"
)

var registry = []detect.Entry{
	{DisplayName: "7-Zip 23.01 (x64)", DisplayVersion: "23.01", Publisher: "Igor Pavlov", Key: `HKLM\7-Zip`, Name: "7-Zip", Scope: "Machine", Arch: "X64", DisplayIcon: `C:\Program Files\7-Zip\7zFM.exe`},
	{DisplayName: "Old Tool", DisplayVersion: "1.0", Publisher: "Old Co", Key: `HKLM (32 位)\Old Tool`, Name: "Old Tool", Scope: "Machine", Arch: "X86"},
	{DisplayName: "Old Tool", DisplayVersion: "1.0", Publisher: "Old Co (x64)", Key: `HKLM\Old Tool`, Name: "Old Tool", Scope: "Machine", Arch: "X64"},
	{DisplayName: "Git", DisplayVersion: "2.44.0", Publisher: "The Git Development Community", Key: `HKLM\Git_is1`, Name: "Git_is1", Scope: "Machine", Arch: "X64", DisplayIcon: `C:\Program Files\Git\mingw64\share\git\git-for-windows.ico`},
	{DisplayName: "Git", DisplayVersion: "2.40.0", Publisher: "Another Git", Key: `HKCU\Git`, Name: "Git", Scope: "User", Arch: "X64"},
	{DisplayName: "Only In Registry", DisplayVersion: "3", Publisher: "Reg", Key: `HKCU\Only`, Name: "Only", Scope: "User", Arch: "X64"},
}

var packages = []winget.Package{
	{ID: `ARP\Machine\X86\Old Tool`, Name: "Old Tool", Version: "1.0"},
	{ID: `ARP\Machine\X64\7-Zip`, Name: "7-Zip 23.01 (x64)", Version: "23.01"},
	{ID: "Git.Git", Name: "Git", Version: "2.44.0", Source: "winget", Update: "2.45.1"},
	{ID: `MSIX\Microsoft.WindowsCalculator_11.2405.2.0_x64__8wekyb3d8bbwe`, Name: "Windows Calculator", Version: "11.2405.2.0"},
	{ID: "9NBLGGH4NNS1", Name: "App Installer", Version: "1.0", Source: "msstore"},
	{ID: "Git.Git", Name: "Git", Version: "2.40.0", Source: "winget"},
	{ID: "Foo.Bar", Name: "Foo", Version: "1", Source: "corp"},
}

var scoopApps = []scoop.App{{Name: "ripgrep", Version: "14.1.0", Bucket: "main", Update: "14.1.1"}}

func byKey(entries []Entry) map[string]Entry {
	m := map[string]Entry{}
	for _, e := range entries {
		m[e.Key] = e
	}
	return m
}

func TestLoadPreservesUnavailableSourceReason(t *testing.T) {
	list := Load(context.Background(), Sources{
		Notes: []string{"winget unavailable: module search path"},
		Scan:  func() ([]detect.Entry, error) { return registry, nil },
	})
	if len(list.Entries) == 0 || len(list.Notes) != 1 || !strings.Contains(list.Notes[0], "module search path") {
		t.Fatalf("lost degraded-source reason: %+v", list)
	}
	for _, entry := range list.Entries {
		if entry.Update != "" {
			t.Fatal("reported update without source")
		}
	}
}

// TestLoadLastUsed gives each entry its last launch: by its directories,
// its app package or its scoop app.
func TestLoadLastUsed(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 11, d, 12, 0, 0, 0, time.UTC) }
	reg := []detect.Entry{
		{DisplayName: "Editor", Name: "Editor", Scope: "User", Arch: "X64", InstallLocation: `C:\Tools\Editor`},
		// No location: its icon's directory tells.
		{DisplayName: "Viewer", Name: "Viewer", Scope: "User", Arch: "X64", DisplayIcon: `"C:\Tools\Viewer\viewer.exe",0`},
		{DisplayName: "Unused", Name: "Unused", Scope: "User", Arch: "X64", InstallLocation: `C:\Tools\Unused`},
	}
	pkgs := []winget.Package{{ID: `MSIX\Microsoft.ScreenSketch_11.0.0.0_x64__8wekyb3d8bbwe`, Name: "Snipping Tool"}}
	for _, r := range reg {
		pkgs = append(pkgs, winget.Package{ID: `ARP\User\X64\` + r.Name, Name: r.DisplayName})
	}
	rec := usage.Record{At: day(9), Launches: []usage.Launch{
		{Path: `C:\Tools\Editor\editor.exe`, At: day(2)},
		{Path: `C:\Tools\Viewer\viewer.exe`, At: day(3)},
		{AppID: "Microsoft.ScreenSketch_8wekyb3d8bbwe!App", At: day(4)},
		{Path: `C:\Users\me\scoop\apps\ripgrep\current\rg.exe`, At: day(5)},
	}}
	list := Load(context.Background(), Sources{
		Winget: func(context.Context) ([]winget.Package, error) { return pkgs, nil },
		Scoop:  func(context.Context) ([]scoop.App, error) { return []scoop.App{{Name: "ripgrep", Version: "14"}}, nil },
		Scan:   func() ([]detect.Entry, error) { return reg, nil },
		Usage:  func(context.Context) (usage.Record, error) { return rec, nil },
	})
	if !list.UsageTracked {
		t.Error("launches recorded, but not tracked")
	}
	want := map[string]time.Time{"Editor": day(2), "Viewer": day(3), "Snipping Tool": day(4), "ripgrep": day(5), "Unused": {}}
	for _, e := range list.Entries {
		if w, ok := want[e.Name]; !ok || !e.LastUsed.Equal(w) {
			t.Errorf("%s: LastUsed = %v, want %v", e.Name, e.LastUsed, w)
		}
	}

	// Usage that fails says so, and the list stands.
	list = Load(context.Background(), Sources{
		Scan: func() ([]detect.Entry, error) { return reg, nil },
		Usage: func(context.Context) (usage.Record, error) {
			return usage.Record{}, errors.New("读取程序的打开记录失败：x")
		},
	})
	if len(list.Entries) != len(reg) || list.UsageTracked || !slices.Contains(list.Notes, "读取程序的打开记录失败：x") {
		t.Errorf("failed usage: %+v", list)
	}
}

func TestBuild(t *testing.T) {
	entries := Build(packages, true, scoopApps, registry)
	if len(entries) != len(packages)+len(scoopApps) {
		t.Fatalf("got %d entries, want %d: %+v", len(entries), len(packages)+len(scoopApps), entries)
	}
	m := byKey(entries)

	// A program of no source joins its registry key, in its own view.
	old := m[`arp:machine\x86\old tool`]
	if old.Kind != Local || old.Publisher != "Old Co" {
		t.Errorf("old tool = %+v", old)
	}
	zip := m[`arp:machine\x64\7-zip`]
	if zip.Publisher != "Igor Pavlov" || zip.Icon == "" || zip.DisplayName != "7-Zip 23.01 (x64)" {
		t.Errorf("7-zip = %+v", zip)
	}

	// Packages of the winget source join the entry of their name and
	// version; the same ID twice gets two keys.
	git := m["winget:git.git"]
	if git.Kind != Winget || git.WingetID != "Git.Git" || git.Update != "2.45.1" || git.Publisher != "The Git Development Community" {
		t.Errorf("git = %+v", git)
	}
	git2 := m["winget:git.git#2"]
	if git2.Version != "2.40.0" || git2.Publisher != "Another Git" {
		t.Errorf("second git = %+v", git2)
	}

	calc := m["msix:microsoft.windowscalculator_11.2405.2.0_x64__8wekyb3d8bbwe"]
	if calc.Kind != Store || calc.PackageFamily != "Microsoft.WindowsCalculator_8wekyb3d8bbwe" {
		t.Errorf("calculator = %+v", calc)
	}
	if e := m["msstore:9nblggh4nns1"]; e.Kind != Store {
		t.Errorf("msstore = %+v", e)
	}
	if e := m[`pkg:corp\foo.bar`]; e.Kind != Local {
		t.Errorf("other source = %+v", e)
	}
	rg := m["scoop:ripgrep"]
	if rg.Kind != Scoop || rg.Bucket != "main" || rg.ScoopName != "ripgrep" || rg.Update != "14.1.1" {
		t.Errorf("scoop = %+v", rg)
	}
	// winget lists the registry's programs: none comes twice.
	if _, ok := m[`arp:user\x64\only`]; ok {
		t.Error("registry entry listed although winget was read")
	}
	// Sorted by name.
	if !slices.IsSortedFunc(entries, func(a, b Entry) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	}) {
		t.Error("entries not sorted by name")
	}
}

func TestBuildWithoutWinget(t *testing.T) {
	entries := Build(nil, false, nil, registry)
	if len(entries) != len(registry) {
		t.Fatalf("got %d entries, want the registry's %d", len(entries), len(registry))
	}
	for _, e := range entries {
		if e.Kind != Local || !strings.HasPrefix(e.Key, "arp:") || e.DisplayName == "" || e.Reg == nil {
			t.Errorf("entry = %+v", e)
		}
	}
	// A program of the registry keeps its key once winget is read: winget
	// names it the same way.
	withWinget := map[string]bool{}
	for _, e := range Build(packages, true, nil, registry) {
		withWinget[e.Key] = true
	}
	for _, key := range []string{`arp:machine\x86\old tool`, `arp:machine\x64\7-zip`} {
		found := false
		for _, e := range entries {
			found = found || e.Key == key
		}
		if !found || !withWinget[key] {
			t.Errorf("key %s: registry only %v, with winget %v", key, found, withWinget[key])
		}
	}
}

func TestBuildCarriesRegistryFields(t *testing.T) {
	reg := []detect.Entry{{DisplayName: "Tool", Name: "Tool", Scope: "User", Arch: "X64", EstimatedSize: 2048, InstallDate: "2026-01-02", UninstallString: `"C:\Tool\uninst.exe"`}}
	pkgs := []winget.Package{{ID: `ARP\User\X64\Tool`, Name: "Tool"}, {ID: `MSIX\Pkg_1.0_x64__pub`, Name: "Pkg"}}
	byKey := map[string]Entry{}
	for _, e := range Build(pkgs, true, nil, reg) {
		byKey[e.Key] = e
	}
	tool := byKey[`arp:user\x64\tool`]
	if tool.Size != 2048 || tool.InstallDate != "2026-01-02" || tool.Reg == nil || tool.Reg.UninstallString != `"C:\Tool\uninst.exe"` {
		t.Errorf("tool = %+v", tool)
	}
	if p := byKey[`msix:pkg_1.0_x64__pub`]; p.PackageFullName != "Pkg_1.0_x64__pub" || p.PackageFamily != "Pkg_pub" {
		t.Errorf("msix = %+v", p)
	}
}

func TestMatch(t *testing.T) {
	entries := Build(packages, true, scoopApps, registry)
	apps := []catalog.App{
		{ID: "a_git", Name: "Git", Source: catalog.Source{Type: catalog.Winget, ID: "git.git"}},
		{ID: "a_rg", Name: "ripgrep", Source: catalog.Source{Type: catalog.Scoop, Bucket: "main", Name: "RipGrep"}},
		{ID: "a_zip", Name: "7-Zip", Source: catalog.Source{Type: catalog.Winget, ID: "7zip.7zip"}, Detect: &catalog.Detect{DisplayName: detect.SuggestRule("7-Zip 23.01 (x64)")}},
		{ID: "a_calc", Name: "Calc", Source: catalog.Source{Type: catalog.GitHub, Repo: "o/calc"}, Detect: &catalog.Detect{DisplayName: "^Windows Calculator$"}},
		{ID: "a_bad", Name: "Bad", Source: catalog.Source{Type: catalog.Winget, ID: "x.y"}, Detect: &catalog.Detect{DisplayName: "("}},
	}
	got := Match(entries, apps)
	want := map[string][]string{
		"winget:git.git":        {"a_git"},
		"winget:git.git#2":      {"a_git"},
		"scoop:ripgrep":         {"a_rg"},
		`arp:machine\x64\7-zip`: {"a_zip"},
		"msix:microsoft.windowscalculator_11.2405.2.0_x64__8wekyb3d8bbwe": {"a_calc"},
	}
	if len(got) != len(want) {
		t.Errorf("got %v, want %v", got, want)
	}
	for k, ids := range want {
		if !slices.Equal(got[k], ids) {
			t.Errorf("%s: got %v, want %v", k, got[k], ids)
		}
	}
}

func TestFamilyName(t *testing.T) {
	cases := map[string]string{
		"Microsoft.WindowsCalculator_11.2405.2.0_x64__8wekyb3d8bbwe": "Microsoft.WindowsCalculator_8wekyb3d8bbwe",
		"Contoso.App_1.0.0.0_neutral_~_abcdefghijkmn":                "Contoso.App_abcdefghijkmn",
		"nounderscores": "",
	}
	for in, want := range cases {
		if got := familyName(in); got != want {
			t.Errorf("familyName(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestLoadReal reads this machine: winget, scoop when it is there, and the
// registry. It changes nothing.
func TestLoadReal(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	w := winget.New()
	src := Sources{Scan: detect.Scan, Usage: usage.Read}
	wingetOK := false
	if av := w.Check(ctx); av.OK {
		wingetOK = true
		src.Winget = func(ctx context.Context) ([]winget.Package, error) {
			pkgs, _, err := w.Installed(ctx, 0)
			return pkgs, err
		}
	}
	s := scoop.New()
	if av := s.Check(ctx); av.OK {
		src.Scoop = s.Installed
	}
	list := Load(ctx, src)
	if len(list.Notes) > 0 {
		t.Logf("notes: %v", list.Notes)
	}
	if len(list.Entries) == 0 {
		t.Fatal("nothing installed found")
	}
	kinds := map[Kind]int{}
	joined, updates := 0, 0
	for _, e := range list.Entries {
		if e.Update != "" {
			updates++
		}
		kinds[e.Kind]++
		if e.DisplayName != "" && e.Publisher != "" {
			joined++
		}
	}
	t.Logf("%d entries %v; %d with the registry's name and publisher", len(list.Entries), kinds, joined)
	t.Logf("%d entries with source-reported updates", updates)
	used := 0
	for _, e := range list.Entries {
		if !e.LastUsed.IsZero() {
			used++
		}
	}
	t.Logf("%d entries with a last launch; Windows tracks launches: %v", used, list.UsageTracked)
	if list.UsageTracked && used == 0 {
		t.Error("launches tracked, but no entry has one")
	}
	if !wingetOK {
		t.Logf("winget not available: registry only")
		return
	}
	if kinds[Winget] == 0 {
		t.Error("no package of the winget source")
	}
	if joined == 0 {
		t.Error("no entry joined its registry key")
	}
	// Git is installed on the development machine; elsewhere it may not be.
	for _, e := range list.Entries {
		if strings.EqualFold(e.WingetID, "Git.Git") {
			t.Logf("Git: %+v", e)
			if e.Kind != Winget {
				t.Errorf("Git.Git kind = %s", e.Kind)
			}
		}
	}
}
