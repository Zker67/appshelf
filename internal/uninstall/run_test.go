package uninstall

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"appshelf/internal/catalog"
	"appshelf/internal/detect"
	"appshelf/internal/procs"
	"appshelf/internal/runner"
	"appshelf/internal/source"
)

// fakeProc is a process of fakeSys.
type fakeProc struct {
	pid, parent uint32
	name, path  string
	// life is how many snapshots it is seen in before it exits; < 0 never.
	life int
	// busy processes work between readings; unmeasured ones cannot be read.
	busy, unmeasured bool
	cpu              time.Duration
	// title, when set, is its window's.
	title string
	text  []string
	// removesKey takes the program's registry key away as it exits.
	removesKey bool
	// spawn are what it starts as it starts.
	spawn []fakeProc
}

// fakeSys plays Windows for a removal: processes with their parents,
// activity and windows, the program's registry key, Windows Installer.
type fakeSys struct {
	mu      sync.Mutex
	present bool
	procs   []*fakeProc
	nextPID uint32
	// root is the uninstaller that starts, with code its exit code; codes,
	// when set, are the exit codes of the runs in turn.
	root     fakeProc
	code     uint32
	codes    []uint32
	startErr error
	// alsoStart start beside the uninstaller, not as its children.
	alsoStart []fakeProc
	started   []string // base name and parameters
	files     []string // full path of what started
	ended     []uint32
	denied    map[uint32]bool
	fronted   []uintptr
	msiBusy   int // how many more times Windows Installer reads busy
	pending   []string
}

func (f *fakeSys) runner() *Runner {
	r := New(Options{}, nil, nil)
	if f.nextPID == 0 {
		f.nextPID = 1000
	}
	r.sys = system{
		start: func(file, params string, elevate bool) (process, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.startErr != nil {
				return nil, f.startErr
			}
			f.started = append(f.started, filepath.Base(file)+" "+params)
			f.files = append(f.files, file)
			if len(f.codes) > 0 {
				f.code, f.codes = f.codes[0], f.codes[1:]
			}
			root := f.root
			if root.name == "" {
				root.name = filepath.Base(file)
			}
			root.path = file
			root.parent = 1 // AppShelf
			pid := f.addLocked(root)
			for _, p := range f.alsoStart {
				p.parent = 4242
				f.addLocked(p)
			}
			return fakeProcess{f, pid}, nil
		},
		snapshot: f.snapshot,
		path: func(pid uint32) (string, bool) {
			f.mu.Lock()
			defer f.mu.Unlock()
			if p := f.findLocked(pid); p != nil && p.path != "" {
				return p.path, true
			}
			return "", false
		},
		usage: func(pid uint32) (procs.Usage, bool) {
			f.mu.Lock()
			defer f.mu.Unlock()
			p := f.findLocked(pid)
			if p == nil || p.unmeasured {
				return procs.Usage{}, false
			}
			if p.busy {
				p.cpu += time.Hour
			}
			return procs.Usage{CPU: p.cpu}, true
		},
		windows: func(pids map[uint32]bool) []procs.Window {
			f.mu.Lock()
			defer f.mu.Unlock()
			var out []procs.Window
			for _, p := range f.procs {
				if pids[p.pid] && p.title != "" {
					out = append(out, procs.Window{PID: p.pid, Handle: uintptr(p.pid) * 10, Title: p.title, Text: p.text})
				}
			}
			return out
		},
		front: func(h uintptr) bool {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.fronted = append(f.fronted, h)
			return true
		},
		end: func(pid uint32) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.denied[pid] {
				return procs.ErrDenied
			}
			f.ended = append(f.ended, pid)
			f.procs = slices.DeleteFunc(f.procs, func(p *fakeProc) bool { return p.pid == pid })
			return nil
		},
		msiBusy: func() bool {
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.msiBusy > 0 {
				f.msiBusy--
				return true
			}
			return false
		},
	}
	r.lookup = func(scope, arch, name string) (detect.Entry, bool, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		return detect.Entry{UninstallString: `C:\Gone\uninst.exe`}, f.present, nil
	}
	r.pending = func() []string {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.pending
	}
	r.t = timings{poll: time.Millisecond, grace: 20 * time.Millisecond, userIdle: 5 * time.Millisecond, stallIdle: 15 * time.Millisecond, idleMax: 2 * time.Second, wingetEvery: time.Millisecond}
	return r
}

