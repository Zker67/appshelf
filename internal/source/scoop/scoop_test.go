package scoop

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"appshelf/internal/catalog"
	"appshelf/internal/runner"
	"appshelf/internal/source"
)

// exportSample is scoop export's output as scoop-export.ps1 writes it:
// the buckets of list_buckets and the apps of scoop-list.ps1.
const exportSample = `{
  "buckets": [
    {"Name": "main", "Source": "https://github.com/ScoopInstaller/Main", "Updated": "2026-11-01T10:00:00+08:00", "Manifests": 1400},
    {"Name": "extras", "Source": "https://github.com/ScoopInstaller/Extras", "Updated": "2026-11-01T10:00:00+08:00", "Manifests": 2100}
  ],
  "apps": [
    {"Name": "git", "Version": "2.55.0", "Source": "main", "Updated": "2026-10-01T10:00:00+08:00", "Info": ""},
    {"Name": "7zip", "Version": "24.09", "Source": "main", "Updated": "2026-10-01T10:00:00+08:00", "Info": ""},
    {"Name": "vscode", "Version": "1.140.0", "Source": "versions", "Updated": "2026-10-01T10:00:00+08:00", "Info": "Global install"}
  ]
}`

func app(id, bucket, name string) catalog.App {
	return catalog.App{ID: id, Name: name, Source: catalog.Source{Type: catalog.Scoop, Bucket: bucket, Name: name}}
}

func TestBuildStatuses(t *testing.T) {
	exp, err := parseExport(exportSample)
	if err != nil {
		t.Fatal(err)
	}
	st := statusResult{Rows: []statusRow{
		{Name: "git", Installed: "2.55.0", Latest: "2.56.0"},
		{Name: "7zip", Installed: "24.09", Info: "Held package"},
	}}
	apps := []catalog.App{
		app("a_git", "main", "git"),
		app("a_7z", "main", "7zip"),
		app("a_code", "extras", "vscode"),
		app("a_obs", "extras", "obsidian"),
		app("a_my", "mine", "tool"),
	}
	got := buildStatuses(apps, exp, st, nil)
	if s := got["a_git"]; s.State != source.UpdateAvailable || s.Version != "2.55.0" || s.Latest != "2.56.0" {
		t.Errorf("git: %+v", s)
	}
	if s := got["a_7z"]; s.State != source.Installed || !strings.Contains(s.Note, "Held package") {
		t.Errorf("7zip: %+v", s)
	}
	if s := got["a_code"]; s.State != source.Installed || !strings.Contains(s.Note, "versions") {
		t.Errorf("vscode from another bucket: %+v", s)
	}
	if s := got["a_obs"]; s.State != source.NotInstalled || s.Note != "" {
		t.Errorf("obsidian: %+v", s)
	}
	if s := got["a_my"]; s.State != source.NotInstalled || !strings.Contains(s.Note, "mine") {
		t.Errorf("an app of a bucket not added: %+v", s)
	}

	failed := buildStatuses(apps[:1], exp, statusResult{}, errors.New("网络错误"))
	if s := failed["a_git"]; s.State != source.Installed || !strings.Contains(s.Note, "检查更新失败") {
		t.Errorf("status failed: %+v", s)
	}
}

func TestParseExportRejectsOldFormat(t *testing.T) {
	if _, err := parseExport("git (2.55.0) [main]\n7zip (24.09) [main]\n"); err == nil {
		t.Error("the text format of old scoop versions parsed")
	}
	if _, err := parseExport("  "); err == nil {
		t.Error("empty output parsed")
	}
}

// The tests below run scoop for real, read-only, where it is installed.

func available(t *testing.T) *Source {
	t.Helper()
	s := New()
	if av := s.Check(context.Background()); !av.OK {
		t.Skipf("scoop not available: %s", av.Reason)
	}
	return s
}

// TestCheckReal checks scoop on this machine, which only reads: it finds
// scoop, or says it is missing and whether AppShelf can install it.
func TestCheckReal(t *testing.T) {
	av := New().Check(context.Background())
	switch {
	case av.OK:
		if av.Fixable {
			t.Errorf("available scoop is fixable: %+v", av)
		}
	case strings.HasPrefix(av.Reason, "未安装 scoop"):
	default:
		t.Errorf("check failed: %+v", av)
	}
}

