package uninstall

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"appshelf/internal/catalog"
	"appshelf/internal/detect"
	"appshelf/internal/procs"
	"appshelf/internal/runner"
	"appshelf/internal/source"
)

// WingetAPI is what removing needs of winget.
type WingetAPI interface {
	Uninstall(ctx context.Context, id, src string, interactive bool, log io.Writer) (source.Result, error)
	Recheck(ctx context.Context, app catalog.App) (source.Status, error)
}

// ScoopAPI is what removing needs of scoop.
type ScoopAPI interface {
	Uninstall(ctx context.Context, name string, global bool, log io.Writer) (source.Result, error)
	Recheck(ctx context.Context, app catalog.App) (source.Status, error)
}

// Runner removes programs and tells whether they are still there.
type Runner struct {
	opt    Options
	winget WingetAPI // nil without winget
	scoop  ScoopAPI  // nil without scoop

	// What it uses of Windows, replaced in tests.
	shellRun   func(file, params string, elevate bool) (uint32, error)
	lookup     func(scope, arch, name string) (detect.Entry, bool, error)
	processes  func() ([]string, error)
	powershell func(ctx context.Context, script string, in, out any, log io.Writer) error
	// settleMax is how long to wait for an uninstaller's copies to end;
	// grace how long after they end for the key to go; poll how often.
	settleMax, grace, poll time.Duration
}

// New returns a runner removing with winget and scoop, either nil when the
// machine lacks it.
func New(opt Options, w WingetAPI, s ScoopAPI) *Runner {
	return &Runner{
		opt: opt, winget: w, scoop: s,
		shellRun: func(file, params string, elevate bool) (uint32, error) {
			if elevate {
				return runner.ShellRunElevated(file, params, "")
			}
			return runner.ShellRun(file, params, "")
		},
		lookup:    detect.Lookup,
		processes: procs.Paths,
		powershell: func(ctx context.Context, script string, in, out any, log io.Writer) error {
			_, err := runner.PowerShell(ctx, script, in, out, runner.Options{Log: log, Timeout: 10 * time.Minute})
			return err
		},
		settleMax: 30 * time.Minute, grace: 5 * time.Second, poll: 2 * time.Second,
	}
}

// Present reports whether t is still on the machine.
func (r *Runner) Present(ctx context.Context, t Target) (bool, error) {
	switch t.Method {
	case ByWinget:
		if r.winget == nil {
			return false, errors.New("winget 不可用")
		}
		st, err := r.winget.Recheck(ctx, catalog.App{ID: "uninstall", Name: t.Name, Source: catalog.Source{Type: catalog.Winget, ID: t.WingetID}})
		if err != nil {
			return false, err
		}
		return st.IsInstalled(), nil
	case ByScoop:
		if r.scoop == nil {
			return false, errors.New("scoop 不可用")
		}
		st, err := r.scoop.Recheck(ctx, catalog.App{ID: "uninstall", Name: t.Name, Source: catalog.Source{Type: catalog.Scoop, Name: t.ScoopName}})
		if err != nil {
			return false, err
		}
		if st.Blocked() {
			return false, errors.New(st.Note)
		}
		return st.IsInstalled(), nil
	case ByStore:
		var out struct{ Found bool }
		if err := r.powershell(ctx, appxFindScript, map[string]any{"Family": familyOf(t.PackageFullName), "Name": strings.SplitN(t.PackageFullName, "_", 2)[0]}, &out, nil); err != nil {
			return false, fmt.Errorf("读取应用包失败：%w", err)
		}
		return out.Found, nil
	case ByRegistry:
		if t.Reg == nil {
			return false, errors.New("没有这个程序的注册表项")
		}
		_, ok, err := r.lookup(t.Reg.Scope, t.Reg.Arch, t.Reg.Name)
		return ok, err
	}
	return false, errors.New("不支持的卸载方式")
}

const appxFindScript = `
$found = @(Get-AppxPackage -Name ([string]$P.Name) | Where-Object { $_.PackageFamilyName -eq [string]$P.Family }).Count -gt 0
Write-Result @{ Found = $found }
`

const appxRemoveScript = `
$ErrorActionPreference = 'Stop'
Remove-AppxPackage -Package ([string]$P.Package)
Write-Result @{ Done = $true }
`