// add starts a process, and what it spawns, returning its ID.
func (f *fakeSys) add(p fakeProc) uint32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.addLocked(p)
}

func (f *fakeSys) addLocked(p fakeProc) uint32 {
	if p.pid == 0 {
		f.nextPID++
		p.pid = f.nextPID
	}
	spawn := p.spawn
	p.spawn = nil
	f.procs = append(f.procs, &p)
	for _, c := range spawn {
		c.parent = p.pid
		f.addLocked(c)
	}
	return p.pid
}

func (f *fakeSys) findLocked(pid uint32) *fakeProc {
	for _, p := range f.procs {
		if p.pid == pid {
			return p
		}
	}
	return nil
}

// snapshot lists the processes, those whose life is over exiting first.
func (f *fakeSys) snapshot() ([]procs.Proc, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.procs = slices.DeleteFunc(f.procs, func(p *fakeProc) bool {
		if p.life != 0 {
			return false
		}
		if p.removesKey {
			f.present = false
		}
		return true
	})
	var out []procs.Proc
	for _, p := range f.procs {
		if p.life > 0 {
			p.life--
		}
		out = append(out, procs.Proc{PID: p.pid, Parent: p.parent, Name: p.name})
	}
	return out, nil
}

func (f *fakeSys) running(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.ContainsFunc(f.procs, func(p *fakeProc) bool { return strings.EqualFold(p.name, name) })
}

type fakeProcess struct {
	f   *fakeSys
	pid uint32
}

func (p fakeProcess) PID() uint32 { return p.pid }
func (p fakeProcess) Close()      {}
func (p fakeProcess) Exited() (uint32, bool, error) {
	p.f.mu.Lock()
	defer p.f.mu.Unlock()
	if p.f.findLocked(p.pid) != nil {
		return 0, false, nil
	}
	return p.f.code, true, nil
}

// recorder is a task's log: its output and the progress reported.
type recorder struct {
	mu       sync.Mutex
	out      strings.Builder
	progress []source.Progress
}

func (l *recorder) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.out.Write(b)
}

func (l *recorder) ReportProgress(p source.Progress) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.progress = append(l.progress, p)
}

func (l *recorder) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.out.String()
}

// waited returns the last progress waiting so, and whether there was one.
func (l *recorder) waited(wait string) (source.Progress, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := len(l.progress) - 1; i >= 0; i-- {
		if l.progress[i].Wait == wait {
			return l.progress[i], true
		}
	}
	return source.Progress{}, false
}

func (l *recorder) until(t *testing.T, wait string) source.Progress {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if p, ok := l.waited(wait); ok {
			return p
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("never waited %q; log:\n%s", wait, l.String())
	return source.Progress{}
}

// nsisFile writes an NSIS uninstaller named name in a new directory.
func nsisFile(t *testing.T, name string) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "App", name)
	os.MkdirAll(filepath.Dir(exe), 0o755)
	if err := os.WriteFile(exe, []byte("MZ Nullsoft"), 0o644); err != nil {
		t.Fatal(err)
	}
	return exe
}

// plainFile writes an uninstaller of no installer AppShelf knows.
func plainFile(t *testing.T) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "remove.exe")
	if err := os.WriteFile(exe, []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}
	return exe
}

// run runs the removal of tg in the background, returning what it returns
// and the commands to send it.
func run(r *Runner, tg Target, interactive bool, log io.Writer) (chan<- Command, <-chan error) {
	cmds := make(chan Command, 4)
	done := make(chan error, 1)
	go func() {
		_, err := r.Run(WithCommands(context.Background(), cmds), tg, interactive, log)
		done <- err
	}()
	return cmds, done
}

func wait(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return")
	}
	return nil
}

