package winget

import (
	"context"
	"os"
	"strings"
	"testing"

	"appshelf/internal/catalog"
	"appshelf/internal/runner"
	"appshelf/internal/source"
)

// These tests read this machine's winget for real, without changing
// anything; they skip where winget or the module is missing.

func available(t *testing.T) *Source {
	t.Helper()
	s := New()
	if av := s.Check(context.Background()); !av.OK {
		t.Skipf("winget not available: %s", av.Reason)
	}
	return s
}

func app(id string) catalog.App {
	return catalog.App{ID: "a_" + strings.ToLower(strings.ReplaceAll(id, ".", "")), Name: id, Source: catalog.Source{Type: catalog.Winget, ID: id}}
}

// installedWingetID finds a package installed on this machine that winget
// knows from its source, to check that Statuses sees it.
func installedWingetID(t *testing.T) (id, version string) {
	t.Helper()
	var rows []row
	_, err := runner.PowerShell(context.Background(), rowScript+`
$rows = @(Get-WinGetPackage | Where-Object { $_.Source -eq 'winget' } | Select-Object -First 1 | ForEach-Object { ConvertTo-Row $_ })
Write-Result $rows`, nil, &rows, runner.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Skip("no package from the winget source is installed")
	}
	return rows[0].ID, rows[0].InstalledVersion
}

func TestStatuses(t *testing.T) {
	s := available(t)
	id, ver := installedWingetID(t)
	installed := app(id)
	// The ID in another case still reads as installed.
	other := app(strings.ToUpper(id))
	other.ID = "a_upper"
	missing := app("AppShelf.Test.NoSuchPackage")
	got, err := s.Statuses(context.Background(), []catalog.App{installed, other, missing})
	if err != nil {
		t.Fatal(err)
	}
	if st := got[installed.ID]; !st.IsInstalled() || (displayVersion(ver) != "" && st.Version == "") {
		t.Errorf("%s (%s): %+v", id, ver, st)
	}
	if st := got[other.ID]; !st.IsInstalled() {
		t.Errorf("%s in upper case: %+v", id, st)
	}
	if st := got[missing.ID]; st.State != source.NotInstalled {
		t.Errorf("missing package: %+v", st)
	}
}

func TestRecheck(t *testing.T) {
	s := available(t)
	st, err := s.Recheck(context.Background(), app("AppShelf.Test.NoSuchPackage"))
	if err != nil || st.State != source.NotInstalled {
		t.Errorf("missing package: %+v %v", st, err)
	}
	id, _ := installedWingetID(t)
	st, err = s.Recheck(context.Background(), app(strings.ToLower(id)))
	if err != nil || !st.IsInstalled() {
		t.Errorf("%s in lower case: %+v %v", id, st, err)
	}
}

func TestSearch(t *testing.T) {
	s := available(t)
	got, err := s.Search(context.Background(), "Visual Studio Code")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range got {
		if r.ID == "Microsoft.VisualStudioCode" && r.Source == "winget" {
			found = true
		}
	}
	if !found {
		t.Errorf("Microsoft.VisualStudioCode not found in %+v", got)
	}
	if got, err := s.Search(context.Background(), "-Name x"); err != nil {
		t.Errorf("a query starting with a dash: %v (%v)", err, got)
	}
}

func TestInterpret(t *testing.T) {
	cases := []struct {
		code    uint32
		already bool
		reboot  bool
		fails   bool
	}{
		{0, false, false, false},
		{codeAlreadyInstalled, true, false, false},
		{codeRebootToFinish, false, true, false},
		{codeNoPackageFound, false, false, true},
		{0x80070005, false, false, true},
	}
	for _, tc := range cases {
		res, err := interpret(runner.Result{ExitCode: int(tc.code)})
		if res.AlreadyInstalled != tc.already || res.RebootRequired != tc.reboot || (err != nil) != tc.fails {
			t.Errorf("0x%08X: %+v %v", tc.code, res, err)
		}
	}
}

// TestInstall installs a real package: it runs only with
// APPSHELF_INSTALL_TEST=1 and the package in APPSHELF_TEST_WINGET_ID,
// which the user chose for it.
func TestInstall(t *testing.T) {
	id := os.Getenv("APPSHELF_TEST_WINGET_ID")
	if os.Getenv("APPSHELF_INSTALL_TEST") != "1" || id == "" {
		t.Skip("set APPSHELF_INSTALL_TEST=1 and APPSHELF_TEST_WINGET_ID to install a package")
	}
	s := available(t)
	a := app(id)
	ctx := context.Background()
	if _, err := s.Install(ctx, a, os.Stdout); err != nil {
		t.Fatal(err)
	}
	st, err := s.Recheck(ctx, a)
	if err != nil || !st.IsInstalled() {
		t.Fatalf("after installing: %+v %v", st, err)
	}
	res, err := s.Install(ctx, a, os.Stdout)
	if err != nil || !res.AlreadyInstalled {
		t.Errorf("installing again: %+v %v", res, err)
	}
}
