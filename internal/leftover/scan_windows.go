//go:build windows

package leftover

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows/registry"

	"appshelf/internal/detect"
	"appshelf/internal/runner"
)

// registry lists the keys of RegRoot named as the program, directly or
// under its publisher's key.
func (f *finder) registry() {
	root := f.cfg.RegRoot
	if root == "" {
		return
	}
	names, err := subkeys(root)
	if err != nil {
		return
	}
	for _, n := range names {
		path := root + `\` + n
		if m := f.match(n); m != nil {
			f.regKey(path, []Reason{*m})
			continue
		}
		if f.publisher == "" || normPublisher(n) != f.publisher {
			continue
		}
		subs, err := subkeys(path)
		if err != nil {
			continue
		}
		found := 0
		for _, s := range subs {
			if m := f.match(s); m != nil {
				f.regKey(path+`\`+s, []Reason{*m, underPublisher})
				found++
			}
		}
		if found > 0 && found == len(subs) && !f.publisherUsed() && !hasValues(path) {
			f.regKey(path, []Reason{underPublisher, {"只剩这个软件的设置", 2}})
		}
	}
}

func (f *finder) regKey(path string, reasons []Reason) {
	if f.nameUsedBy(path[strings.LastIndexByte(path, '\\')+1:]) {
		reasons = append(reasons, nameUsed)
	}
	f.add(Item{Kind: RegKey, Path: path, Reasons: reasons})
}

func subkeys(path string) ([]string, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil, err
	}
	defer k.Close()
	return k.ReadSubKeyNames(-1)
}

func hasValues(path string) bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
	if err != nil {
		return true
	}
	defer k.Close()
	names, err := k.ReadValueNames(-1)
	return err != nil || len(names) > 0
}

// run lists the programs started at login that are the program's: in its
// directory, or named as it and gone.
func (f *finder) run() {
	if f.cfg.RunKey == "" {
		return
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, f.cfg.RunKey, registry.QUERY_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	names, err := k.ReadValueNames(-1)
	if err != nil {
		return
	}
	loc := cleanPath(f.p.Location)
	for _, n := range names {
		cmd, _, err := k.GetStringValue(n)
		if err != nil {
			continue
		}
		file := detect.CommandFile(cmd)
		var reasons []Reason
		if loc != "" && detect.OwnDirectory(f.p.Location) && within(cleanPath(file), loc) {
			reasons = append(reasons, explicitRun)
		} else if m := f.match(n); m != nil && file != "" && filepath.IsAbs(file) && !exists(file) {
			reasons = append(reasons, *m, targetGone)
		}
		if len(reasons) > 0 {
			f.add(Item{Kind: RegValue, Path: f.cfg.RunKey, Value: n, Reasons: reasons})
		}
	}
}

// shortcuts lists the Start Menu's and the Desktop's shortcuts that point
// into the program's directory, or are named as it and point to nothing,
// and the Start Menu's folders left with nothing else.
func (f *finder) shortcuts(ctx context.Context) {
	type folder struct {
		path  string
		links []string
		other bool // holds more than shortcuts
	}
	var links []string
	var folders []*folder
	for _, root := range f.cfg.ShortcutDirs {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			path := filepath.Join(root, e.Name())
			if !e.IsDir() {
				if strings.EqualFold(filepath.Ext(e.Name()), ".lnk") {
					links = append(links, path)
				}
				continue
			}
			if f.match(e.Name()) == nil {
				continue
			}
			fo := &folder{path: path}
			filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				if strings.EqualFold(filepath.Ext(p), ".lnk") {
					fo.links = append(fo.links, p)
				} else if !strings.EqualFold(d.Name(), "desktop.ini") {
					fo.other = true
				}
				return nil
			})
			folders = append(folders, fo)
			links = append(links, fo.links...)
		}
	}
	if len(links) == 0 && len(folders) == 0 {
		return
	}
	resolve := f.cfg.links
	if resolve == nil {
		resolve = readLinks
	}
	targets := resolve(ctx, links)
	loc := cleanPath(f.p.Location)
	ours := map[string]bool{}
	for _, l := range links {
		target := targets[strings.ToLower(l)]
		var reasons []Reason
		name := strings.TrimSuffix(filepath.Base(l), filepath.Ext(l))
		switch {
		case loc != "" && detect.OwnDirectory(f.p.Location) && target != "" && within(cleanPath(target), loc):
			reasons = append(reasons, explicitLink)
		case target != "" && filepath.IsAbs(target) && !exists(target):
			if m := f.match(name); m != nil {
				reasons = append(reasons, *m, targetGone)
			}
		}
		if len(reasons) == 0 || Protected(f.cfg, l, f.others, f.p.Location) {
			continue
		}
		ours[strings.ToLower(l)] = true
		f.add(Item{Kind: Shortcut, Path: l, Reasons: reasons})
	}
	for _, fo := range folders {
		all := !fo.other
		for _, l := range fo.links {
			all = all && ours[strings.ToLower(l)]
		}
		if !all || f.protected(fo.path) {
			continue
		}
		reasons := []Reason{*f.match(filepath.Base(fo.path))}
		if len(fo.links) == 0 {
			reasons = append(reasons, emptyDir)
		} else {
			reasons = append(reasons, onlyLinks)
		}
		f.add(Item{Kind: Dir, Path: fo.path, Reasons: reasons})
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

const linksScript = `
$shell = New-Object -ComObject WScript.Shell
# Not $out: PowerShell's names ignore case, and $Out is the result's file.
$targets = @{}
foreach ($p in @($P.Paths)) {
  if (-not $p) { continue }
  try { $targets[[string]$p] = [string]$shell.CreateShortcut([string]$p).TargetPath } catch {}
}
Write-Result $targets
`

// readLinks reads the targets of shortcuts, by their path in lower case.
func readLinks(ctx context.Context, paths []string) map[string]string {
	out := map[string]string{}
	if len(paths) == 0 {
		return out
	}
	var got map[string]string
	if _, err := runner.PowerShell(ctx, linksScript, map[string]any{"Paths": paths}, &got, runner.Options{Timeout: 2 * time.Minute}); err != nil {
		return out
	}
	for p, t := range got {
		out[strings.ToLower(p)] = t
	}
	return out
}
