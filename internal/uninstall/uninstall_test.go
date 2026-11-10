package uninstall

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"appshelf/internal/detect"
)

func regTarget(e detect.Entry) Target {
	if e.Scope == "" {
		e.Scope, e.Arch = "Machine", "X64"
	}
	if e.Name == "" {
		e.Name = "App"
	}
	return Target{Key: "arp:x", Name: e.DisplayName, Method: ByRegistry, Scope: e.Scope, Reg: &e, Location: e.InstallLocation}
}

func TestPlanForInstallers(t *testing.T) {
	dir := t.TempDir()
	inno := filepath.Join(dir, "Inno App", "unins000.exe")
	os.MkdirAll(filepath.Dir(inno), 0o755)
	os.WriteFile(inno, []byte("MZ"), 0o644)
	os.WriteFile(strings.TrimSuffix(inno, ".exe")+".dat", nil, 0o644)
	innoNoDat := filepath.Join(dir, "Other", "unins000.exe")
	os.MkdirAll(filepath.Dir(innoNoDat), 0o755)
	os.WriteFile(innoNoDat, []byte("MZ"), 0o644)
	nsis := filepath.Join(dir, "NSIS App", "uninstall.exe")
	os.MkdirAll(filepath.Dir(nsis), 0o755)
	os.WriteFile(nsis, []byte("MZ...<assemblyIdentity name=\"Nullsoft.NSIS.exehead\"/>"), 0o644)
	plain := filepath.Join(dir, "Plain", "remove.exe")
	os.MkdirAll(filepath.Dir(plain), 0o755)
	os.WriteFile(plain, []byte("MZ"), 0o644)

	code := "{12345678-1234-1234-1234-1234567890AB}"
	cases := []struct {
		name        string
		e           detect.Entry
		interactive bool
		kind        Kind
		file        string // base name of the file run
		params      string
		quiet       bool
		canRetry    bool
	}{
		{"msi by key name", detect.Entry{Name: strings.ToLower(code), WindowsInstaller: true, UninstallString: "MsiExec.exe /I" + code}, false, MSI, "msiexec.exe", "/x " + code + " /qn /norestart", true, true},
		{"msi by command", detect.Entry{Name: "App", UninstallString: "MsiExec.exe /X" + code}, false, MSI, "msiexec.exe", "/x " + code + " /qn /norestart", true, true},
		{"msi interactive", detect.Entry{Name: code, WindowsInstaller: true}, true, MSI, "msiexec.exe", "/x " + code, false, true},
		{"inno", detect.Entry{UninstallString: `"` + inno + `"`}, false, Inno, "unins000.exe", "/VERYSILENT /SUPPRESSMSGBOXES /NORESTART", true, true},
		{"inno before its quiet string", detect.Entry{UninstallString: `"` + inno + `"`, QuietUninstallString: `"` + inno + `" /SILENT`}, false, Inno, "unins000.exe", "/VERYSILENT /SUPPRESSMSGBOXES /NORESTART", true, true},
		{"inno interactive", detect.Entry{UninstallString: `"` + inno + `"`}, true, Inno, "unins000.exe", "", false, false},
		{"unins without its log is no inno", detect.Entry{UninstallString: `"` + innoNoDat + `"`}, false, Other, "unins000.exe", "", false, false},
		{"nsis keeps its arguments, /S before _?=", detect.Entry{UninstallString: `"` + nsis + `" _?=C:\x`}, false, NSIS, "uninstall.exe", `/S _?=C:\x`, true, true},
		{"nsis with arguments of its own", detect.Entry{UninstallString: `"` + nsis + `" /currentuser`}, false, NSIS, "uninstall.exe", `/currentuser /S`, true, true},
		{"quiet string first", detect.Entry{UninstallString: `"` + plain + `"`, QuietUninstallString: `"` + plain + `" /quiet`}, false, Other, "remove.exe", "/quiet", true, true},
		{"burn bundle", detect.Entry{UninstallString: `"C:\ProgramData\Package Cache\{A}\setup.exe" /uninstall`}, false, Burn, "setup.exe", "/uninstall /quiet /norestart", true, true},
		{"other opens its window", detect.Entry{UninstallString: plain + " /remove"}, false, Other, "remove.exe", "/remove", false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.e.DisplayName = "App"
			p := PlanFor(regTarget(c.e), c.interactive, Options{})
			if p.Blocked != "" {
				t.Fatalf("blocked: %s", p.Blocked)
			}
			if p.Kind != c.kind || !strings.EqualFold(filepath.Base(p.File), c.file) || p.Params != c.params || p.Quiet != c.quiet || p.Interactive != c.canRetry {
				t.Fatalf("plan = %+v", p)
			}
			if !c.quiet && !slices.Contains(p.Notes, "会打开卸载程序的窗口，请在窗口里完成卸载") {
				t.Errorf("no note that a window opens: %q", p.Notes)
			}
		})
	}
}

