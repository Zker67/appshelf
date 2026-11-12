// Package winget installs apps with winget. It reads what is installed and
// what has updates through the Microsoft.WinGet.Client PowerShell module,
// whose objects come back as JSON. It installs, updates and removes by
// exact ID through the module's cmdlets, winget's COM interface, which
// report how far the download has come; with the winget command when the
// module cannot be loaded.
package winget

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"appshelf/internal/catalog"
	"appshelf/internal/runner"
	"appshelf/internal/source"
)

// ModuleName is the PowerShell module the queries need.
const ModuleName = "Microsoft.WinGet.Client"

const queryTimeout = 3 * time.Minute

// Source is the winget source.
type Source struct {
	// cli serializes the winget command: its local index takes one
	// installer at a time. The module's queries go through winget's COM
	// server, which handles them alongside.
	cli sync.Mutex

	// list serializes the full queries of what is installed, and keeps the
	// last one: the catalog's states and the list of installed software
	// share it.
	list     sync.Mutex
	listed   []Package
	listedAt time.Time

	// search is the PowerShell kept for the searches, started on first use.
	searchOnce sync.Once
	search     *runner.Worker
}

// New returns the winget source.
func New() *Source { return &Source{} }

func (s *Source) Type() catalog.SourceType { return catalog.Winget }

const checkScript = `
$m = Get-Module -ListAvailable -Name Microsoft.WinGet.Client | Sort-Object Version -Descending | Select-Object -First 1
$v = ''
if ($m) { $v = [string]$m.Version }
Write-Result @{ Module = [bool]$m; Version = $v }
`

// Check finds winget and the module.
func (s *Source) Check(ctx context.Context) source.Availability {
	path, err := exec.LookPath("winget")
	if err != nil {
		return source.Availability{Reason: "找不到 winget，请先从 Microsoft Store 安装“应用安装程序”"}
	}
	res, err := runner.Run(ctx, path, []string{"--version"}, runner.Options{Timeout: time.Minute})
	if err != nil || res.ExitCode != 0 {
		return source.Availability{Reason: "winget 无法运行：" + firstNonEmpty(errText(err), res.Tail(2))}
	}
	var m struct {
		Module  bool
		Version string
	}
	if _, err := runner.PowerShell(ctx, checkScript, nil, &m, runner.Options{Timeout: time.Minute}); err != nil {
		return source.Availability{Reason: "无法检查 PowerShell 模块：" + err.Error()}
	}
	if !m.Module {
		return source.Availability{Reason: "缺少 PowerShell 模块 " + ModuleName, Fixable: true}
	}
	return source.Availability{OK: true, Reason: "winget " + strings.TrimSpace(res.Tail(1)) + "，模块 " + m.Version}
}

const installModuleScript = `
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
$p = Get-PackageProvider -ListAvailable -Name NuGet -ErrorAction SilentlyContinue | Where-Object { $_.Version -ge [version]'2.8.5.201' }
if (-not $p) {
  Install-PackageProvider -Name NuGet -MinimumVersion 2.8.5.201 -Force -Scope CurrentUser | Out-Null
}
Install-Module -Name Microsoft.WinGet.Client -Scope CurrentUser -Force -AllowClobber -Repository PSGallery
Write-Result @{ Ok = $true }
`

// Fix installs the PowerShell module for the current user, from the
// PowerShell Gallery.
func (s *Source) Fix(ctx context.Context, log io.Writer) error {
	_, err := runner.PowerShell(ctx, installModuleScript, nil, nil, runner.Options{Log: log, Timeout: 10 * time.Minute})
	if err != nil {
		return fmt.Errorf("安装 %s 失败：%w", ModuleName, err)
	}
	return nil
}

// row is a package as the module reports it.
type row struct {
	ID                string `json:"Id"`
	Name              string
	InstalledVersion  string
	Source            string
	IsUpdateAvailable bool
	Latest            string
}

const rowScript = `
function ConvertTo-Row($pkg) {
  $latest = ''
  if ($pkg.IsUpdateAvailable) { $latest = [string](@($pkg.AvailableVersions)[0]) }
  [pscustomobject]@{
    Id = [string]$pkg.Id
    Name = [string]$pkg.Name
    InstalledVersion = [string]$pkg.InstalledVersion
    Source = [string]$pkg.Source
    IsUpdateAvailable = [bool]$pkg.IsUpdateAvailable
    Latest = $latest
  }
}
Import-Module Microsoft.WinGet.Client
`