// TestInstallScriptParses parses the script that installs scoop, without
// running it.
func TestInstallScriptParses(t *testing.T) {
	const parse = `
$errors = $null
[void][System.Management.Automation.Language.Parser]::ParseInput([string]$P.Script, [ref]$null, [ref]$errors)
Write-Result @{ Errors = @($errors | ForEach-Object { $_.Message }) }
`
	var out struct{ Errors []string }
	if _, err := runner.PowerShell(context.Background(), parse, map[string]any{"Script": installScoopScript}, &out, runner.Options{}); err != nil {
		t.Fatal(err)
	}
	if len(out.Errors) > 0 {
		t.Fatalf("install script: %v", out.Errors)
	}
}

func TestStatusesReal(t *testing.T) {
	s := available(t)
	got, err := s.Statuses(context.Background(), []catalog.App{app("a_none", "main", "appshelf-no-such-app")})
	if err != nil {
		t.Fatal(err)
	}
	if st := got["a_none"]; st.State != source.NotInstalled {
		t.Errorf("missing app: %+v", st)
	}
	st, err := s.Recheck(context.Background(), app("a_none", "main", "appshelf-no-such-app"))
	if err != nil || st.State != source.NotInstalled {
		t.Errorf("recheck: %+v %v", st, err)
	}
}

func TestSearchReal(t *testing.T) {
	s := available(t)
	got, err := s.Search(context.Background(), "7zip")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range got {
		if r.Name == "7zip" && r.ID == r.Source+"/7zip" {
			return
		}
	}
	t.Errorf("7zip not found in %+v", got)
}

// TestInstallReal installs a real app: only with APPSHELF_INSTALL_TEST=1
// and APPSHELF_TEST_SCOOP_APP as bucket/name, chosen by the user.
func TestInstallReal(t *testing.T) {
	spec := os.Getenv("APPSHELF_TEST_SCOOP_APP")
	bucket, name, ok := strings.Cut(spec, "/")
	if os.Getenv("APPSHELF_INSTALL_TEST") != "1" || !ok {
		t.Skip("set APPSHELF_INSTALL_TEST=1 and APPSHELF_TEST_SCOOP_APP=bucket/name to install an app")
	}
	s := available(t)
	a := app("a_test", bucket, name)
	ctx := context.Background()
	if _, err := s.Install(ctx, a, os.Stdout); err != nil {
		t.Fatal(err)
	}
	st, err := s.Recheck(ctx, a)
	if err != nil || !st.IsInstalled() {
		t.Fatalf("after installing: %+v %v", st, err)
	}
}

func TestInstalledAppsGlobal(t *testing.T) {
	exp, err := parseExport(exportSample)
	if err != nil {
		t.Fatal(err)
	}
	apps := installedApps(exp, statusResult{Rows: []statusRow{{Name: "git", Installed: "2.55.0", Latest: "2.56.0"}}})
	got := map[string]App{}
	for _, a := range apps {
		got[a.Name] = a
	}
	if got["vscode"].Global != true || got["git"].Global || got["git"].Update != "2.56.0" {
		t.Errorf("apps: %+v", apps)
	}
}

// stageRecorder is a log that keeps the stages reported.
type stageRecorder struct {
	strings.Builder
	stages []string
}

func (r *stageRecorder) ReportProgress(p source.Progress) {
	if n := len(r.stages); n == 0 || r.stages[n-1] != p.Stage {
		r.stages = append(r.stages, p.Stage)
	}
}

func TestProgressFollowsScoopsSteps(t *testing.T) {
	rec := &stageRecorder{}
	p := newProgress(rec, "update", "git", "")
	// scoop update git, as scoop 0.5 writes it.
	out := "Updating one outdated app:\r\nUpdating 'git' (2.45.0 -> 2.45.1)\nDownloading new version\n" +
		"PortableGit-2.45.1-64-bit.7z.exe (61.5 MB) [====] 100%\n" +
		"Checking hash of PortableGit-2.45.1-64-bit.7z.exe ... ok.\nUninstalling 'git' (2.45.0)\nRemoving shim 'git.shim'.\n" +
		"Installing 'git' (2.45.1) [64bit] from 'main' bucket\nExtracting PortableGit-2.45.1-64-bit.7z.exe ... done.\n" +
		`Linking ~\scoop\apps\git\current => ~\scoop\apps\git\2.45.1` + "\nCreating shim for 'git'.\n'git' (2.45.1) was installed successfully!\n"
	io.WriteString(p, out)
	want := []string{"正在准备", "正在更新", "正在下载", "正在校验", "正在移除旧版本", "正在安装新版本", "正在解压", "正在完成"}
	if strings.Join(rec.stages, ",") != strings.Join(want, ",") {
		t.Fatalf("stages = %q, want %q", rec.stages, want)
	}
	if rec.String() != out {
		t.Fatal("the output must reach the log")
	}
}
