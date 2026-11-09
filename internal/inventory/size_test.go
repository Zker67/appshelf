package inventory

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"appshelf/internal/detect"
	"appshelf/internal/source/winget"
)

// TestBuildLooseName joins the packages winget names after their manifest
// to the registry entry their name begins, or that begins theirs, of their
// version.
func TestBuildLooseName(t *testing.T) {
	reg := []detect.Entry{
		{DisplayName: "Everything 1.4.1.1032 (x64)", DisplayVersion: "1.4.1.1032", Name: "Everything", Scope: "Machine", Arch: "X64", EstimatedSize: 3 << 20},
		{DisplayName: "Python 3.14.10 (64-bit)", DisplayVersion: "3.14.10150.0", Name: "{py}", Scope: "User", Arch: "X64", EstimatedSize: 100 << 20},
		{DisplayName: "Google Chrome", DisplayVersion: "130.0", Name: "Google Chrome", Scope: "Machine", Arch: "X64", EstimatedSize: 400 << 20},
		{DisplayName: "GitHub Desktop", DisplayVersion: "2.0", Name: "GitHubDesktop", Scope: "User", Arch: "X64", EstimatedSize: 1},
		{DisplayName: "Tool Pro 5", DisplayVersion: "5", Name: "ToolPro", Scope: "User", Arch: "X64", EstimatedSize: 1},
		{DisplayName: "Editor 2", DisplayVersion: "2.1", Name: "Editor", Scope: "User", Arch: "X64", EstimatedSize: 1},
	}
	pkgs := []winget.Package{
		{ID: "voidtools.Everything", Name: "Everything", Version: "1.4.1.1032", Source: "winget"},
		{ID: "Python.Python.3.14", Name: "Python 3.14", Version: "3.14.10", Source: "winget"},
		{ID: "Google.Chrome.EXE", Name: "Google Chrome (EXE)", Version: "130.0", Source: "winget"},
		// A name of other words, or of another version, joins nothing.
		{ID: "Git.Git", Name: "Git", Version: "2.0", Source: "winget"},
		{ID: "Some.Editor", Name: "Editor", Version: "3.0", Source: "winget"},
		// The package of the key keeps its entry, though listed after.
		{ID: "Tool.Tool", Name: "Tool", Version: "5", Source: "winget"},
		{ID: `ARP\User\X64\ToolPro`, Name: "Tool Pro 5", Version: "5"},
	}
	m := byKey(Build(pkgs, true, nil, reg))
	for key, size := range map[string]int64{
		"winget:voidtools.everything": 3 << 20,
		"winget:python.python.3.14":   100 << 20,
		"winget:google.chrome.exe":    400 << 20,
		`arp:user\x64\toolpro`:        1,
	} {
		if e := m[key]; e.Size != size || e.Reg == nil {
			t.Errorf("%s: size %d, want %d: %+v", key, e.Size, size, e)
		}
	}
	for _, key := range []string{"winget:git.git", "winget:some.editor", "winget:tool.tool"} {
		if e := m[key]; e.Reg != nil {
			t.Errorf("%s joined %q", key, e.Reg.DisplayName)
		}
	}
}

func writeFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDirSize(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.bin"), 100)
	writeFile(t, filepath.Join(dir, "sub", "b.bin"), 50)
	if n := DirSize(dir); n != 150 {
		t.Errorf("DirSize = %d, want 150", n)
	}
	if n := DirSize(filepath.Join(dir, "missing")); n != 0 {
		t.Errorf("missing dir: %d", n)
	}
	if runtime.GOOS != "windows" {
		return
	}
	// A junction, as scoop's persisted data, is not followed.
	other := t.TempDir()
	writeFile(t, filepath.Join(other, "big.bin"), 1000)
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", filepath.Join(dir, "link"), other).CombinedOutput(); err != nil {
		t.Fatalf("mklink: %v %s", err, out)
	}
	if n := DirSize(dir); n != 150 {
		t.Errorf("with a junction, DirSize = %d, want 150", n)
	}
}

