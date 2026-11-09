package detect

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"appshelf/internal/catalog"
)

func TestMatch(t *testing.T) {
	entries := []Entry{
		{DisplayName: "Git", DisplayVersion: "2.55.0.5"},
		{DisplayName: "Microsoft Visual Studio Code (User)", DisplayVersion: "1.140.0"},
		{DisplayName: "Microsoft Visual Studio Code", DisplayVersion: "1.143.1"},
	}
	r, err := Match(catalog.Detect{DisplayName: "^microsoft visual studio code"}, entries)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Installed || r.Version != "1.143.1" {
		t.Errorf("highest version not taken: %+v", r)
	}
	r, _ = Match(catalog.Detect{DisplayName: "^Git$"}, entries)
	if !r.Installed || r.Version != "2.55.0.5" {
		t.Errorf("git: %+v", r)
	}
	r, _ = Match(catalog.Detect{DisplayName: "^Figma"}, entries)
	if r.Installed {
		t.Errorf("figma matched: %+v", r)
	}
	if _, err := Match(catalog.Detect{DisplayName: "["}, entries); err == nil {
		t.Error("an invalid rule matched")
	}

	dir := t.TempDir()
	exe := filepath.Join(dir, "tool.exe")
	os.WriteFile(exe, []byte("x"), 0o644)
	t.Setenv("APPSHELF_TEST_DIR", dir)
	r, _ = Match(catalog.Detect{DisplayName: "^Nothing", ExePath: `%APPSHELF_TEST_DIR%\tool.exe`}, entries)
	if runtime.GOOS == "windows" && !r.Installed {
		t.Errorf("exe path not found: %+v", r)
	}
	r, _ = Match(catalog.Detect{ExePath: `%APPSHELF_TEST_DIR%\missing.exe`}, entries)
	if r.Installed {
		t.Errorf("a missing exe matched: %+v", r)
	}
}

func TestExpandEnv(t *testing.T) {
	t.Setenv("APPSHELF_X", `C:\X`)
	if got := ExpandEnv(`%APPSHELF_X%\a\%NO_SUCH_VAR_123%`); got != `C:\X\a\%NO_SUCH_VAR_123%` {
		t.Errorf("got %q", got)
	}
}

// TestScan reads this machine's registry: there are always programs
// listed, and Git, which builds this project's tools, is one of them when
// installed by its installer.
func TestScan(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the registry is Windows'")
	}
	entries, err := Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("no programs found")
	}
	for _, e := range entries {
		if e.DisplayName == "" || e.Key == "" {
			t.Fatalf("incomplete entry %+v", e)
		}
	}
	t.Logf("%d programs", len(entries))

	// Git for Windows' installer registers "Git": when git runs from its
	// installation, the rule must find it.
	git, err := exec.LookPath("git")
	if err != nil || !strings.Contains(strings.ToLower(git), `\program files\git\`) {
		t.Logf("git not installed by its installer (%q), skipping the Git check", git)
		return
	}
	r, err := Match(catalog.Detect{DisplayName: "^Git$"}, entries)
	if err != nil || !r.Installed || r.Version == "" {
		t.Errorf("Git is installed at %s but the rule found %+v (%v)", git, r, err)
	}
}

// TestScanReadsUninstallFields reads the fields uninstalling needs, and one
// key again with Lookup.
func TestScanReadsUninstallFields(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the registry is Windows'")
	}
	entries, err := Scan()
	if err != nil {
		t.Fatal(err)
	}
	var withCmd, withSize, withDate, guessed int
	var sample Entry
	for _, e := range entries {
		if e.UninstallString != "" {
			withCmd++
			if sample.Name == "" {
				sample = e
			}
		}
		if e.EstimatedSize > 0 {
			withSize++
		}
		if e.InstallDate != "" {
			withDate++
			if len(e.InstallDate) != 10 || e.InstallDate[4] != '-' {
				t.Errorf("install date %q", e.InstallDate)
			}
		}
		if e.LocationGuessed {
			guessed++
			if guessed <= 12 {
				t.Logf("guessed %s → %s", e.DisplayName, e.InstallLocation)
			}
			if !OwnDirectory(e.InstallLocation) {
				t.Errorf("guessed a shared directory %q for %s", e.InstallLocation, e.DisplayName)
			}
		}
	}
	t.Logf("%d programs: %d with an uninstall command, %d sizes, %d dates, %d locations guessed", len(entries), withCmd, withSize, withDate, guessed)
	if withCmd == 0 {
		t.Fatal("no program has an uninstall command")
	}
	got, ok, err := Lookup(sample.Scope, sample.Arch, sample.Name)
	if err != nil || !ok || got.DisplayName != sample.DisplayName || got.UninstallString != sample.UninstallString {
		t.Fatalf("Lookup(%s %s %s) = %+v %v %v", sample.Scope, sample.Arch, sample.Name, got, ok, err)
	}
	if _, ok, err := Lookup("User", "X64", "AppShelf-no-such-key"); ok || err != nil {
		t.Fatalf("a missing key: %v %v", ok, err)
	}
	if _, _, err := Lookup("User", "X64", `a\b`); err == nil {
		t.Fatal("a name with a backslash must be refused")
	}
}

func TestInstallDate(t *testing.T) {
	for in, want := range map[string]string{"20260131": "2026-01-31", "2026013": "", "20261301": "", "abcdefgh": "", "18000101": "", "": ""} {
		if got := installDate(in); got != want {
			t.Errorf("installDate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCommandFile(t *testing.T) {
	dir := t.TempDir()
	spaced := filepath.Join(dir, "My App", "uninst.exe")
	os.MkdirAll(filepath.Dir(spaced), 0o755)
	os.WriteFile(spaced, nil, 0o644)
	for in, want := range map[string]string{
		`"C:\Program Files\App\unins000.exe" /SILENT`: `C:\Program Files\App\unins000.exe`,
		spaced + " /S":                     spaced,
		`MsiExec.exe /X{1234}`:             `MsiExec.exe`,
		`C:\App\app.exe,0`:                 `C:\App\app.exe`,
		`C:\App\app.ico,0`:                 `C:\App\app.ico`,
		`rundll32.exe shell32.dll,Control`: `rundll32.exe`,
		`   `:                              "",
		`"C:\unterminated`:                 `C:\unterminated`,
	} {
		if got := CommandFile(in); got != want {
			t.Errorf("CommandFile(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOwnDirectory(t *testing.T) {
	pf := os.Getenv("ProgramFiles")
	if pf == "" {
		t.Skip("no ProgramFiles")
	}
	for dir, want := range map[string]bool{
		pf:                       false,
		filepath.Join(pf, "App"): true,
		`C:\`:                    false,
		filepath.Join(os.Getenv("SystemRoot"), "System32"): false,
		`C:\ProgramData\Package Cache\{X}`:                 false,
		`relative\dir`:                                     false,
	} {
		if got := OwnDirectory(dir); got != want {
			t.Errorf("OwnDirectory(%q) = %v, want %v", dir, got, want)
		}
	}
}
