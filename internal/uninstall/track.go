package uninstall

import (
	"path/filepath"
	"strings"

	"appshelf/internal/procs"
)

// tracker tells the processes of one removal, as Bulk Crap Uninstaller
// does (BulkUninstallEntry.UninstallThread): the uninstaller started, what
// it starts, the copies of itself it leaves to do the work, and the
// programs of its directory started since. A process running before the
// removal began is never one, nor is a browser it opens.
type tracker struct {
	base map[uint32]string // pid → name of the processes running before
	seen map[uint32]string // pid → name of the processes of the removal; "" until seen
	// skip are the new processes found not to be of it, pid → name, for
	// their paths not to be read again.
	skip map[uint32]string
	dir  string // the program's own directory, "" when unknown or shared
	path func(pid uint32) (string, bool)
}

func newTracker(snap []procs.Proc, dir string, path func(uint32) (string, bool)) *tracker {
	k := &tracker{base: map[uint32]string{}, seen: map[uint32]string{}, skip: map[uint32]string{}, dir: dir, path: path}
	for _, p := range snap {
		k.base[p.PID] = strings.ToLower(p.Name)
	}
	return k
}

// add makes a process started for the removal one of it.
func (k *tracker) add(pid uint32) {
	k.seen[pid] = ""
	delete(k.base, pid)
}

// alive returns the processes of the removal among those running.
func (k *tracker) alive(snap []procs.Proc) []procs.Proc {
	in := map[uint32]bool{}
	for changed := true; changed; {
		changed = false
		for _, p := range snap {
			if in[p.PID] || !k.of(p) {
				continue
			}
			in[p.PID] = true
			k.seen[p.PID] = strings.ToLower(p.Name)
			changed = true
		}
	}
	var out []procs.Proc
	for _, p := range snap {
		if in[p.PID] {
			out = append(out, p)
		}
	}
	return out
}

func (k *tracker) of(p procs.Proc) bool {
	name := strings.ToLower(p.Name)
	if n, ok := k.base[p.PID]; ok && n == name {
		return false
	}
	if ignoredProcess[name] {
		return false
	}
	if n, ok := k.seen[p.PID]; ok && (n == "" || n == name) {
		return true
	}
	if _, ok := k.seen[p.Parent]; ok {
		// Its parent may have exited: NSIS's starts its copy and goes.
		return true
	}
	if copyName[name] {
		return true
	}
	if k.skip[p.PID] == name {
		return false
	}
	path, ok := k.path(p.PID)
	if ok && k.copyPath(path) {
		return true
	}
	k.skip[p.PID] = name
	return false
}

// copyPath reports where uninstallers' copies run: a program ending in
// .tmp (Inno Setup's _iu*.tmp, unins*.tmp), WiX bundles' .cr and .be
// directories, or the program's own directory.
func (k *tracker) copyPath(path string) bool {
	low := strings.ToLower(path)
	if filepath.Ext(low) == ".tmp" || strings.Contains(low, `\.cr\`) || strings.Contains(low, `\.be\`) {
		return true
	}
	return k.dir != "" && procs.Under([]string{path}, k.dir)
}

// copyName are the names uninstallers' copies run as: NSIS's Au_.exe,
// electron-builder's Un_A.exe and Un_B.exe, InstallShield's _isdel.exe.
var copyName = map[string]bool{"au_.exe": true, "un_a.exe": true, "un_b.exe": true, "_isdel.exe": true}

// ignoredProcess are what uninstallers open as they end, a survey page in
// a browser, and Windows' own: waiting for them would never end.
var ignoredProcess = map[string]bool{
	"chrome.exe": true, "msedge.exe": true, "firefox.exe": true, "iexplore.exe": true,
	"opera.exe": true, "brave.exe": true, "vivaldi.exe": true,
	"explorer.exe": true, "conhost.exe": true,
}

func names(list []procs.Proc) string {
	var out []string
	seen := map[string]bool{}
	for _, p := range list {
		if !seen[strings.ToLower(p.Name)] {
			seen[strings.ToLower(p.Name)] = true
			out = append(out, p.Name)
		}
	}
	if len(out) > 3 {
		out = append(out[:3], "…")
	}
	return strings.Join(out, "、")
}