// Run removes t, quietly unless interactive asks for the uninstaller's
// window, writing what happens to log. It returns once the uninstaller and
// the copies of itself it starts have ended. An error says what went
// wrong; whether t is gone is for Present to tell.
func (r *Runner) Run(ctx context.Context, t Target, interactive bool, log io.Writer) (source.Result, error) {
	if log == nil {
		log = io.Discard
	}
	p := PlanFor(t, interactive, r.opt)
	if p.Blocked != "" {
		return source.Result{}, errors.New(p.Blocked)
	}
	switch t.Method {
	case ByWinget:
		if r.winget == nil {
			return source.Result{}, errors.New("winget 不可用")
		}
		return r.winget.Uninstall(ctx, t.WingetID, t.WingetSource, interactive, log)
	case ByScoop:
		if r.scoop == nil {
			return source.Result{}, errors.New("scoop 不可用")
		}
		return r.scoop.Uninstall(ctx, t.ScoopName, t.Global, log)
	case ByStore:
		fmt.Fprintf(log, "> %s\n\n", p.Command)
		source.ReportProgress(log, source.Progress{Stage: "正在卸载"})
		if err := r.powershell(ctx, appxRemoveScript, map[string]any{"Package": t.PackageFullName}, nil, log); err != nil {
			return source.Result{}, fmt.Errorf("移除应用包失败：%w", err)
		}
		return source.Result{}, nil
	}
	return r.runRegistry(ctx, t, p, log)
}

func (r *Runner) runRegistry(ctx context.Context, t Target, p Plan, log io.Writer) (source.Result, error) {
	fmt.Fprintf(log, "> %s\n", p.Command)
	if p.Quiet {
		fmt.Fprintf(log, "（%s，静默卸载）\n\n", KindText(p.Kind))
	} else {
		fmt.Fprintf(log, "（%s，打开卸载程序窗口）\n\n", KindText(p.Kind))
	}
	source.ReportProgress(log, source.Progress{Stage: "正在卸载"})
	code, err := r.shellRun(p.File, p.Params, p.Elevate)
	if err != nil {
		return source.Result{}, err
	}
	fmt.Fprintf(log, "卸载程序退出码：%d\n", code)
	source.ReportProgress(log, source.Progress{Stage: "正在等待卸载程序结束"})
	r.settle(ctx, t, log)
	switch code {
	case 0, 1605: // 1605: Windows Installer knows no such product
		return source.Result{}, nil
	case 3010, 1641:
		return source.Result{RebootRequired: true}, nil
	case 1602, 1223:
		return source.Result{}, errors.New("卸载被取消")
	case 1618:
		return source.Result{}, errors.New("另一个安装或卸载正在进行，请稍后重试")
	}
	return source.Result{}, fmt.Errorf("卸载程序以退出码 %d 结束", code)
}

// settle waits while t's registry key is there and an uninstaller still
// runs: NSIS and Inno Setup copy themselves to the temporary directory and
// exit at once, the copy doing the work.
func (r *Runner) settle(ctx context.Context, t Target, log io.Writer) {
	if t.Reg == nil {
		return
	}
	deadline := time.Now().Add(r.settleMax)
	waited := false
	for {
		if _, ok, err := r.lookup(t.Reg.Scope, t.Reg.Arch, t.Reg.Name); err == nil && !ok {
			return
		}
		if !r.busy(t) || time.Now().After(deadline) {
			break
		}
		if !waited {
			fmt.Fprintln(log, "[AppShelf] 卸载程序还在运行，等待它结束")
			waited = true
		}
		if !sleep(ctx, r.poll) {
			return
		}
	}
	// Uninstallers remove their key last.
	end := time.Now().Add(r.grace)
	for time.Now().Before(end) {
		if _, ok, err := r.lookup(t.Reg.Scope, t.Reg.Arch, t.Reg.Name); err == nil && !ok {
			return
		}
		if !sleep(ctx, min(r.poll, r.grace)) {
			return
		}
	}
}

// busy reports an uninstaller still running: a copy NSIS or Inno Setup
// made of itself, or a program of t's directory.
func (r *Runner) busy(t Target) bool {
	paths, err := r.processes()
	if err != nil {
		return false
	}
	own := t.Location != "" && detect.OwnDirectory(t.Location)
	for _, p := range paths {
		base := strings.ToLower(filepath.Base(p))
		switch {
		case base == "au_.exe", base == "un_a.exe", base == "un_b.exe":
			return true
		case strings.HasSuffix(base, ".tmp") && (strings.HasPrefix(base, "_iu") || strings.HasPrefix(base, "unins")):
			return true
		}
		if own && procs.Under([]string{p}, t.Location) {
			return true
		}
	}
	return false
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// KindText names an installer for the user.
func KindText(k Kind) string {
	switch k {
	case MSI:
		return "Windows Installer"
	case Burn:
		return "WiX 安装包"
	case Inno:
		return "Inno Setup"
	case NSIS:
		return "NSIS"
	case InstallShield:
		return "InstallShield"
	}
	return "卸载程序"
}
