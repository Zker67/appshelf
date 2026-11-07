// Package scoop installs apps with scoop. scoop's commands are PowerShell
// scripts: AppShelf runs them in Windows PowerShell and reads their
// objects, or the JSON of scoop export, rather than their tables.
package scoop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"appshelf/internal/catalog"
	"appshelf/internal/runner"
	"appshelf/internal/source"
)

const (
	queryTimeout  = 3 * time.Minute
	updateTimeout = 10 * time.Minute
)

// Source is the scoop source.
type Source struct {
	// mu serializes scoop: its commands share the apps and buckets
	// directories.
	mu sync.Mutex
}

// New returns the scoop source.
func New() *Source { return &Source{} }

func (s *Source) Type() catalog.SourceType { return catalog.Scoop }

// scoopPrelude lets scoop's own scripts run as they expect: they rely on
// errors that do not stop them.
const scoopPrelude = `
if (-not (Get-Command scoop -ErrorAction SilentlyContinue)) {
  Write-Failure 'scoop-missing'
  exit 20
}
$ErrorActionPreference = 'Continue'
`

const codeMissing = 20

// Check finds scoop.
func (s *Source) Check(ctx context.Context) source.Availability {
	_, err := runner.PowerShell(ctx, scoopPrelude+`Write-Result @{ Ok = $true }`, nil, nil, runner.Options{Timeout: time.Minute})
	var se *runner.ScriptError
	if errors.As(err, &se) && se.ExitCode == codeMissing {
		return source.Availability{Reason: "未安装 scoop"}
	}
	if err != nil {
		return source.Availability{Reason: "无法检查 scoop：" + err.Error()}
	}
	return source.Availability{OK: true, Reason: "已找到 scoop"}
}

const exportScript = scoopPrelude + `
$text = (& scoop export 6>$null | Out-String)
Write-Result @{ Text = $text }
`

// statusScript reads scoop status. With Refresh, it first runs scoop
// update, which brings scoop and the buckets up to date: status compares
// the installed apps with the manifests of the local buckets.
const statusScript = scoopPrelude + `
$updateFailed = $false
if ($P.Refresh) {
  & scoop update *>$null
  $updateFailed = ($LASTEXITCODE -ne 0)
}
$rows = @(& scoop status -l 6>$null | ForEach-Object {
  [pscustomobject]@{
    Name = [string]$_.Name
    Installed = [string]$_.'Installed Version'
    Latest = [string]$_.'Latest Version'
    Info = [string]$_.Info
  }
})
Write-Result @{ Rows = $rows; UpdateFailed = $updateFailed }
`

// exported is the JSON of scoop export.
type exported struct {
	Buckets []struct {
		Name string
	} `json:"buckets"`
	Apps []struct {
		Name    string
		Version string
		Source  string
		Info    string
	} `json:"apps"`
}

// statusRow is an app scoop status lists: outdated, or with a problem.
type statusRow struct {
	Name      string
	Installed string
	Latest    string
	Info      string
}

type statusResult struct {
	Rows         []statusRow
	UpdateFailed bool
}

func parseExport(text string) (exported, error) {
	var e exported
	text = strings.TrimSpace(text)
	if text == "" {
		return e, errors.New("scoop export 没有输出")
	}
	if err := json.Unmarshal([]byte(text), &e); err != nil {
		return e, errors.New("无法解析 scoop export 的输出，请先运行 scoop update 更新 scoop")
	}
	return e, nil
}

func (s *Source) export(ctx context.Context) (exported, error) {
	var out struct{ Text string }
	if _, err := runner.PowerShell(ctx, exportScript, nil, &out, runner.Options{Timeout: queryTimeout}); err != nil {
		return exported{}, fmt.Errorf("读取 scoop 已装软件失败：%w", err)
	}
	return parseExport(out.Text)
}

func (s *Source) status(ctx context.Context, refresh bool) (statusResult, error) {
	var out statusResult
	timeout := queryTimeout
	if refresh {
		timeout = updateTimeout
	}
	if _, err := runner.PowerShell(ctx, statusScript, map[string]any{"Refresh": refresh}, &out, runner.Options{Timeout: timeout}); err != nil {
		return statusResult{}, err
	}
	return out, nil
}

