//go:build windows

package usage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"appshelf/internal/procs"
	"appshelf/internal/runner"
)

const userAssist = `Software\Microsoft\Windows\CurrentVersion\Explorer\UserAssist`

// shortcutScript resolves shortcuts: those UserAssist names, and the Start
// menu's and the taskbar's, whose AppUserModelIDs tell what programs the
// launches recorded by ID are. Folders are opened once each: ParseName on
// a fresh one is slow.
const shortcutScript = `
$ws = New-Object -ComObject WScript.Shell
$sh = New-Object -ComObject Shell.Application
$folders = @{}
$files = @{}
foreach ($l in @($P.Links)) { if ($l -and (Test-Path -LiteralPath ([string]$l))) { $files[[string]$l] = $true } }
$dirs = @([Environment]::GetFolderPath('CommonPrograms'), [Environment]::GetFolderPath('Programs'), (Join-Path $env:APPDATA 'Microsoft\Internet Explorer\Quick Launch\User Pinned'))
foreach ($d in $dirs) {
  if (-not $d -or -not (Test-Path -LiteralPath $d)) { continue }
  Get-ChildItem -LiteralPath $d -Recurse -Filter *.lnk -File -ErrorAction SilentlyContinue | ForEach-Object { $files[$_.FullName] = $true }
}
$found = @()
foreach ($f in $files.Keys) {
  $target = ''
  $id = ''
  try { $target = [string]$ws.CreateShortcut($f).TargetPath } catch {}
  try {
    $dir = [IO.Path]::GetDirectoryName($f)
    $ns = $folders[$dir]
    if (-not $ns) { $ns = $sh.Namespace($dir); $folders[$dir] = $ns }
    $item = $ns.ParseName([IO.Path]::GetFileName($f))
    if ($item) { $id = [string]$item.ExtendedProperty('System.AppUserModel.ID') }
  } catch {}
  $found += [pscustomobject]@{ Path = $f; Target = $target; AppID = $id }
}
Write-Result @{ Links = $found }
`

type shortcut struct {
	Path, Target, AppID string
}

// Read reads what Windows recorded of the user's launches and the
// processes running. It returns what it could read with the error of what
// it could not.
func Read(ctx context.Context) (Record, error) {
	rec := Record{At: time.Now()}
	raw, err := readUserAssist()
	if err != nil {
		return rec, fmt.Errorf("读取程序的打开记录失败：%w", err)
	}
	folders := map[string]string{}
	folder := func(guid string) string {
		if p, ok := folders[guid]; ok {
			return p
		}
		p := ""
		if g, err := windows.GUIDFromString(guid); err == nil {
			id := windows.KNOWNFOLDERID(g)
			p, _ = windows.KnownFolderPath(&id, 0)
		}
		folders[guid] = p
		return p
	}
	type pending struct {
		link, appID string
		at          time.Time
	}
	var links []string
	var later []pending
	for _, v := range raw {
		n, ok := parseName(v.name)
		if !ok {
			continue
		}
		if n.AppID != "" {
			if strings.Contains(n.AppID, "!") {
				rec.Launches = append(rec.Launches, Launch{AppID: n.AppID, At: v.at})
			} else {
				later = append(later, pending{appID: n.AppID, at: v.at})
			}
			continue
		}
		path := n.Rest
		if n.Folder != "" {
			base := folder(n.Folder)
			if base == "" {
				continue
			}
			path = base + `\` + n.Rest
		}
		if isShortcut(path) {
			links = append(links, path)
			later = append(later, pending{link: path, at: v.at})
			continue
		}
		rec.Launches = append(rec.Launches, Launch{Path: path, At: v.at})
	}
	if rec.Running, err = procs.Paths(); err != nil {
		rec.Running = nil
	}
	if len(later) == 0 {
		return rec, nil
	}
	var got struct{ Links []shortcut }
	if _, err := runner.PowerShell(ctx, shortcutScript, map[string]any{"Links": links}, &got, runner.Options{Timeout: time.Minute}); err != nil {
		return rec, fmt.Errorf("读取快捷方式失败：%w", err)
	}
	byPath := map[string]shortcut{}
	byID := map[string]string{}
	for _, s := range got.Links {
		byPath[strings.ToLower(s.Path)] = s
		if s.AppID != "" && s.Target != "" {
			byID[strings.ToLower(s.AppID)] = s.Target
		}
	}
	for _, p := range later {
		l := Launch{At: p.at}
		if p.link != "" {
			s := byPath[strings.ToLower(p.link)]
			l.Path, l.AppID = s.Target, s.AppID
		} else {
			l.AppID, l.Path = p.appID, byID[strings.ToLower(p.appID)]
		}
		if l.Path != "" || strings.Contains(l.AppID, "!") {
			rec.Launches = append(rec.Launches, l)
		}
	}
	return rec, nil
}

type rawValue struct {
	name string
	at   time.Time
}

// readUserAssist reads the values of every UserAssist counter, their names
// decoded, those with a time of last run.
func readUserAssist() ([]rawValue, error) {
	root, err := registry.OpenKey(registry.CURRENT_USER, userAssist, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		if err == registry.ErrNotExist {
			return nil, nil
		}
		return nil, err
	}
	defer root.Close()
	guids, err := root.ReadSubKeyNames(0)
	if err != nil {
		return nil, err
	}
	var out []rawValue
	for _, g := range guids {
		k, err := registry.OpenKey(root, g+`\Count`, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		names, _ := k.ReadValueNames(0)
		for _, n := range names {
			data, _, err := k.GetBinaryValue(n)
			if err != nil {
				continue
			}
			if at := lastRun(data); !at.IsZero() {
				out = append(out, rawValue{name: rot13(n), at: at})
			}
		}
		k.Close()
	}
	return out, nil
}