// An NSIS uninstaller without arguments runs from a copy with _?=, which
// does the work itself and returns its exit code; the copy goes after.
func TestRunNSISFromACopy(t *testing.T) {
	exe := nsisFile(t, "uninst.exe")
	f := &fakeSys{present: true, root: fakeProc{life: 3, busy: true, removesKey: true}}
	r := f.runner()
	tg := regTarget(detect.Entry{DisplayName: "App", UninstallString: `"` + exe + `"`})
	log := &recorder{}
	if _, err := r.Run(context.Background(), tg, false, log); err != nil {
		t.Fatalf("run: %v\n%s", err, log.String())
	}
	want := "uninst.exe /S _?=" + filepath.Dir(exe)
	if len(f.started) != 1 || f.started[0] != want {
		t.Fatalf("started %q, want %q", f.started, want)
	}
	copied := f.files[0]
	if copied == exe || !strings.Contains(copied, "AppShelf-uninstall-") {
		t.Fatalf("ran %s, not a copy", copied)
	}
	if _, err := os.Stat(filepath.Dir(copied)); !os.IsNotExist(err) {
		t.Errorf("the copy's directory is left: %v", err)
	}
	if _, err := os.Stat(exe); err != nil {
		t.Errorf("the uninstaller itself is touched: %v", err)
	}
	if f.present || !strings.Contains(log.String(), "副本运行") {
		t.Errorf("present %v, log:\n%s", f.present, log.String())
	}
}

// An NSIS uninstaller with arguments of its own runs as it is: Run waits
// for the copy it starts, whose parent is gone by then.
func TestRunWaitsForTheUninstallersCopy(t *testing.T) {
	exe := nsisFile(t, "uninst.exe")
	f := &fakeSys{present: true, root: fakeProc{life: 0, spawn: []fakeProc{{name: "Au_.exe", path: `C:\Temp\~nsu.tmp\Au_.exe`, life: 4, busy: true, removesKey: true}}}}
	r := f.runner()
	tg := regTarget(detect.Entry{DisplayName: "App", UninstallString: `"` + exe + `" /currentuser`})
	log := &recorder{}
	if _, err := r.Run(context.Background(), tg, false, log); err != nil {
		t.Fatalf("run: %v", err)
	}
	if f.started[0] != "uninst.exe /currentuser /S" || f.files[0] != exe {
		t.Fatalf("started %q from %q", f.started, f.files)
	}
	if f.present || f.running("Au_.exe") {
		t.Fatal("Run returned before the copy ended")
	}
	if !strings.Contains(log.String(), "等待卸载程序结束：Au_.exe") {
		t.Errorf("log:\n%s", log.String())
	}
	if p, ok := log.waited(WaitWorking); !ok || !strings.Contains(p.Stage, "Au_.exe") {
		t.Errorf("progress = %+v", log.progress)
	}
}