// Statuses reads what is installed with scoop export, then brings the
// buckets up to date and reads the updates with scoop status.
func (s *Source) Statuses(ctx context.Context, apps []catalog.App) (map[string]source.Status, error) {
	if len(apps) == 0 {
		return map[string]source.Status{}, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, err := s.export(ctx)
	if err != nil {
		return nil, err
	}
	st, stErr := s.status(ctx, true)
	return buildStatuses(apps, exp, st, stErr), nil
}

// App is an app scoop installed.
type App struct {
	Name    string
	Version string
	Bucket  string // where it was installed from
	Update  string // the newer version of the local buckets, when there is one
	// Global means installed for all users, which needs administrator rights
	// to update.
	Global bool
}

// Installed lists what scoop installed, with the updates the local buckets
// know of: it does not bring the buckets up to date.
func (s *Source) Installed(ctx context.Context) ([]App, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, err := s.export(ctx)
	if err != nil {
		return nil, err
	}
	st, _ := s.status(ctx, false)
	return installedApps(exp, st), nil
}

func installedApps(exp exported, st statusResult) []App {
	latest := map[string]string{}
	for _, r := range st.Rows {
		if r.Latest != "" {
			latest[strings.ToLower(r.Name)] = r.Latest
		}
	}
	out := make([]App, 0, len(exp.Apps))
	for _, a := range exp.Apps {
		if a.Name == "" {
			continue
		}
		global := strings.Contains(strings.ToLower(a.Info), "global install")
		out = append(out, App{Name: a.Name, Version: a.Version, Bucket: a.Source, Update: latest[strings.ToLower(a.Name)], Global: global})
	}
	return out
}

// Info is what a manifest says of an app.
type Info struct {
	Name        string
	Description string
	Version     string
	Website     string
	License     string
}

const infoScript = scoopPrelude + `
$i = & scoop info ([string]$P.Ref) 6>$null | Select-Object -First 1
if (-not $i) {
  Write-Failure 'not-found'
  exit 12
}
$license = $i.License
if ($license -isnot [string]) { $license = [string]($license | Out-String).Trim() }
Write-Result @{
  Name = [string]$i.Name
  Description = [string]$i.Description
  Version = [string]$i.Version
  Website = [string]$i.Website
  License = [string]$license
}
`

// Info reads the manifest of bucket/name with scoop info.
func (s *Source) Info(ctx context.Context, bucket, name string) (Info, error) {
	var out Info
	_, err := runner.PowerShell(ctx, infoScript, map[string]any{"Ref": bucket + "/" + name}, &out, runner.Options{Timeout: queryTimeout})
	var se *runner.ScriptError
	switch {
	case errors.As(err, &se) && se.ExitCode == codeMissing:
		return Info{}, errors.New("未安装 scoop")
	case errors.As(err, &se) && se.ExitCode == 12:
		return Info{}, errors.New("scoop 找不到这个软件")
	case err != nil:
		return Info{}, fmt.Errorf("读取 scoop 软件信息失败：%w", err)
	}
	return out, nil
}

// Recheck reads one app's state with the local buckets.
func (s *Source) Recheck(ctx context.Context, app catalog.App) (source.Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, err := s.export(ctx)
	if err != nil {
		return source.Status{}, err
	}
	st, stErr := s.status(ctx, false)
	return buildStatuses([]catalog.App{app}, exp, st, stErr)[app.ID], nil
}

// buildStatuses matches the catalog's apps with what scoop reports. An app
// is installed when scoop has an app of its name, whatever the bucket:
// scoop keeps one app of a name.
func buildStatuses(apps []catalog.App, exp exported, st statusResult, stErr error) map[string]source.Status {
	installed := map[string]int{}
	for i, a := range exp.Apps {
		installed[strings.ToLower(a.Name)] = i
	}
	buckets := map[string]bool{}
	for _, b := range exp.Buckets {
		buckets[strings.ToLower(b.Name)] = true
	}
	rows := map[string]statusRow{}
	for _, r := range st.Rows {
		rows[strings.ToLower(r.Name)] = r
	}
	out := map[string]source.Status{}
	for _, app := range apps {
		name := strings.ToLower(app.Source.Name)
		i, ok := installed[name]
		if !ok {
			s := source.Status{State: source.NotInstalled}
			if !buckets[strings.ToLower(app.Source.Bucket)] {
				s.Note = "将先添加 bucket：" + app.Source.Bucket
			}
			out[app.ID] = s
			continue
		}
		a := exp.Apps[i]
		s := source.Status{State: source.Installed, Version: a.Version}
		var notes []string
		if a.Source != "" && !strings.EqualFold(a.Source, app.Source.Bucket) {
			notes = append(notes, "通过 "+a.Source+" 安装")
		}
		if r, ok := rows[name]; ok {
			if r.Latest != "" {
				s.State, s.Latest = source.UpdateAvailable, r.Latest
			}
			if r.Info != "" {
				notes = append(notes, "scoop："+r.Info)
			}
		}
		switch {
		case stErr != nil:
			notes = append(notes, "检查更新失败："+stErr.Error())
		case st.UpdateFailed:
			notes = append(notes, "scoop update 失败，更新信息可能不是最新")
		}
		s.Note = strings.Join(notes, "；")
		out[app.ID] = s
	}
	return out
}

const searchScript = scoopPrelude + `
$q = [regex]::Escape([string]$P.Query)
$rows = @(& scoop search $q 6>$null | Select-Object -First 30 | ForEach-Object {
  [pscustomobject]@{ Name = [string]$_.Name; Version = [string]$_.Version; Source = [string]$_.Source }
})
Write-Result $rows
`

// Search finds apps in the buckets, matching the query as text.
func (s *Source) Search(ctx context.Context, query string) ([]source.SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	var rows []struct {
		Name    string
		Version string
		Source  string
	}
	if _, err := runner.PowerShell(ctx, searchScript, map[string]any{"Query": query}, &rows, runner.Options{Timeout: queryTimeout}); err != nil {
		return nil, fmt.Errorf("scoop 搜索失败：%w", err)
	}
	out := make([]source.SearchResult, 0, len(rows))
	for _, r := range rows {
		if r.Name == "" {
			continue
		}
		out = append(out, source.SearchResult{ID: r.Source + "/" + r.Name, Name: r.Name, Version: r.Version, Source: r.Source})
	}
	return out, nil
}

// installScript adds the app's bucket when it is missing, from scoop's
// known buckets or the app's bucket URL, then installs bucket/name.
const installScript = scoopPrelude + `
$bucket = [string]$P.Bucket
$name = [string]$P.Name
$url = [string]$P.BucketUrl
$added = @(& scoop bucket list 6>$null | ForEach-Object { [string]$_.Name })
if ($added -notcontains $bucket) {
  if ($url) {
    & scoop bucket add $bucket $url
  } else {
    $known = @(& scoop bucket known 6>$null | ForEach-Object { [string]$_ })
    if ($known -notcontains $bucket) {
      Write-Failure 'unknown-bucket'
      exit 10
    }
    & scoop bucket add $bucket
  }
  $added = @(& scoop bucket list 6>$null | ForEach-Object { [string]$_.Name })
  if ($added -notcontains $bucket) {
    Write-Failure 'bucket-add-failed'
    exit 11
  }
}
& scoop install "$bucket/$name"
exit $LASTEXITCODE
`

const upgradeScript = scoopPrelude + `
& scoop update ([string]$P.Name)
exit $LASTEXITCODE
`

// Install installs bucket/name, adding the bucket first when needed.
func (s *Source) Install(ctx context.Context, app catalog.App, log io.Writer) (source.Result, error) {
	in := map[string]any{"Bucket": app.Source.Bucket, "Name": app.Source.Name, "BucketUrl": app.Source.BucketURL}
	if log != nil {
		fmt.Fprintf(log, "> scoop install %s/%s\n\n", app.Source.Bucket, app.Source.Name)
	}
	return s.run(ctx, installScript, in, log, app)
}

// Upgrade updates the app.
func (s *Source) Upgrade(ctx context.Context, app catalog.App, log io.Writer) (source.Result, error) {
	if log != nil {
		fmt.Fprintf(log, "> scoop update %s\n\n", app.Source.Name)
	}
	return s.run(ctx, upgradeScript, map[string]any{"Name": app.Source.Name}, log, app)
}

func (s *Source) run(ctx context.Context, script string, in map[string]any, log io.Writer, app catalog.App) (source.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := runner.PowerShell(ctx, script, in, nil, runner.Options{Log: log})
	var se *runner.ScriptError
	if errors.As(err, &se) {
		switch se.ExitCode {
		case codeMissing:
			return source.Result{}, errors.New("未安装 scoop")
		case 10:
			return source.Result{}, fmt.Errorf("bucket「%s」不在 scoop 的已知列表中，请在软件里填写 bucket 地址", app.Source.Bucket)
		case 11:
			return source.Result{}, fmt.Errorf("添加 bucket「%s」失败（scoop 添加 bucket 需要 git），详见日志", app.Source.Bucket)
		}
		return source.Result{}, fmt.Errorf("scoop 以退出码 %d 结束，详见日志", se.ExitCode)
	}
	if err != nil {
		return source.Result{}, err
	}
	return source.Result{}, nil
}
