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