// TestSizes gives the Store's and scoop's entries the size of their
// directory, measuring a directory once.
func TestSizes(t *testing.T) {
	pkgDir := t.TempDir()
	writeFile(t, filepath.Join(pkgDir, "app.exe"), 300)
	scoopRoot := t.TempDir()
	t.Setenv("SCOOP", scoopRoot)
	writeFile(t, filepath.Join(scoopRoot, "apps", "rg", "14.1.0", "rg.exe"), 70)
	writeFile(t, filepath.Join(scoopRoot, "apps", "rg", "13.0.0", "rg.exe"), 9)

	var mu sync.Mutex
	measured := map[string]int{}
	s := &Sizes{
		dirSize: func(d string) int64 {
			mu.Lock()
			measured[d]++
			mu.Unlock()
			return DirSize(d)
		},
		packagePath: func(full string) string {
			if full == "App_1.0_x64__pub" {
				return pkgDir
			}
			return ""
		},
	}
	entries := func() []Entry {
		return []Entry{
			{Key: "msix", Kind: Store, PackageFullName: "App_1.0_x64__pub"},
			{Key: "gone", Kind: Store, PackageFullName: "Gone_1.0_x64__pub"},
			{Key: "msstore", Kind: Store, StoreID: "9N"},
			{Key: "scoop", Kind: Scoop, ScoopName: "rg", Version: "14.1.0", Scope: "User"},
			{Key: "reg", Kind: Winget, Size: 42, Location: pkgDir},
			{Key: "local", Kind: Local, Location: pkgDir},
		}
	}
	want := map[string]int64{"msix": 300, "gone": 0, "msstore": 0, "scoop": 70, "reg": 42, "local": 0}
	for round := range 2 {
		got := entries()
		s.set(got)
		for _, e := range got {
			if e.Size != want[e.Key] {
				t.Errorf("round %d, %s: size %d, want %d", round, e.Key, e.Size, want[e.Key])
			}
		}
	}
	if len(measured) != 2 {
		t.Errorf("measured %v, want the package's and scoop's version directories", measured)
	}
	for d, n := range measured {
		if n != 1 {
			t.Errorf("%s measured %d times", d, n)
		}
	}
}

func TestScoopVersionDir(t *testing.T) {
	app := t.TempDir()
	writeFile(t, filepath.Join(app, "2.0", "x"), 1)
	if d := scoopVersionDir(app, "2.0"); d != filepath.Join(app, "2.0") {
		t.Errorf("version dir = %q", d)
	}
	if d := scoopVersionDir(app, "3.0"); d != "" {
		t.Errorf("no such version, no current: %q", d)
	}
	if runtime.GOOS != "windows" {
		return
	}
	// A version scoop names otherwise: what "current" links to.
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", filepath.Join(app, "current"), filepath.Join(app, "2.0")).CombinedOutput(); err != nil {
		t.Fatalf("mklink: %v %s", err, out)
	}
	if d := scoopVersionDir(app, "nightly"); !strings.EqualFold(filepath.Clean(d), filepath.Join(app, "2.0")) {
		t.Errorf("current = %q", d)
	}
}

// TestPackagePathReal finds the directory of an app package of this
// machine: the Calculator's, when installed.
func TestPackagePathReal(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	if p := PackagePath("Nobody.Nothing_1.0.0.0_x64__0000000000000"); p != "" {
		t.Errorf("unknown package: %q", p)
	}
	out, err := exec.Command("powershell", "-NoProfile", "-Command", "(Get-AppxPackage Microsoft.WindowsCalculator | Select-Object -First 1).PackageFullName").Output()
	full := strings.TrimSpace(string(out))
	if err != nil || full == "" {
		t.Skip("Calculator not installed")
	}
	p := PackagePath(full)
	if p == "" || !isDir(p) {
		t.Fatalf("PackagePath(%s) = %q", full, p)
	}
	if n := DirSize(p); n == 0 {
		t.Errorf("%s: size 0", p)
	}
	t.Logf("%s: %s", full, p)
}