// What ran before the removal, a browser the uninstaller opens and a
// program started beside it do not hold it up.
func TestRunWaitsOnlyForItsOwnProcesses(t *testing.T) {
	dir := `C:\AppShelfTest\App` // the processes' paths only
	exe := dir + `\remove.exe`
	f := &fakeSys{present: true, root: fakeProc{life: 1, spawn: []fakeProc{{name: "msedge.exe", path: `C:\Edge\msedge.exe`, life: -1}}}}
	f.alsoStart = []fakeProc{{name: "notepad.exe", path: `C:\Windows\notepad.exe`, life: -1}}
	f.add(fakeProc{pid: 50, name: "Au_.exe", path: `C:\Temp\other\Au_.exe`, life: -1}) // another program's removal
	f.add(fakeProc{pid: 51, name: "app.exe", path: dir + `\app.exe`, life: -1})        // the program, open
	r := f.runner()
	tg := regTarget(detect.Entry{DisplayName: "App", UninstallString: `"` + exe + `" /S`, QuietUninstallString: `"` + exe + `" /S`})
	tg.Location = dir
	_, done := run(r, tg, false, &recorder{})
	start := time.Now()
	if err := wait(t, done); err != nil {
		t.Fatalf("run: %v", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("waited %v for processes not of the removal", d)
	}
}

// Programs of the program's directory started during the removal are of
// it: an uninstaller may hand over to one.
func TestRunWaitsForItsDirectory(t *testing.T) {
	dir := `C:\AppShelfTest\App`
	exe := dir + `\remove.exe`
	f := &fakeSys{present: true, root: fakeProc{life: 0}}
	f.alsoStart = []fakeProc{{name: "helper.exe", path: dir + `\helper.exe`, life: 5, busy: true, removesKey: true}}
	r := f.runner()
	tg := regTarget(detect.Entry{DisplayName: "App", UninstallString: `"` + exe + `" /S`, QuietUninstallString: `"` + exe + `" /S`})
	tg.Location = dir
	if _, err := r.Run(context.Background(), tg, false, io.Discard); err != nil {
		t.Fatal(err)
	}
	if f.present {
		t.Error("Run returned before the program's helper ended")
	}
}

// fakeWinget is winget removing a program: the uninstaller it runs leaves
// a copy, not AppShelf's child, that takes the program away.
type fakeWinget struct {
	f    *fakeSys
	copy fakeProc
}

func (w fakeWinget) Uninstall(context.Context, string, string, bool, io.Writer) (source.Result, error) {
	w.f.add(w.copy)
	return source.Result{}, nil
}

func (w fakeWinget) Recheck(context.Context, catalog.App) (source.Status, error) {
	w.f.mu.Lock()
	defer w.f.mu.Unlock()
	if w.f.present {
		return source.Status{State: source.Installed}, nil
	}
	return source.Status{State: source.NotInstalled}, nil
}

// winget says an NSIS program removed as its uninstaller exits, while the
// copy of it still works: Run waits for the copy, with the program's
// registry key or without.
func TestRunWaitsForTheCopyAfterWinget(t *testing.T) {
	for _, withReg := range []bool{true, false} {
		f := &fakeSys{present: true}
		r := f.runner()
		r.winget = fakeWinget{f, fakeProc{name: "Un_A.exe", path: `C:\Temp\Un_A.exe`, parent: 4242, life: 4, busy: true, removesKey: true}}
		tg := Target{Key: "winget:upscayl.upscayl", Name: "Upscayl", Method: ByWinget, WingetID: "Upscayl.Upscayl", WingetSource: "winget"}
		if withReg {
			tg.Reg = &detect.Entry{Scope: "Machine", Arch: "X64", Name: "upscayl-uuid", DisplayName: "Upscayl"}
		}
		log := &recorder{}
		if _, err := r.Run(context.Background(), tg, false, log); err != nil {
			t.Fatalf("reg %v: %v", withReg, err)
		}
		if f.present || f.running("Un_A.exe") {
			t.Fatalf("reg %v: Run returned before the copy ended", withReg)
		}
		if !strings.Contains(log.String(), "等待卸载程序结束：Un_A.exe") {
			t.Errorf("reg %v: log = %s", withReg, log.String())
		}
	}
}

// With its window, an uninstaller waits for the user; stopping the wait
// returns at once, leaving it running.
func TestRunWaitsForTheUserAndStops(t *testing.T) {
	exe := plainFile(t)
	f := &fakeSys{present: true, root: fakeProc{life: -1, title: "App 卸载", text: []string{"确定要卸载 App 吗？"}}}
	r := f.runner()
	tg := regTarget(detect.Entry{DisplayName: "App", UninstallString: `"` + exe + `"`})
	log := &recorder{}
	cmds, done := run(r, tg, false, log)
	p := log.until(t, WaitUser)
	if !p.Window || p.Prompt != "「App 卸载」确定要卸载 App 吗？" || p.Stage != "等待你在卸载窗口里完成操作" {
		t.Errorf("progress = %+v", p)
	}
	cmds <- ShowWindow
	cmds <- StopWaiting
	err := wait(t, done)
	if !errors.Is(err, errStopped) {
		t.Fatalf("err = %v", err)
	}
	if len(f.fronted) != 1 || len(f.ended) != 0 || !f.present {
		t.Errorf("fronted %v, ended %v", f.fronted, f.ended)
	}
	if !strings.Contains(log.String(), "你停止了等待") {
		t.Errorf("log:\n%s", log.String())
	}
}

// A quiet uninstaller that shows a window and does nothing waits for the
// user; one working with its progress window does not.
func TestRunQuietWindow(t *testing.T) {
	exe := nsisFile(t, "setup.exe")
	tg := regTarget(detect.Entry{DisplayName: "App", UninstallString: `"` + exe + `"`})

	f := &fakeSys{present: true, root: fakeProc{life: -1, title: "App", text: []string{"请先关闭 App"}}}
	r := f.runner()
	log := &recorder{}
	cmds, done := run(r, tg, false, log)
	p := log.until(t, WaitUser)
	if p.Stage != "卸载程序弹出了窗口，可能在等你操作" || !strings.Contains(p.Prompt, "请先关闭 App") {
		t.Errorf("progress = %+v", p)
	}
	cmds <- StopWaiting
	wait(t, done)

	f = &fakeSys{present: true, root: fakeProc{life: 30, busy: true, title: "正在卸载", removesKey: true}}
	log = &recorder{}
	if _, err := f.runner().Run(context.Background(), tg, false, log); err != nil {
		t.Fatal(err)
	}
	if _, ok := log.waited(WaitUser); ok {
		t.Errorf("a working uninstaller waited for the user: %+v", log.progress)
	}
	if _, ok := log.waited(WaitWorking); !ok {
		t.Errorf("progress = %+v", log.progress)
	}
}

// A quiet uninstaller without a window that does nothing may be stuck;
// ending it ends what runs of the removal. msiexec is not judged so.
func TestRunStalledAndEnded(t *testing.T) {
	exe := nsisFile(t, "setup.exe")
	tg := regTarget(detect.Entry{DisplayName: "App", UninstallString: `"` + exe + `"`})
	f := &fakeSys{present: true, root: fakeProc{life: -1, spawn: []fakeProc{{name: "Au_.exe", life: -1}}}}
	r := f.runner()
	log := &recorder{}
	cmds, done := run(r, tg, false, log)
	p := log.until(t, WaitStalled)
	if p.Stage != "卸载程序长时间没有动静，可能卡住了" || p.Window {
		t.Errorf("progress = %+v", p)
	}
	cmds <- EndUninstaller
	if err := wait(t, done); !errors.Is(err, errEnded) {
		t.Fatalf("err = %v\n%s", err, log.String())
	}
	if len(f.ended) != 2 || f.running("Au_.exe") {
		t.Errorf("ended %v", f.ended)
	}
	if !strings.Contains(log.String(), "可能卡住了") || !strings.Contains(log.String(), "已结束 Au_.exe") {
		t.Errorf("log:\n%s", log.String())
	}

	// msiexec's work is in a system service: idle is no sign.
	code := "{12345678-1234-1234-1234-1234567890AB}"
	msi := regTarget(detect.Entry{Name: code, Scope: "User", Arch: "X64", WindowsInstaller: true, DisplayName: "MSI"})
	f = &fakeSys{present: true, root: fakeProc{life: 40, removesKey: true}}
	log = &recorder{}
	if _, err := f.runner().Run(context.Background(), msi, false, log); err != nil {
		t.Fatal(err)
	}
	if _, ok := log.waited(WaitStalled); ok {
		t.Error("msiexec judged stuck")
	}
}

// An uninstaller running as administrator cannot be ended: the user is
// told, and can stop waiting.
func TestRunEndDenied(t *testing.T) {
	exe := nsisFile(t, "setup.exe")
	tg := regTarget(detect.Entry{DisplayName: "App", UninstallString: `"` + exe + `"`})
	f := &fakeSys{present: true, root: fakeProc{life: -1}, denied: map[uint32]bool{1001: true}}
	r := f.runner()
	log := &recorder{}
	cmds, done := run(r, tg, false, log)
	log.until(t, WaitStalled)
	cmds <- SwitchToWindow
	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(log.String(), "没有权限") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cmds <- StopWaiting
	if err := wait(t, done); !errors.Is(err, errStopped) {
		t.Fatalf("err = %v", err)
	}
	log.mu.Lock()
	last := log.progress[len(log.progress)-1]
	log.mu.Unlock()
	if !strings.Contains(last.Stage, "以管理员身份运行") {
		t.Errorf("stage = %q\n%s", last.Stage, log.String())
	}
}

// Waiting stops on its own after nothing happened for long, and as ctx
// ends.
func TestRunGivesUpAndAborts(t *testing.T) {
	exe := plainFile(t)
	tg := regTarget(detect.Entry{DisplayName: "App", UninstallString: `"` + exe + `" /S`, QuietUninstallString: `"` + exe + `" /S`})
	f := &fakeSys{present: true, root: fakeProc{life: -1}}
	r := f.runner()
	r.t.idleMax = 30 * time.Millisecond
	if _, err := r.Run(context.Background(), tg, false, io.Discard); !errors.Is(err, errIdle) {
		t.Errorf("err = %v", err)
	}
	f = &fakeSys{present: true, root: fakeProc{life: -1, busy: true}}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := f.runner().Run(ctx, tg, false, io.Discard); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v", err)
	}
}

