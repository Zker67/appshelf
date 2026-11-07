package winget

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

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
	vs, err := s.SearchVersions(context.Background(), "Visual Studio Code")
	if err != nil || vs["microsoft.visualstudiocode"] == "" {
		t.Errorf("versions: %v, %v", vs, err)
	}
	if _, err := s.SearchVersions(context.Background(), "-Name x"); err != nil {
		t.Errorf("versions of a query starting with a dash: %v", err)
	}
}

// TestSearchQuick searches for words that find many packages, which took
// half a minute when the search read each package's version.
func TestSearchQuick(t *testing.T) {
	s := available(t)
	t.Cleanup(func() { s.searcher().Close() })
	ctx := context.Background()
	start := time.Now()
	if err := s.Warm(ctx); err != nil {
		t.Fatal(err)
	}
	t.Logf("warm: %v", time.Since(start))
	for _, q := range []string{"player", "manager", "tool"} {
		start := time.Now()
		got, err := s.Search(ctx, q)
		took := time.Since(start)
		if err != nil || len(got) < 20 {
			t.Fatalf("%s: %d results, %v", q, len(got), err)
		}
		start = time.Now()
		vs, err := s.SearchVersions(ctx, q)
		vtook := time.Since(start)
		n := 0
		for _, r := range got {
			if vs[strings.ToLower(r.ID)] != "" {
				n++
			}
		}
		t.Logf("%s: %d results in %v; versions of %d in %v", q, len(got), took, n, vtook)
		if err != nil || n < len(got)*8/10 {
			t.Errorf("%s: versions of %d of %d results, %v", q, n, len(got), err)
		}
		if took > 5*time.Second || vtook > 10*time.Second {
			t.Errorf("%s: search %v, versions %v", q, took, vtook)
		}
	}
}

func TestParseSearchTable(t *testing.T) {
	out := "\r   - \r   \\ \r" +
		"名称            ID                     版本          匹配\r\n" +
		"------------------------------------------------------------------\r\n" +
		"MDX Notes       maqibin.MDXNotes       1.0.3         Tag: 微信\r\n" +
		"企业微信        Tencent.WeCom          5.0.13.6005   Tag: 微信\r\n" +
		"微信开发者工具  Tencent.WeixinDevT…    2.02.2609250  Tag: 微信\r\n" +
		"Node.js         OpenJS.NodeJS          25.1.0\r\n" +
		"\x1b[2KQuanshi Meeting Quanshi.QuanshiMeeting 6.26.25011003\r\n" +
		"<由于结果限制而截断了其他条目>\r\n"
	got := parseSearchTable([]byte(out))
	for id, v := range map[string]string{
		"maqibin.mdxnotes": "1.0.3", "tencent.wecom": "5.0.13.6005", "openjs.nodejs": "25.1.0",
		"quanshi.quanshimeeting": "6.26.25011003",
	} {
		if got[id] != v {
			t.Errorf("%s = %q, want %q", id, got[id], v)
		}
	}
	for k := range got {
		if strings.Contains(k, "…") {
			t.Errorf("a shortened ID: %q", k)
		}
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

func TestInterpretSaysWhy(t *testing.T) {
	_, err := interpret(runner.Result{ExitCode: int(uint32(0x8A150105))})
	if err == nil || !strings.Contains(err.Error(), "磁盘空间不足") || !strings.Contains(err.Error(), "0x8A150105") {
		t.Errorf("disk full: %v", err)
	}
	_, err = interpret(runner.Result{ExitCode: int(uint32(0x8A150114))})
	if err == nil || !strings.Contains(err.Error(), "不支持直接更新") {
		t.Errorf("upgrade not supported: %v", err)
	}
}