// listScript reads every installed package once. Reading AvailableVersions
// costs time, so only packages with an update read it. With about 300
// packages it takes 3 to 6 seconds, so the catalog's states use
// statusesScript, which converts only the catalog's packages.
const listScript = rowScript + `
$rows = @(Get-WinGetPackage | ForEach-Object { ConvertTo-Row $_ })
Write-Result $rows
`

// statusesScript reads every installed package once and keeps those of
// the catalog.
const statusesScript = rowScript + `
$want = New-Object 'System.Collections.Generic.HashSet[string]' ([System.StringComparer]::OrdinalIgnoreCase)
foreach ($id in @($P.Ids)) { [void]$want.Add([string]$id) }
$rows = @(Get-WinGetPackage | Where-Object { $want.Contains([string]$_.Id) } | ForEach-Object { ConvertTo-Row $_ })
Write-Result $rows
`

// Package is an installed package as winget reports it: one of a source,
// or a program of no source, whose ID starts with ARP\ (a program of the
// registry) or MSIX\ (an app package).
type Package struct {
	ID      string
	Name    string
	Version string // "" when winget cannot read it
	Source  string // "winget", "msstore", or "" for no source
	Update  string // the newer version of the source, when there is one
}

// Installed lists what is installed, reading it again unless the last list
// is younger than maxAge.
func (s *Source) Installed(ctx context.Context, maxAge time.Duration) ([]Package, time.Time, error) {
	s.list.Lock()
	defer s.list.Unlock()
	if !s.listedAt.IsZero() && time.Since(s.listedAt) < maxAge {
		return s.listed, s.listedAt, nil
	}
	var rows []row
	if _, err := runner.PowerShell(ctx, listScript, nil, &rows, runner.Options{Timeout: queryTimeout}); err != nil {
		return nil, time.Time{}, fmt.Errorf("读取 winget 已装软件失败：%w", err)
	}
	pkgs := make([]Package, 0, len(rows))
	for _, r := range rows {
		if r.ID == "" {
			continue
		}
		p := Package{ID: r.ID, Name: r.Name, Version: displayVersion(r.InstalledVersion), Source: r.Source}
		if r.IsUpdateAvailable {
			p.Update = r.Latest
		}
		pkgs = append(pkgs, p)
	}
	s.listed, s.listedAt = pkgs, time.Now()
	return pkgs, s.listedAt, nil
}

// Statuses reads the state of the apps. A package installed outside
// winget but recognized as one of its packages counts as installed; IDs
// compare without case, so a different case never reads as not installed.
func (s *Source) Statuses(ctx context.Context, apps []catalog.App) (map[string]source.Status, error) {
	out := map[string]source.Status{}
	if len(apps) == 0 {
		return out, nil
	}
	ids := make([]string, 0, len(apps))
	for _, a := range apps {
		ids = append(ids, a.Source.ID)
	}
	var rows []row
	if _, err := runner.PowerShell(ctx, statusesScript, map[string]any{"Ids": ids}, &rows, runner.Options{Timeout: queryTimeout}); err != nil {
		return nil, fmt.Errorf("读取 winget 已装软件失败：%w", err)
	}
	byID := map[string]row{}
	for _, r := range rows {
		byID[strings.ToLower(r.ID)] = r
	}
	for _, a := range apps {
		r, ok := byID[strings.ToLower(a.Source.ID)]
		out[a.ID] = statusOf(r, ok)
	}
	return out, nil
}

const recheckScript = rowScript + `
$rows = @(Get-WinGetPackage -Id ([string]$P.Id) -MatchOption EqualsCaseInsensitive | ForEach-Object { ConvertTo-Row $_ })
Write-Result $rows
`

// Recheck reads the state of one app.
func (s *Source) Recheck(ctx context.Context, app catalog.App) (source.Status, error) {
	var rows []row
	if _, err := runner.PowerShell(ctx, recheckScript, map[string]any{"Id": app.Source.ID}, &rows, runner.Options{Timeout: queryTimeout}); err != nil {
		return source.Status{}, fmt.Errorf("读取 winget 安装状态失败：%w", err)
	}
	for _, r := range rows {
		if strings.EqualFold(r.ID, app.Source.ID) {
			return statusOf(r, true), nil
		}
	}
	return source.Status{State: source.NotInstalled}, nil
}

func statusOf(r row, found bool) source.Status {
	if !found {
		return source.Status{State: source.NotInstalled}
	}
	st := source.Status{State: source.Installed, Version: displayVersion(r.InstalledVersion)}
	if r.IsUpdateAvailable && r.Latest != "" {
		st.State, st.Latest = source.UpdateAvailable, r.Latest
	}
	return st
}