func TestRunExitCodes(t *testing.T) {
	plain := plainFile(t)
	nsis := nsisFile(t, "setup.exe")
	cases := []struct {
		file string
		code uint32
		want string // "", "reboot", "cancel", or part of the error
	}{
		{plain, 0, ""}, {plain, 1605, ""}, {plain, 3010, "reboot"}, {plain, 1641, "reboot"},
		{plain, 1602, "cancel"}, {plain, 1223, "cancel"}, {plain, 1618, "另一个安装"},
		{plain, 0xC000013A, "被中止"}, {plain, 2, "退出码 2"}, {plain, 1, "退出码 1"},
		{nsis, 1, "cancel"}, {nsis, 2, "cancel"}, {nsis, 1627, ""}, {nsis, 3, "退出码 3"},
	}
	for _, c := range cases {
		tg := regTarget(detect.Entry{DisplayName: "App", UninstallString: `"` + c.file + `" /x`, QuietUninstallString: `"` + c.file + `" /q`})
		f := &fakeSys{code: c.code}
		res, err := f.runner().Run(context.Background(), tg, false, io.Discard)
		switch {
		case c.want == "" && (err != nil || res.RebootRequired),
			c.want == "reboot" && (err != nil || !res.RebootRequired),
			c.want == "cancel" && !errors.Is(err, ErrCanceled),
			c.want != "" && c.want != "reboot" && c.want != "cancel" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%s %d: %+v %v", filepath.Base(c.file), c.code, res, err)
		}
	}
	tg := regTarget(detect.Entry{DisplayName: "App", UninstallString: `"` + plain + `"`, QuietUninstallString: `"` + plain + `" /q`})
	f := &fakeSys{startErr: runner.ErrElevationCanceled}
	if _, err := f.runner().Run(context.Background(), tg, false, io.Discard); !errors.Is(err, ErrCanceled) || !strings.Contains(err.Error(), "管理员权限") {
		t.Errorf("declined elevation: %v", err)
	}
	f = &fakeSys{startErr: errors.New("无法启动")}
	if _, err := f.runner().Run(context.Background(), tg, false, io.Discard); err == nil || errors.Is(err, ErrCanceled) {
		t.Errorf("start failed: %v", err)
	}
	// Blocked plans never run.
	f = &fakeSys{}
	blocked := regTarget(detect.Entry{DisplayName: "A", NoRemove: true, UninstallString: plain})
	if _, err := f.runner().Run(context.Background(), blocked, false, io.Discard); err == nil || len(f.started) != 0 {
		t.Errorf("a blocked plan ran: %v %q", err, f.started)
	}
}

