package winget

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	"appshelf/internal/runner"
	"appshelf/internal/source"
)

// Installs, updates and removals go through winget's COM interface, by the
// module's cmdlets, which report how far the download has come: winget's
// command reports nothing of it when its output is not a console. The
// cmdlets' progress records reach no console host; a background job keeps
// them for its parent to read, while a nested runspace waits forever.

// comOp is one install, update or removal.
type comOp struct {
	Verb   string // install, upgrade or uninstall
	ID     string
	Source string // winget or msstore
	Mode   string // the cmdlet's -Mode: Default, Silent or Interactive
}

// cmdlet is the module's cmdlet for op.
func (op comOp) cmdlet() string {
	switch op.Verb {
	case "upgrade":
		return "Update-WinGetPackage"
	case "uninstall":
		return "Uninstall-WinGetPackage"
	}
	return "Install-WinGetPackage"
}

// comResult is what the script answers.
type comResult struct {
	// Unavailable says why the module could not be loaded: nothing ran.
	Unavailable    string
	Status         string // the cmdlet's, or AlreadyInstalled, NotInstalled
	RebootRequired bool
	Code           int64  // the HRESULT of the error, as winget's exit code
	Message        string // the error's message
	InstallerCode  int64  // the installer's or uninstaller's exit code
}

// progressMark starts the lines the script writes for a progress record:
// the mark, the status and the percentage, separated by tabs.
const progressMark = "@@progress\t"

const comScript = `
$job = Start-Job -ArgumentList ($P | ConvertTo-Json -Compress) -ScriptBlock {
  param([string]$json)
  $ErrorActionPreference = 'Stop'
  $ProgressPreference = 'Continue'
  $P = $json | ConvertFrom-Json
  try {
    Import-Module Microsoft.WinGet.Client
  } catch {
    return [pscustomobject]@{ Unavailable = [string]$_.Exception.Message }
  }
  $find = @{ Id = [string]$P.Id; MatchOption = 'EqualsCaseInsensitive'; Source = [string]$P.Source }
  switch ([string]$P.Verb) {
    'install' {
      if (@(Get-WinGetPackage @find).Count -gt 0) { return [pscustomobject]@{ Status = 'AlreadyInstalled' } }
      $r = Install-WinGetPackage @find -Mode ([string]$P.Mode) -Confirm:$false
    }
    'upgrade' {
      $r = Update-WinGetPackage @find -Mode ([string]$P.Mode) -Confirm:$false
    }
    'uninstall' {
      if (@(Get-WinGetPackage @find).Count -eq 0) { return [pscustomobject]@{ Status = 'NotInstalled' } }
      $r = Uninstall-WinGetPackage @find -Mode ([string]$P.Mode) -Confirm:$false
    }
  }
  $e = $r.ExtendedErrorCode
  $code = 0; $message = ''
  if ($e) { $code = [int64]$e.HResult; $message = [string]$e.Message }
  $installer = 0
  if ($r.PSObject.Properties['InstallerErrorCode']) { $installer = [int64]$r.InstallerErrorCode }
  if ($r.PSObject.Properties['UninstallerErrorCode']) { $installer = [int64]$r.UninstallerErrorCode }
  [pscustomobject]@{
    Status = [string]$r.Status
    RebootRequired = [bool]$r.RebootRequired
    Code = $code
    Message = $message
    InstallerCode = $installer
  }
}
$child = $job.ChildJobs[0]
$stdout = [Console]::Out
$last = ''
function Send-Progress {
  foreach ($r in $child.Progress.ReadAll()) {
    if ([string]$r.RecordType -eq 'Completed') { continue }
    $line = "@@progress` + "`t" + `" + [string]$r.StatusDescription + "` + "`t" + `" + [int]$r.PercentComplete
    if ($line -ne $script:last) {
      $stdout.WriteLine($line)
      $stdout.Flush()
      $script:last = $line
    }
  }
}
while ($job.State -eq 'NotStarted' -or $job.State -eq 'Running') {
  Send-Progress
  Start-Sleep -Milliseconds 300
}
Send-Progress
$rows = @(Receive-Job $job)
Remove-Job $job -Force
if ($rows.Count -eq 0) { throw 'winget 没有返回结果' }
$r = $rows[-1]
Write-Result @{
  Unavailable = [string]$r.Unavailable
  Status = [string]$r.Status
  RebootRequired = [bool]$r.RebootRequired
  Code = [int64]$r.Code
  Message = [string]$r.Message
  InstallerCode = [int64]$r.InstallerCode
}
`

// errUnavailable means the module could not be loaded: the command does
// the work instead.
var errUnavailable = errors.New("winget COM 接口不可用")

