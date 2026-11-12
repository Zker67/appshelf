package procs

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Installers follows the installer processes an install or update leaves
// running once the command AppShelf waited for has exited: Inno Setup's
// setup.tmp, a WiX bundle's copy in a .cr or .be directory, Squirrel's
// Update.exe in SquirrelTemp, or a setup an installer unpacked in the
// temporary directory. They are told by where they run, not as children,
// for the program an installer opens as it ends not to be waited for.
type Installers struct {
	sys   System
	temps []string
	base  map[uint32]string // pid → name of the processes running before
	seen  map[uint32]string // pid → name of the installers found
	skip  map[uint32]string // pid → name of the new processes that are not
	// Times are how Wait waits.
	Times WaitTimes
}

// System is what Installers reads of Windows, replaced in tests.
type System struct {
	Snapshot func() ([]Proc, error)
	Path     func(pid uint32) (string, bool)
	Usage    func(pid uint32) (Usage, bool)
	// Windows, when set, tells the installers showing a window: they wait
	// for the user, and count as working.
	Windows func(pids map[uint32]bool) []Window
}

// Live reads the processes of this machine.
func Live() System {
	return System{Snapshot: Snapshot, Path: Path, Usage: UsageOf, Windows: Windows}
}

// WaitTimes are how Wait waits.
type WaitTimes struct {
	Poll time.Duration // how often the processes are looked at
	// Grace is how long, after the installers found have ended, another
	// may start, as the next step of a chain.
	Grace time.Duration
	// Idle is how long none of them may work before waiting stops; Max
	// how long waiting may take in all.
	Idle, Max time.Duration
}

// DefaultWaitTimes are the times of WatchInstallers.
var DefaultWaitTimes = WaitTimes{Poll: time.Second, Grace: 3 * time.Second, Idle: 2 * time.Minute, Max: 30 * time.Minute}

// TempDirs are where installers unpack and run their setups: the user's
// temporary directory and Windows'.
func TempDirs() []string {
	dirs := []string{os.TempDir()}
	if root := os.Getenv("SystemRoot"); root != "" {
		dirs = append(dirs, filepath.Join(root, "Temp"), filepath.Join(root, "SystemTemp"))
	}
	return dirs
}

// WatchInstallers notes the processes running now, before an install or
// update starts: none of them is ever one of its installers.
func WatchInstallers(sys System, temps []string) *Installers {
	w := &Installers{sys: sys, temps: temps, base: map[uint32]string{}, seen: map[uint32]string{}, skip: map[uint32]string{}, Times: DefaultWaitTimes}
	if snap, err := sys.Snapshot(); err == nil {
		for _, p := range snap {
			w.base[p.PID] = strings.ToLower(p.Name)
		}
	}
	return w
}

// Alive returns the installers started since WatchInstallers that run.
func (w *Installers) Alive() ([]Proc, error) {
	snap, err := w.sys.Snapshot()
	if err != nil {
		return nil, err
	}
	var out []Proc
	for _, p := range snap {
		if w.of(p) {
			out = append(out, p)
		}
	}
	return out, nil
}

func (w *Installers) of(p Proc) bool {
	name := strings.ToLower(p.Name)
	if n, ok := w.base[p.PID]; ok && n == name {
		return false
	}
	if n, ok := w.seen[p.PID]; ok && n == name {
		return true
	}
	if n, ok := w.skip[p.PID]; ok && n == name {
		return false
	}
	path, ok := w.sys.Path(p.PID)
	if ok && !ignoredInstaller[name] && w.installerPath(path) {
		w.seen[p.PID] = name
		return true
	}
	w.skip[p.PID] = name
	return false
}

// installerPath reports where installers' setups run: a program ending in
// .tmp, a WiX bundle's .cr and .be directories, Squirrel's SquirrelTemp,
// or a temporary directory.
func (w *Installers) installerPath(path string) bool {
	low := strings.ToLower(strings.ReplaceAll(path, "/", `\`))
	if filepath.Ext(low) == ".tmp" || strings.Contains(low, `\.cr\`) || strings.Contains(low, `\.be\`) || strings.Contains(low, `\squirreltemp\`) {
		return true
	}
	return slices.ContainsFunc(w.temps, func(dir string) bool { return Under([]string{path}, dir) })
}

// ignoredInstaller are what installers open as they end, a page in a
// browser, and Windows' own: waiting for them would never end.
var ignoredInstaller = map[string]bool{
	"chrome.exe": true, "msedge.exe": true, "firefox.exe": true, "iexplore.exe": true,
	"opera.exe": true, "brave.exe": true, "vivaldi.exe": true,
	"explorer.exe": true, "conhost.exe": true,
}

// WaitEnd is how Wait ended.
type WaitEnd int

const (
	WaitDone    WaitEnd = iota // no installer runs, or none can be told
	WaitIdle                   // none of them worked for Times.Idle
	WaitTimeout                // they still ran after Times.Max
	WaitAborted                // ctx ended
)

// A process works when it uses more than 1% of a processor or moves more
// than 10 KB a second, as uninstall measures it.
const (
	workCPU = 0.01
	workIO  = 10 * 1024
)

// Wait waits for the installers started since WatchInstallers to end,
// calling report each time those it waits for change, with them. It
// returns at once when none ran. One showing a window never counts as
// idle: the user may be reading it.
func (w *Installers) Wait(ctx context.Context, report func(alive []Proc)) WaitEnd {
	t := w.Times
	start := time.Now()
	lastWork := start
	var (
		emptySince time.Time
		shown      string
		prev       map[uint32]Usage
		prevAt     time.Time
	)
	for {
		alive, err := w.Alive()
		if err != nil {
			return WaitDone
		}
		now := time.Now()
		if len(alive) == 0 {
			if len(w.seen) == 0 {
				return WaitDone
			}
			if emptySince.IsZero() {
				emptySince = now
			}
			if now.Sub(emptySince) >= t.Grace {
				return WaitDone
			}
		} else {
			emptySince = time.Time{}
			if names := Names(alive); names != shown {
				shown = names
				report(alive)
			}
			cur := map[uint32]Usage{}
			for _, p := range alive {
				u, ok := w.sys.Usage(p.PID)
				if !ok {
					lastWork = now // what cannot be read may work
					continue
				}
				cur[p.PID] = u
				before, seen := prev[p.PID]
				if !seen || working(before, u, now.Sub(prevAt)) {
					lastWork = now
				}
			}
			prev, prevAt = cur, now
			if w.sys.Windows != nil && len(w.sys.Windows(pidsOf(alive))) > 0 {
				lastWork = now // its window may wait for the user
			}
			if now.Sub(lastWork) >= t.Idle {
				return WaitIdle
			}
			if now.Sub(start) >= t.Max {
				return WaitTimeout
			}
		}
		timer := time.NewTimer(t.Poll)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return WaitAborted
		}
	}
}

func working(before, now Usage, dt time.Duration) bool {
	if dt <= 0 {
		return true
	}
	cpu := float64(now.CPU-before.CPU) / float64(dt)
	io := float64(now.IO-before.IO) / dt.Seconds()
	return cpu > workCPU || io > workIO
}

// Names lists the names of processes once each, at most three, as
// "setup.tmp、Update.exe".
func Names(list []Proc) string {
	var out []string
	seen := map[string]bool{}
	for _, p := range list {
		if low := strings.ToLower(p.Name); !seen[low] {
			seen[low] = true
			out = append(out, p.Name)
		}
	}
	if len(out) > 3 {
		out = append(out[:3], "…")
	}
	return strings.Join(out, "、")
}