// A Windows Installer removal waits while another installs, can stop
// waiting before it starts, and runs again once after 1618.
func TestRunWaitsForWindowsInstaller(t *testing.T) {
	code := "{12345678-1234-1234-1234-1234567890AB}"
	tg := regTarget(detect.Entry{Name: code, Scope: "User", Arch: "X64", WindowsInstaller: true, DisplayName: "MSI"})

	f := &fakeSys{present: true, msiBusy: 5, root: fakeProc{life: 1, removesKey: true}}
	log := &recorder{}
	if _, err := f.runner().Run(context.Background(), tg, false, log); err != nil {
		t.Fatal(err)
	}
	if _, ok := log.waited(WaitInstaller); !ok || len(f.started) != 1 || f.msiBusy != 0 {
		t.Errorf("progress %+v, started %q", log.progress, f.started)
	}

	f = &fakeSys{present: true, msiBusy: 1 << 30}
	log = &recorder{}
	cmds, done := run(f.runner(), tg, false, log)
	log.until(t, WaitInstaller)
	cmds <- StopWaiting
	if err := wait(t, done); err == nil || !strings.Contains(err.Error(), "没有开始卸载") || len(f.started) != 0 {
		t.Errorf("err %v, started %q", err, f.started)
	}

	f = &fakeSys{present: true, codes: []uint32{1618, 0}, root: fakeProc{life: 1}}
	if _, err := f.runner().Run(context.Background(), tg, false, io.Discard); err != nil || len(f.started) != 2 {
		t.Errorf("err %v, started %q", err, f.started)
	}
	f = &fakeSys{present: true, codes: []uint32{1618, 1618}, root: fakeProc{life: 1}}
	if _, err := f.runner().Run(context.Background(), tg, false, io.Discard); err == nil || len(f.started) != 2 {
		t.Errorf("err %v, started %q", err, f.started)
	}
}