// viaCOM does op through the module, its progress reported to log as work
// for the stage after the download, as "正在安装". It returns
// errUnavailable, having done nothing, when the module cannot be loaded.
func (s *Source) viaCOM(ctx context.Context, log io.Writer, op comOp, work string) (source.Result, error) {
	if log == nil {
		log = io.Discard
	}
	s.cli.Lock()
	defer s.cli.Unlock()
	fmt.Fprintf(log, "> %s -Id %s -MatchOption EqualsCaseInsensitive -Source %s -Mode %s\n（winget COM 接口）\n\n", op.cmdlet(), op.ID, op.Source, op.Mode)
	source.ReportProgress(log, source.Progress{Stage: "正在准备"})
	pl := &comLog{log: log, work: work}
	var r comResult
	_, err := runner.PowerShell(ctx, comScript, op, &r, runner.Options{Log: pl})
	pl.flush()
	if err != nil {
		return source.Result{}, err
	}
	if r.Unavailable != "" {
		fmt.Fprintf(log, "[AppShelf] 无法加载 %s（%s），改用 winget 命令\n\n", ModuleName, r.Unavailable)
		return source.Result{}, errUnavailable
	}
	fmt.Fprintf(log, "winget 结果：%s\n", r.Status)
	return interpretCOM(r, op.Verb)
}

// comStatusTexts say in plain words why the cmdlet did not succeed, when
// no error code says more.
var comStatusTexts = map[string]string{
	"BlockedByPolicy":              "组织策略阻止了这个操作，请联系管理员",
	"CatalogError":                 "无法读取 winget 源，请检查网络后重试",
	"InternalError":                "winget 内部错误",
	"InvalidOptions":               "安装参数无效",
	"DownloadError":                "下载安装程序失败，请检查网络后重试",
	"InstallError":                 "安装程序运行失败",
	"UninstallError":               "卸载程序运行失败",
	"ManifestError":                "软件包清单有误，请稍后重试",
	"NoApplicableInstallers":       "没有适用于这台电脑的安装程序",
	"NoApplicableUpgrade":          "没有适用的更新",
	"PackageAgreementsNotAccepted": "需要接受软件包协议",
}

// interpretCOM turns the script's answer into a result, by winget's codes
// as the command's are.
func interpretCOM(r comResult, verb string) (source.Result, error) {
	switch r.Status {
	case "Ok":
		return source.Result{RebootRequired: r.RebootRequired}, nil
	case "AlreadyInstalled":
		return source.Result{AlreadyInstalled: true}, nil
	case "NotInstalled":
		// Gone already, or never there: the check after it tells.
		return source.Result{}, nil
	}
	if r.Code != 0 {
		if verb == "uninstall" && uint32(r.Code) == codeCanceledByUser {
			return source.Result{}, errors.New("卸载被取消")
		}
		return interpretCode(uint32(r.Code), r.Message)
	}
	text := comStatusTexts[r.Status]
	if text == "" {
		text = "winget 没有完成：" + r.Status
	}
	if r.InstallerCode != 0 {
		who := "安装程序"
		if verb == "uninstall" {
			who = "卸载程序"
		}
		return source.Result{}, fmt.Errorf("%s（%s退出码 %d）", text, who, r.InstallerCode)
	}
	if r.Status == "NoApplicableUpgrade" {
		return interpretCode(codeUpdateNotApplicable, "")
	}
	return source.Result{}, errors.New(text)
}

// comLog passes the script's output to the log, but its progress lines,
// which it reports as progress, writing a line each time the stage
// changes.
type comLog struct {
	log  io.Writer
	work string

	mu    sync.Mutex
	line  []byte
	stage string
}

func (c *comLog) Write(b []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, ch := range b {
		if ch == '\n' {
			c.lineDone(string(c.line))
			c.line = c.line[:0]
			continue
		}
		c.line = append(c.line, ch)
	}
	return len(b), nil
}

func (c *comLog) flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.line) > 0 {
		c.lineDone(string(c.line))
		c.line = c.line[:0]
	}
}

func (c *comLog) lineDone(line string) {
	line = strings.TrimRight(line, "\r")
	rest, ok := strings.CutPrefix(line, progressMark)
	if !ok {
		if strings.TrimSpace(line) != "" {
			fmt.Fprintln(c.log, line)
		}
		return
	}
	status, pct, _ := strings.Cut(rest, "\t")
	n, _ := strconv.Atoi(strings.TrimSpace(pct))
	p := comProgress(status, n, c.work)
	if p.Stage != c.stage {
		fmt.Fprintf(c.log, "[winget] %s\n", p.Stage)
		c.stage = p.Stage
	}
	source.ReportProgress(c.log, p)
}

// comProgress is a progress record of the module's as AppShelf shows it.
// A percentage counts only between 0 and 100: the module reports 0 or -1
// for none, and a stage at 100 is over.
func comProgress(status string, percent int, work string) source.Progress {
	s := strings.ToLower(strings.TrimSpace(status))
	var stage string
	switch {
	case strings.Contains(s, "download"):
		stage = "正在下载"
	case strings.Contains(s, "post"):
		stage = "正在完成"
	case strings.Contains(s, "uninstall"):
		stage = "正在卸载"
	case strings.Contains(s, "install"):
		stage = work
	case s == "" || strings.Contains(s, "queue"):
		stage = "正在准备"
	default:
		stage = strings.TrimSpace(status)
	}
	p := source.Progress{Stage: stage}
	if percent > 0 && percent < 100 {
		p.Percent = float64(percent)
	}
	return p
}