// displayVersion hides the "Unknown" winget reports for versions it cannot
// read.
func displayVersion(v string) string {
	if strings.EqualFold(v, "unknown") {
		return ""
	}
	return v
}

// searchScript finds packages. It reads no version and no publisher: the
// module reads them from each package's manifests, downloading what it has
// not cached, one package after another, which takes half a minute for 50
// packages. SearchVersions reads the versions instead.
const searchScript = `
Import-Module Microsoft.WinGet.Client
$rows = @(Find-WinGetPackage -Query ([string]$P.Query) -Source winget -Count 50 | ForEach-Object {
  [pscustomobject]@{ Id = [string]$_.Id; Name = [string]$_.Name }
})
Write-Result $rows
`

// warmScript loads the module and opens the winget source, as the first
// search would.
const warmScript = `
Import-Module Microsoft.WinGet.Client
$null = @(Find-WinGetPackage -Id AppShelf.Warm.NoSuchPackage -MatchOption Equals -Source winget -Count 1)
`

// Search finds packages of the winget source, without their versions. It
// runs in a PowerShell kept for the searches.
func (s *Source) Search(ctx context.Context, query string) ([]source.SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	var rows []struct {
		ID   string `json:"Id"`
		Name string
	}
	if err := s.searcher().Run(ctx, searchScript, map[string]any{"Query": query}, &rows, queryTimeout); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("winget 搜索失败：%w", err)
	}
	out := make([]source.SearchResult, 0, len(rows))
	for _, r := range rows {
		out = append(out, source.SearchResult{ID: r.ID, Name: r.Name, Source: "winget"})
	}
	return out, nil
}

// Warm starts the PowerShell of the searches and opens the winget source,
// for the first search to be quick.
func (s *Source) Warm(ctx context.Context) error {
	return s.searcher().Warm(ctx, warmScript)
}

func (s *Source) searcher() *runner.Worker {
	s.searchOnce.Do(func() { s.search = runner.NewWorker() })
	return s.search
}

// SearchVersions runs winget search for the query and returns the latest
// version of each package it lists, by lowercase ID. winget reads them from
// its local index, without downloading. It is for display: the IDs to look
// up come from Search, and an ID winget shortens finds nothing.
func (s *Source) SearchVersions(ctx context.Context, query string) (map[string]string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return map[string]string{}, nil
	}
	path, err := exec.LookPath("winget")
	if err != nil {
		return nil, errors.New("找不到 winget")
	}
	args := []string{"search", "--query", query, "--source", "winget", "--count", "100",
		"--disable-interactivity", "--accept-source-agreements"}
	res, err := runner.Run(ctx, path, args, runner.Options{Timeout: time.Minute})
	if err != nil {
		return nil, err
	}
	return parseSearchTable(res.Output), nil
}

// parseSearchTable reads winget search's table: each ID is followed by its
// version. Names come first and may hold spaces, and what follows the
// version is localized, so every word maps to the next one; a winget ID has
// a dot and no space, and the IDs looked up are exact.
func parseSearchTable(output []byte) map[string]string {
	out := map[string]string{}
	text := strings.ReplaceAll(string(output), "\r\n", "\n")
	for line := range strings.SplitSeq(text, "\n") {
		if i := strings.LastIndexByte(line, '\r'); i >= 0 {
			line = line[i+1:]
		}
		f := strings.Fields(stripControl(line))
		for i := 0; i+1 < len(f); i++ {
			k := strings.ToLower(f[i])
			if !strings.Contains(k, ".") || strings.ContainsRune(k, '…') {
				continue
			}
			if _, ok := out[k]; !ok {
				out[k] = f[i+1]
			}
		}
	}
	return out
}