func TestRunWithSources(t *testing.T) {
	w := &fakeSource{st: source.Status{State: source.Installed}}
	s := &fakeScoop{fakeSource{st: source.Status{State: source.NotInstalled}}}
	f := &fakeSys{}
	r := f.runner()
	r.winget, r.scoop = w, s
	if _, err := r.Run(context.Background(), Target{Name: "Git", Method: ByWinget, WingetID: "Git.Git"}, true, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), Target{Name: "rg", Method: ByScoop, ScoopName: "ripgrep", Global: true}, false, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Join(w.calls, ",") != "Git.Git||i" || strings.Join(s.calls, ",") != "ripgrep|g" {
		t.Fatalf("calls = %q %q", w.calls, s.calls)
	}
	if ok, _ := r.Present(context.Background(), Target{Method: ByWinget, WingetID: "Git.Git"}); !ok {
		t.Error("winget says installed")
	}
	if ok, _ := r.Present(context.Background(), Target{Method: ByScoop, ScoopName: "ripgrep"}); ok {
		t.Error("scoop says not installed")
	}
	if _, err := New(Options{}, nil, nil).Run(context.Background(), Target{Name: "Git", Method: ByWinget, WingetID: "Git.Git"}, false, nil); err == nil {
		t.Error("winget missing must fail")
	}
}

// fakeSource plays winget and scoop.
type fakeSource struct {
	calls []string
	st    source.Status
}

func (f *fakeSource) Uninstall(_ context.Context, a, b string, interactive bool, _ io.Writer) (source.Result, error) {
	f.calls = append(f.calls, a+"|"+b+"|"+map[bool]string{true: "i", false: "q"}[interactive])
	return source.Result{}, nil
}
func (f *fakeSource) Recheck(context.Context, catalog.App) (source.Status, error) { return f.st, nil }

type fakeScoop struct{ fakeSource }

func (f *fakeScoop) Uninstall(_ context.Context, name string, global bool, _ io.Writer) (source.Result, error) {
	f.calls = append(f.calls, name+"|"+map[bool]string{true: "g", false: "u"}[global])
	return source.Result{}, nil
}

func TestRemains(t *testing.T) {
	dir := t.TempDir()
	tg := regTarget(detect.Entry{DisplayName: "App"})
	tg.Location = dir
	f := &fakeSys{present: true}
	r := f.runner()
	if got := r.Remains(tg); got != "程序文件已经删除，但注册表里还留着它的记录" {
		t.Errorf("files gone: %q", got)
	}
	os.WriteFile(filepath.Join(dir, "app.exe"), nil, 0o644)
	if got := r.Remains(tg); got != "" {
		t.Errorf("files there: %q", got)
	}
	f.pending = []string{filepath.Join(dir, "app.exe"), ""}
	if got := r.Remains(tg); got != "部分文件要重启电脑后才会删除" {
		t.Errorf("pending: %q", got)
	}
	f.present = false
	if got := r.Remains(tg); got != "" {
		t.Errorf("gone: %q", got)
	}
}