// TestPlanForMSIElevation: msiexec without its window cannot ask for
// administrator rights, so a program for all users asks first.
func TestPlanForMSIElevation(t *testing.T) {
	code := "{12345678-1234-1234-1234-1234567890AB}"
	cases := []struct {
		scope       string
		interactive bool
		elevate     bool
	}{
		{"Machine", false, true},
		{"User", false, false},
		{"Machine", true, false},
	}
	for _, c := range cases {
		p := PlanFor(regTarget(detect.Entry{DisplayName: "App", Name: code, Scope: c.scope, Arch: "X64", WindowsInstaller: true}), c.interactive, Options{})
		if p.Kind != MSI || p.Elevate != c.elevate {
			t.Errorf("%s, interactive %v: plan = %+v", c.scope, c.interactive, p)
		}
	}
}

func TestPlanForBlocks(t *testing.T) {
	self := t.TempDir()
	cases := map[string]Target{
		"no remove":   regTarget(detect.Entry{DisplayName: "A", NoRemove: true, UninstallString: "x.exe"}),
		"no command":  regTarget(detect.Entry{DisplayName: "A"}),
		"no key":      {Name: "A", Method: ByRegistry},
		"msi no code": regTarget(detect.Entry{DisplayName: "A", Name: "App", WindowsInstaller: true}),
		"winget":      {Name: "App Installer", Method: ByWinget, WingetID: "Microsoft.AppInstaller"},
		"winget msix": {Name: "App Installer", Method: ByStore, PackageFullName: "Microsoft.DesktopAppInstaller_1.0.0.0_x64__8wekyb3d8bbwe"},
		"scoop":       {Name: "scoop", Method: ByScoop, ScoopName: "scoop"},
		"itself":      {Name: "AppShelf", Method: ByRegistry, Location: self + `\`, Reg: &detect.Entry{UninstallString: "x.exe"}},
		"no method":   {Name: "A"},
	}
	for name, tg := range cases {
		if p := PlanFor(tg, false, Options{SelfDir: self}); p.Blocked == "" {
			t.Errorf("%s: not blocked: %+v", name, p)
		}
	}
	// A WindowsInstaller program whose key is no GUID but whose command
	// names msiexec is MSI with the command's code.
	e := detect.Entry{DisplayName: "A", Name: "App", WindowsInstaller: true, UninstallString: "MsiExec.exe /X{12345678-1234-1234-1234-1234567890ab}"}
	if p := PlanFor(regTarget(e), false, Options{}); p.Blocked != "" || !strings.Contains(p.Params, "{12345678-1234-1234-1234-1234567890AB}") {
		t.Errorf("msi by command: %+v", p)
	}
}

func TestPlanForSourcesAndNotes(t *testing.T) {
	w := PlanFor(Target{Name: "Microsoft Visual C++ 2015-2022 Redistributable", Method: ByWinget, WingetID: "Microsoft.VCRedist.2015+.x64", Scope: "Machine"}, false, Options{})
	if w.Blocked != "" || !w.Quiet || !w.Interactive || w.Command != "winget uninstall --id Microsoft.VCRedist.2015+.x64 --exact --source winget --silent" {
		t.Fatalf("winget plan = %+v", w)
	}
	if !slices.Contains(w.Notes, "这是运行库或驱动，其他软件可能依赖它") || !slices.Contains(w.Notes, "为所有用户安装，可能会请求管理员权限") {
		t.Errorf("notes = %q", w.Notes)
	}
	if p := PlanFor(Target{Name: "x", Method: ByWinget, WingetID: "9N", WingetSource: "msstore"}, true, Options{}); p.Quiet || !strings.HasSuffix(p.Command, "--source msstore --interactive") {
		t.Errorf("msstore interactive = %+v", p)
	}
	if p := PlanFor(Target{Name: "rg", Method: ByScoop, ScoopName: "ripgrep", Global: true}, false, Options{}); p.Command != "scoop uninstall ripgrep --global" || p.Interactive {
		t.Errorf("scoop = %+v", p)
	}
	if p := PlanFor(Target{Name: "Calc", Method: ByStore, PackageFullName: "Microsoft.WindowsCalculator_11.0_x64__8wekyb3d8bbwe", Scope: "Machine"}, false, Options{}); p.Blocked != "" || len(p.Notes) != 0 {
		t.Errorf("store = %+v", p)
	}
}

func TestSplitCommand(t *testing.T) {
	for in, want := range map[string][2]string{
		`"C:\Program Files\App\unins000.exe" /SILENT`: {`C:\Program Files\App\unins000.exe`, "/SILENT"},
		`"C:\App\u.exe"`:  {`C:\App\u.exe`, ""},
		`C:\App\u.exe /S`: {`C:\App\u.exe`, "/S"},
		`rundll32.exe dfshim.dll,ShArpMaintain app`: {"rundll32.exe", "dfshim.dll,ShArpMaintain app"},
		``: {"", ""},
	} {
		if f, p := SplitCommand(in); f != want[0] || p != want[1] {
			t.Errorf("SplitCommand(%q) = %q, %q", in, f, p)
		}
	}
}

func TestProductCode(t *testing.T) {
	if c := ProductCode(detect.Entry{Name: "{abcdef01-2345-6789-abcd-ef0123456789}"}); c != "{ABCDEF01-2345-6789-ABCD-EF0123456789}" {
		t.Errorf("by name = %q", c)
	}
	if c := ProductCode(detect.Entry{Name: "App", UninstallString: "setup.exe {abcdef01-2345-6789-abcd-ef0123456789}"}); c != "" {
		t.Errorf("a GUID outside an msiexec command = %q", c)
	}
}

// TestPlansOfThisMachine plans the removal of every program of this
// machine's registry, and inspects a few. It removes nothing.
func TestPlansOfThisMachine(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	entries, err := detect.Scan()
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[Kind]int{}
	quiet, blocked := 0, 0
	var some []Target
	for _, e := range entries {
		tg := Target{Key: "x", Name: e.DisplayName, Method: ByRegistry, Scope: e.Scope, Reg: &e, Location: e.InstallLocation}
		p := PlanFor(tg, false, Options{})
		if p.Blocked != "" {
			blocked++
			continue
		}
		kinds[p.Kind]++
		if p.Quiet {
			quiet++
		}
		if p.File == "" || p.Command == "" {
			t.Errorf("%s: no command: %+v", e.DisplayName, p)
		}
		if len(some) < 8 {
			some = append(some, tg)
		}
	}
	t.Logf("%d programs: %v; %d quiet, %d blocked", len(entries), kinds, quiet, blocked)
	if len(some) == 0 {
		t.Skip("no program to inspect")
	}
	r := New(Options{}, nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	signed := 0
	for i, in := range r.Inspect(ctx, some) {
		if in.Signer != "" {
			signed++
		}
		t.Logf("%s: %s %s · %q · %s", some[i].Name, KindText(in.Kind), in.Command, in.Notes, in.Signer)
	}
	t.Logf("%d of %d signed", signed, len(some))

	// A file Windows signs reads as signed.
	explorer := filepath.Join(os.Getenv("SystemRoot"), "explorer.exe")
	in := r.Inspect(ctx, []Target{regTarget(detect.Entry{DisplayName: "Explorer", UninstallString: `"` + explorer + `"`})})[0]
	if in.SignedFile != explorer || in.Signer == "" || !in.Signed {
		t.Errorf("explorer.exe: %+v", in)
	}
	t.Logf("explorer.exe signed by %s", in.Signer)
}

// TestUninstallRealOptIn removes a real program by its registry key, only
// with APPSHELF_UNINSTALL_TEST=1 and APPSHELF_UNINSTALL_REG set to the key
// as <Machine|User>\<X64|X86>\<key name>, of a program the user chose. It
// changes the machine.
func TestUninstallRealOptIn(t *testing.T) {
	if os.Getenv("APPSHELF_UNINSTALL_TEST") != "1" || os.Getenv("APPSHELF_UNINSTALL_REG") == "" {
		t.Skip("set APPSHELF_UNINSTALL_TEST=1 and APPSHELF_UNINSTALL_REG to remove a real program")
	}
	parts := strings.SplitN(os.Getenv("APPSHELF_UNINSTALL_REG"), `\`, 3)
	if len(parts) != 3 {
		t.Fatal(`APPSHELF_UNINSTALL_REG must be <Machine|User>\<X64|X86>\<key name>`)
	}
	e, ok, err := detect.Lookup(parts[0], parts[1], parts[2])
	if err != nil || !ok {
		t.Fatalf("no such program: %v", err)
	}
	tg := Target{Key: "test", Name: e.DisplayName, Method: ByRegistry, Scope: e.Scope, Reg: &e, Location: e.InstallLocation}
	r := New(Options{}, nil, nil)
	in := r.Inspect(context.Background(), []Target{tg})[0]
	t.Logf("plan: %s %s, quiet %v, notes %q", KindText(in.Kind), in.Command, in.Quiet, in.Notes)
	if in.Blocked != "" {
		t.Fatal(in.Blocked)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	var log strings.Builder
	_, runErr := r.Run(ctx, tg, false, &log)
	t.Log(log.String())
	present, err := r.Present(ctx, tg)
	if err != nil || present {
		t.Fatalf("still there after removing (%v): %v", runErr, err)
	}
}