// stripControl drops the escape sequences of winget's progress output.
func stripControl(s string) string {
	if !strings.ContainsRune(s, 0x1b) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// LatestVersion returns the version of a package the winget source would
// install now.
func (s *Source) LatestVersion(ctx context.Context, id string) (string, error) {
	var rows []struct {
		ID      string `json:"Id"`
		Version string
	}
	if _, err := runner.PowerShell(ctx, latestScript, map[string]any{"Id": id}, &rows, runner.Options{Timeout: queryTimeout}); err != nil {
		return "", fmt.Errorf("winget 查询失败：%w", err)
	}
	for _, r := range rows {
		if strings.EqualFold(r.ID, id) {
			return r.Version, nil
		}
	}
	return "", errors.New("winget 源中找不到这个 ID")
}

const latestScript = `
Import-Module Microsoft.WinGet.Client
$rows = @(Find-WinGetPackage -Id ([string]$P.Id) -MatchOption EqualsCaseInsensitive -Source winget | ForEach-Object {
  [pscustomobject]@{ Id = [string]$_.Id; Version = [string]$_.Version }
})
Write-Result $rows
`

// Exit codes of winget, from its documentation (returnCodes.md).
const (
	codeNoPackageFound      = 0x8A150014
	codeMultiplePackages    = 0x8A150016
	codeUpdateNotApplicable = 0x8A15002B
	codeAlreadyInstalled    = 0x8A150061
	codeInUse               = 0x8A150101
	codeInstallInProgress   = 0x8A150102
	codeRebootToFinish      = 0x8A150109
	codeCanceledByUser      = 0x8A15010C
)

// codeTexts say in plain words why an install or update failed, for the
// other exit codes of returnCodes.md that the user can act on.
var codeTexts = map[uint32]string{
	0x8A150011: "下载的安装程序与清单的校验值不一致，已停止。可能是发布方刚更新了文件，稍后再试",
	0x8A15002D: "安装程序没有通过安全检查，已停止",
	0x8A15002E: "下载的文件大小不对，可能是网络中断，请重试",
	0x8A150049: "Windows Installer 运行失败，请查看日志",
	0x8A150103: "有文件正被占用，请关闭这个软件后重试",
	0x8A150104: "缺少这个软件需要的系统组件",
	0x8A150105: "磁盘空间不足，请清理后重试",
	0x8A150106: "内存不足，请关闭其他程序后重试",
	0x8A150107: "需要联网，请连接网络后重试",
	0x8A150108: "安装程序出错，请查看日志或联系软件作者",
	0x8A15010A: "需要先重启电脑，再重试",
	0x8A15010B: "电脑将重启以完成安装",
	0x8A15010D: "已安装了这个软件的另一个版本",
	0x8A15010E: "已安装了更高的版本",
	0x8A15010F: "组织策略阻止了安装，请联系管理员",
	0x8A150110: "依赖的软件包安装失败",
	0x8A150111: "软件正被其他程序使用，请关闭后重试",
	0x8A150112: "安装参数无效",
	0x8A150113: "这台电脑的系统不支持这个软件",
	0x8A150114: "这个软件的安装程序不支持直接更新，请卸载后重新安装",
	0x8A150115: "安装程序报告了自定义错误，请查看日志",
}

// Install installs the app's package by its exact ID from the winget
// source; an installed package is left as it is.
func (s *Source) Install(ctx context.Context, app catalog.App, log io.Writer) (source.Result, error) {
	res, err := s.viaCOM(ctx, log, comOp{Verb: "install", ID: app.Source.ID, Source: "winget", Mode: "Default"}, "正在安装")
	if errors.Is(err, errUnavailable) {
		return s.run(ctx, log, "install", app.Source.ID, "--no-upgrade")
	}
	return res, err
}

// Upgrade updates the app's package by its exact ID, its installer in its
// default mode, which may show its progress.
func (s *Source) Upgrade(ctx context.Context, app catalog.App, log io.Writer) (source.Result, error) {
	return s.upgrade(ctx, app, "Default", log)
}

// UpgradeSilent updates the app's package by its exact ID, its installer
// asked to show no window: winget's silent mode.
func (s *Source) UpgradeSilent(ctx context.Context, app catalog.App, log io.Writer) (source.Result, error) {
	return s.upgrade(ctx, app, "Silent", log)
}

// UpgradeInteractive updates the app's package by its exact ID with its
// installer's window, for the user to finish it there.
func (s *Source) UpgradeInteractive(ctx context.Context, app catalog.App, log io.Writer) (source.Result, error) {
	return s.upgrade(ctx, app, "Interactive", log)
}

// upgradeFlags are the command's flags of the cmdlet's modes.
var upgradeFlags = map[string][]string{"Silent": {"--silent"}, "Interactive": {"--interactive"}}

func (s *Source) upgrade(ctx context.Context, app catalog.App, mode string, log io.Writer) (source.Result, error) {
	res, err := s.viaCOM(ctx, log, comOp{Verb: "upgrade", ID: app.Source.ID, Source: "winget", Mode: mode}, "正在更新")
	if errors.Is(err, errUnavailable) {
		return s.run(ctx, log, "upgrade", app.Source.ID, upgradeFlags[mode]...)
	}
	return res, err
}

// Uninstall removes a package by its exact ID, from the source that knows
// it ("winget" or "msstore"), silently unless interactive asks for the
// uninstaller's window.
func (s *Source) Uninstall(ctx context.Context, id, src string, interactive bool, log io.Writer) (source.Result, error) {
	if src == "" {
		src = "winget"
	}
	mode := "Silent"
	if interactive {
		mode = "Interactive"
	}
	res, err := s.viaCOM(ctx, log, comOp{Verb: "uninstall", ID: id, Source: src, Mode: mode}, "正在卸载")
	if errors.Is(err, errUnavailable) {
		return s.uninstallCLI(ctx, id, src, interactive, log)
	}
	return res, err
}

// uninstallCLI removes a package with the winget command.
func (s *Source) uninstallCLI(ctx context.Context, id, src string, interactive bool, log io.Writer) (source.Result, error) {
	mode := "--silent"
	if interactive {
		mode = "--interactive"
	}
	path, err := exec.LookPath("winget")
	if err != nil {
		return source.Result{}, errors.New("找不到 winget")
	}
	args := []string{"uninstall", "--id", id, "--exact", "--source", src, mode, "--accept-source-agreements", "--disable-interactivity"}
	s.cli.Lock()
	defer s.cli.Unlock()
	if log == nil {
		log = io.Discard
	}
	fmt.Fprintf(log, "> winget %s\n\n", strings.Join(args, " "))
	p := newProgress(log, id, "正在卸载")
	source.ReportProgress(log, source.Progress{Stage: "正在卸载"})
	defer p.close()
	res, err := runner.Run(ctx, path, args, runner.Options{Log: p})
	if err != nil {
		return source.Result{}, err
	}
	if res.Code() == codeNoPackageFound {
		// Gone already, or never there: the check after it tells.
		return source.Result{}, nil
	}
	r, err := interpret(res)
	if err != nil && res.Code() == codeCanceledByUser {
		return r, errors.New("卸载被取消")
	}
	return r, err
}

func (s *Source) run(ctx context.Context, log io.Writer, verb, id string, extra ...string) (source.Result, error) {
	path, err := exec.LookPath("winget")
	if err != nil {
		return source.Result{}, errors.New("找不到 winget")
	}
	args := []string{verb, "--id", id, "--exact", "--source", "winget",
		"--accept-package-agreements", "--accept-source-agreements", "--disable-interactivity"}
	args = append(args, extra...)
	s.cli.Lock()
	defer s.cli.Unlock()
	if log == nil {
		log = io.Discard
	}
	fmt.Fprintf(log, "> winget %s\n\n", strings.Join(args, " "))
	work := "正在安装"
	if verb == "upgrade" {
		work = "正在更新"
	}
	p := newProgress(log, id, work)
	defer p.close()
	res, err := runner.Run(ctx, path, args, runner.Options{Log: p})
	if err != nil {
		return source.Result{}, err
	}
	return interpret(res)
}

// interpret turns winget's exit code into a result.
func interpret(res runner.Result) (source.Result, error) {
	return interpretCode(res.Code(), res.Tail(2))
}

// interpretCode turns a code of winget's, the command's exit code or the
// COM interface's HRESULT, into a result; detail says more of a code
// without a text of its own.
func interpretCode(code uint32, detail string) (source.Result, error) {
	switch code {
	case 0:
		return source.Result{}, nil
	case codeAlreadyInstalled:
		return source.Result{AlreadyInstalled: true}, nil
	case codeRebootToFinish:
		return source.Result{RebootRequired: true}, nil
	case codeNoPackageFound:
		return source.Result{}, errors.New("winget 源中找不到这个 ID")
	case codeMultiplePackages:
		return source.Result{}, errors.New("这个 ID 匹配到多个软件包，没有安装")
	case codeUpdateNotApplicable:
		return source.Result{}, errors.New("没有适用的更新")
	case codeInUse:
		return source.Result{}, errors.New("软件正在运行，请关闭后重试")
	case codeInstallInProgress:
		return source.Result{}, source.ErrInstallerBusy
	case codeCanceledByUser:
		return source.Result{}, errors.New("安装被取消")
	}
	if text, ok := codeTexts[code]; ok {
		return source.Result{}, fmt.Errorf("%s（winget 退出码 0x%08X）", text, code)
	}
	msg := fmt.Sprintf("winget 退出码 0x%08X", code)
	if detail != "" {
		msg += "：" + detail
	}
	return source.Result{}, errors.New(msg)
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}