func TestTracker(t *testing.T) {
	dir := `C:\Program Files\App`
	paths := map[uint32]string{
		10: `C:\Temp\other\Au_.exe`, 11: dir + `\app.exe`,
		21: `C:\Users\u\AppData\Local\Temp\is-AB.tmp\_iu14D2N.tmp`,
		22: `C:\Users\u\AppData\Local\Temp\{A}\.cr\bundle.exe`,
		23: dir + `\helper.exe`, 24: `C:\Windows\notepad.exe`,
	}
	k := newTracker([]procs.Proc{{PID: 10, Name: "Au_.exe"}, {PID: 11, Name: "app.exe"}, {PID: 12, Name: "svchost.exe"}}, dir, func(pid uint32) (string, bool) {
		p, ok := paths[pid]
		return p, ok
	})
	k.add(20)
	snap := []procs.Proc{
		{PID: 10, Name: "Au_.exe"}, {PID: 11, Name: "app.exe"}, // ran before
		{PID: 20, Parent: 1, Name: "uninst.exe"},
		{PID: 30, Parent: 20, Name: "child.exe"}, {PID: 31, Parent: 30, Name: "grandchild.exe"},
		{PID: 32, Parent: 20, Name: "msedge.exe"}, {PID: 33, Parent: 32, Name: "msedge.exe"},
		{PID: 21, Parent: 999, Name: "_iu14D2N.tmp"}, {PID: 22, Parent: 999, Name: "bundle.exe"},
		{PID: 23, Parent: 999, Name: "helper.exe"}, {PID: 24, Parent: 999, Name: "notepad.exe"},
		{PID: 25, Parent: 999, Name: "Un_A.exe"},
	}
	var got []uint32
	for _, p := range k.alive(snap) {
		got = append(got, p.PID)
	}
	if want := []uint32{20, 30, 31, 21, 22, 23, 25}; !slices.Equal(got, want) {
		t.Fatalf("alive = %v, want %v", got, want)
	}
	// The uninstaller exited; its children still are, and a process
	// taking a pid of the removal with another name is not.
	snap = []procs.Proc{{PID: 30, Parent: 20, Name: "child.exe"}, {PID: 40, Parent: 30, Name: "late.exe"}, {PID: 31, Parent: 5, Name: "other.exe"}}
	got = got[:0]
	for _, p := range k.alive(snap) {
		got = append(got, p.PID)
	}
	if want := []uint32{30, 40}; !slices.Equal(got, want) {
		t.Fatalf("alive after = %v, want %v", got, want)
	}
}

func TestPlanNSISCopy(t *testing.T) {
	uninst := nsisFile(t, "uninst.exe")
	inFolder := filepath.Join(t.TempDir(), "uninstall", "uninst.exe")
	os.MkdirAll(filepath.Dir(inFolder), 0o755)
	os.WriteFile(inFolder, []byte("Nullsoft"), 0o644)
	other := nsisFile(t, "setup.exe")
	cases := []struct {
		file, args  string
		interactive bool
		copy        bool
		params      string
	}{
		{uninst, "", false, true, "/S _?=" + filepath.Dir(uninst)},
		{uninst, "", true, true, "_?=" + filepath.Dir(uninst)},
		{uninst, "/currentuser", false, false, "/currentuser /S"},
		{inFolder, "", false, false, "/S"},
		{other, "", false, false, "/S"},
	}
	for _, c := range cases {
		cmd := `"` + c.file + `"`
		if c.args != "" {
			cmd += " " + c.args
		}
		p := PlanFor(regTarget(detect.Entry{DisplayName: "App", UninstallString: cmd}), c.interactive, Options{})
		if p.Kind != NSIS || p.Copy != c.copy || p.Params != c.params {
			t.Errorf("%s %q interactive %v: %+v", c.file, c.args, c.interactive, p)
		}
	}
	if got := withoutInPlace(`/S _?=C:\Program Files\App`); got != "/S" {
		t.Errorf("withoutInPlace = %q", got)
	}
}
