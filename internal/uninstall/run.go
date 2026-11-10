package uninstall

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
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

// Command is what the user asks of a removal while it waits for its
// uninstaller.
type Command int

const (
	// StopWaiting stops waiting; the uninstaller keeps running.
	StopWaiting Command = iota + 1
	// ShowWindow brings the uninstaller's window to the front.
	ShowWindow
	// EndUninstaller ends the uninstaller's processes.
	EndUninstaller
	// SwitchToWindow ends a quiet uninstaller for the removal to run again
	// with its window, which the queue does.
	SwitchToWindow
)

type commandsKey struct{}

// WithCommands returns ctx carrying the user's commands to the removal Run
// with it.
func WithCommands(ctx context.Context, c <-chan Command) context.Context {
	if c == nil {
		return ctx
	}
	return context.WithValue(ctx, commandsKey{}, c)
}

func commandsOf(ctx context.Context) <-chan Command {
	c, _ := ctx.Value(commandsKey{}).(<-chan Command)
	return c
}

// How a removal waits, as source.Progress.Wait.
const (
	// WaitWorking: the uninstaller's processes still run.
	WaitWorking = "working"
	// WaitUser: its window waits for the user.
	WaitUser = "user"
	// WaitStalled: a quiet uninstaller shows no window and has done
	// nothing for a while.
	WaitStalled = "stalled"
	// WaitInstaller: Windows Installer is busy with another program.
	WaitInstaller = "installer"
)

// ErrCanceled reports a removal the user canceled: in the uninstaller, or
// by declining administrator rights.
var ErrCanceled = errors.New("卸载被取消")

var (
	errStopped = errors.New("已停止等待，卸载程序可能还在运行")
	errEnded   = errors.New("卸载程序已被结束")
	errIdle    = errors.New("卸载程序长时间没有任何动静，已停止等待")
)

// process is an uninstaller started: runner.Process.
type process interface {
	PID() uint32
	Exited() (code uint32, done bool, err error)
	Close()
}

// system is what removing uses of Windows, replaced in tests.
type system struct {
	start    func(file, params string, elevate bool) (process, error)
	snapshot func() ([]procs.Proc, error)
	path     func(pid uint32) (string, bool)
	usage    func(pid uint32) (procs.Usage, bool)
	windows  func(pids map[uint32]bool) []procs.Window
	front    func(handle uintptr) bool
	end      func(pid uint32) error
	msiBusy  func() bool
}

// timings are how a removal waits, shortened in tests.
type timings struct {
	poll  time.Duration // how often the processes are looked at
	grace time.Duration // how long after they end for the program to go
	// userIdle is how long a quiet uninstaller's window must be idle to
	// wait for the user; stallIdle how long one without a window must be
	// to be stuck, after Bulk Crap Uninstaller's 30 samples of over a
	// second (TestUninstallerForStalls).
	userIdle, stallIdle time.Duration
	// idleMax is how long nothing of the removal may show activity, unless
	// it waits for the user, before waiting stops.
	idleMax time.Duration
	// wingetEvery is how often winget is asked whether the program is gone.
	wingetEvery time.Duration
}

var defaultTimings = timings{
	poll: time.Second, grace: 5 * time.Second,
	userIdle: 10 * time.Second, stallIdle: 35 * time.Second,
	idleMax: 30 * time.Minute, wingetEvery: 10 * time.Second,
}

// A process is busy when it uses more than 1% of a processor or moves more
// than 10 KB a second: Bulk Crap Uninstaller's measures.
const (
	busyCPU = 0.01
	busyIO  = 10 * 1024
)

// Runner removes programs and tells whether they are still there.
type Runner struct {
	opt    Options
	winget WingetAPI // nil without winget
	scoop  ScoopAPI  // nil without scoop

	// What it uses of Windows, replaced in tests.
	sys        system
	lookup     func(scope, arch, name string) (detect.Entry, bool, error)
	processes  func() ([]string, error)
	powershell func(ctx context.Context, script string, in, out any, log io.Writer) error
	pending    func() []string // the files Windows removes or replaces at restart
	t          timings
}

// New returns a runner removing with winget and scoop, either nil when the
// machine lacks it.
func New(opt Options, w WingetAPI, s ScoopAPI) *Runner {
	return &Runner{
		opt: opt, winget: w, scoop: s,
		sys: system{
			start: func(file, params string, elevate bool) (process, error) {
				p, err := runner.ShellStart(file, params, "", elevate)
				if err != nil {
					return nil, err
				}
				return p, nil
			},
			snapshot: procs.Snapshot,
			path:     procs.Path,
			usage:    procs.UsageOf,
			windows:  procs.Windows,
			front:    procs.Front,
			end:      procs.End,
			msiBusy:  runner.InstallerBusy,
		},
		lookup:    detect.Lookup,
		processes: procs.Paths,
		powershell: func(ctx context.Context, script string, in, out any, log io.Writer) error {
			_, err := runner.PowerShell(ctx, script, in, out, runner.Options{Log: log, Timeout: 10 * time.Minute})
			return err
		},
		pending: pendingRenames,
		t:       defaultTimings,
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

// Remains tells, of a program still there after its uninstaller ran, why
// it may be: "" when nothing tells.
func (r *Runner) Remains(t Target) string {
	if t.Reg == nil {
		return ""
	}
	e, ok, err := r.lookup(t.Reg.Scope, t.Reg.Arch, t.Reg.Name)
	if err != nil || !ok {
		return ""
	}
	if t.Location != "" && procs.Under(r.pending(), t.Location) {
		return "部分文件要重启电脑后才会删除"
	}
	file, _ := SplitCommand(orDefault(e.UninstallString, e.QuietUninstallString))
	if file == "" || !filepath.IsAbs(file) {
		return ""
	}
	if _, err := os.Stat(file); err == nil {
		return ""
	}
	if t.Location != "" {
		if list, err := os.ReadDir(t.Location); err == nil && len(list) > 0 {
			return ""
		}
	}
	return "程序文件已经删除，但注册表里还留着它的记录"
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
// what it started have ended, or the user stops waiting through the
// commands of ctx (WithCommands). An error says what went wrong, wrapping
// ErrCanceled when the user canceled; whether t is gone is for Present to
// tell.
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
		// winget runs the uninstaller through its own service: what it
		// starts is found by its name and place, not as AppShelf's child.
		w := r.watch(ctx, t, p.Quiet, true, log)
		res, err := r.winget.Uninstall(ctx, t.WingetID, t.WingetSource, interactive, log)
		// winget returns once the uninstaller exits, before the copy of
		// itself an NSIS or Inno Setup uninstaller leaves has done its work.
		source.ReportProgress(log, source.Progress{Stage: "正在等待卸载程序结束"})
		_, _, how := w.wait(nil)
		if err == nil {
			err = how.err(ctx)
		}
		return res, err
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
		fmt.Fprintf(log, "（%s，静默卸载）\n", KindText(p.Kind))
	} else {
		fmt.Fprintf(log, "（%s，打开卸载程序窗口）\n", KindText(p.Kind))
	}
	if p.Copy {
		fmt.Fprintln(log, "（从临时目录里的副本运行，以便等它卸载完、取得它的退出码）")
	}
	fmt.Fprintln(log)
	msi := p.Kind == MSI || p.Kind == Burn
	for again := false; ; again = true {
		if msi {
			if err := r.waitInstaller(ctx, log); err != nil {
				return source.Result{}, err
			}
		}
		source.ReportProgress(log, source.Progress{Stage: "正在卸载"})
		code, done, how, err := r.runOnce(ctx, t, p, log)
		if err != nil {
			return source.Result{}, err
		}
		if err := how.err(ctx); err != nil {
			return source.Result{}, err
		}
		if !done {
			return source.Result{}, nil
		}
		if code == 1618 && msi && !again {
			fmt.Fprintln(log, "[AppShelf] 另一个安装或卸载正在进行（1618），等它结束后重新卸载")
			continue
		}
		return verdict(p.Kind, code)
	}
}

// runOnce starts the uninstaller and waits for it: done means it exited,
// with code.
func (r *Runner) runOnce(ctx context.Context, t Target, p Plan, log io.Writer) (code uint32, done bool, how waitEnd, err error) {
	file, params := p.File, p.Params
	if p.Copy {
		dir, cp, err := copyUninstaller(file)
		if err != nil {
			fmt.Fprintf(log, "[AppShelf] 无法把卸载程序复制到临时目录（%v），直接运行它\n", err)
			params = withoutInPlace(params)
		} else {
			file = cp
			// The copy is gone with its directory once it ended.
			defer func() { os.RemoveAll(dir) }()
		}
	}
	w := r.watch(ctx, t, p.Quiet, p.Kind != MSI && p.Kind != Burn, log)
	proc, err := r.sys.start(file, params, p.Elevate)
	if err != nil {
		if errors.Is(err, runner.ErrElevationCanceled) {
			return 0, false, 0, fmt.Errorf("%w：你拒绝了管理员权限，没有卸载", ErrCanceled)
		}
		return 0, false, 0, err
	}
	defer proc.Close()
	code, done, how = w.wait(proc)
	return code, done, how, nil
}

// copyUninstaller copies an uninstaller to a new directory of the
// temporary directory, returning both.
func copyUninstaller(file string) (dir, copied string, err error) {
	src, err := os.Open(file)
	if err != nil {
		return "", "", err
	}
	defer src.Close()
	if dir, err = os.MkdirTemp("", "AppShelf-uninstall-"); err != nil {
		return "", "", err
	}
	copied = filepath.Join(dir, filepath.Base(file))
	dst, err := os.Create(copied)
	if err == nil {
		_, err = io.Copy(dst, src)
		if cerr := dst.Close(); err == nil {
			err = cerr
		}
	}
	if err != nil {
		os.RemoveAll(dir)
		return "", "", err
	}
	return dir, copied, nil
}

// verdict reads an uninstaller's exit code, as its installer means it.
func verdict(k Kind, code uint32) (source.Result, error) {
	switch {
	case code == 0, code == 1605: // 1605: Windows Installer knows no such product
		return source.Result{}, nil
	case code == 3010, code == 1641:
		return source.Result{RebootRequired: true}, nil
	case code == 1602, code == 1223, k == NSIS && (code == 1 || code == 2):
		// NSIS: 1 is its Cancel button, 2 the script aborting, often
		// after the user canceled.
		return source.Result{}, ErrCanceled
	case k == NSIS && code == 1627:
		return source.Result{}, nil // as Bulk Crap Uninstaller takes it
	case code == 1618:
		return source.Result{}, errors.New("另一个安装或卸载正在进行，请稍后重试")
	case code == 1603 && (k == MSI || k == Burn):
		return source.Result{}, errors.New("Windows Installer 卸载时出错（退出码 1603），详情见日志")
	case code == 0xC000013A:
		return source.Result{}, errors.New("卸载程序被中止")
	}
	return source.Result{}, fmt.Errorf("卸载程序以退出码 %d 结束", code)
}

// waitInstaller waits while Windows Installer works on another program:
// an msiexec started meanwhile gives up with 1618.
func (r *Runner) waitInstaller(ctx context.Context, log io.Writer) error {
	if !r.sys.msiBusy() {
		return nil
	}
	fmt.Fprintln(log, "[AppShelf] Windows Installer 正在进行另一个安装或卸载，等它结束")
	source.ReportProgress(log, source.Progress{Stage: "等待另一个安装或卸载结束", Wait: WaitInstaller})
	cmds := commandsOf(ctx)
	deadline := time.Now().Add(r.t.idleMax)
	for r.sys.msiBusy() {
		if time.Now().After(deadline) {
			return errors.New("另一个安装或卸载长时间没有结束，没有开始卸载")
		}
		select {
		case c := <-cmds:
			if c != ShowWindow {
				fmt.Fprintln(log, "[AppShelf] 你停止了等待")
				return errors.New("已停止等待，没有开始卸载")
			}
		case <-time.After(r.t.poll):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// waitEnd is how waiting for an uninstaller ended.
type waitEnd int

const (
	waitDone    waitEnd = iota // its processes ended, or the program went
	waitStopped                // the user stopped waiting
	waitEnded                  // the user ended the uninstaller
	waitIdle                   // nothing happened for too long
	waitAborted                // ctx ended
)

func (h waitEnd) err(ctx context.Context) error {
	switch h {
	case waitStopped:
		return errStopped
	case waitEnded:
		return errEnded
	case waitIdle:
		return errIdle
	case waitAborted:
		return ctx.Err()
	}
	return nil
}

// watcher follows the processes of one removal.
type watcher struct {
	r     *Runner
	ctx   context.Context
	log   io.Writer
	cmds  <-chan Command
	track *tracker
	gone  func(fresh bool) bool // nil when nothing tells
	// quiet means no window should show; stallable that the processes'
	// activity tells whether they work, which it does not of msiexec,
	// working in a system service.
	quiet, stallable bool
}

// watch notes the processes running before the removal starts.
func (r *Runner) watch(ctx context.Context, t Target, quiet, stallable bool, log io.Writer) *watcher {
	snap, _ := r.sys.snapshot()
	dir := ""
	if t.Location != "" && detect.OwnDirectory(t.Location) {
		dir = t.Location
	}
	return &watcher{
		r: r, ctx: ctx, log: log, cmds: commandsOf(ctx),
		track: newTracker(snap, dir, r.sys.path),
		gone:  r.goneCheck(ctx, t),
		quiet: quiet, stallable: stallable,
	}
}

// wait waits for root, nil when the uninstaller is not AppShelf's, and
// what the removal starts, saying what it waits for. It returns root's
// exit code when done.
func (w *watcher) wait(root process) (code uint32, done bool, how waitEnd) {
	r := w.r
	if root != nil {
		w.track.add(root.PID())
	}
	var (
		state, stage, prompt string
		hasWindow            bool
		lastActive           = time.Now()
		idleSince, winSince  time.Time
		prev                 map[uint32]procs.Usage
		prevAt               time.Time
		ending               bool   // the user ended the uninstaller
		endNote              string // why it could not be ended
	)
	finished := func() waitEnd {
		if ending {
			return waitEnded
		}
		return waitDone
	}
	for {
		if root != nil && !done {
			c, exited, err := root.Exited()
			if err != nil {
				exited = true
			}
			if exited {
				code, done = c, err == nil
				if done {
					fmt.Fprintf(w.log, "卸载程序退出码：%d\n", code)
				}
				root = nil
			}
		}
		var alive []procs.Proc
		if snap, err := r.sys.snapshot(); err == nil {
			alive = w.track.alive(snap)
		}
		if root == nil && w.gone != nil && w.gone(false) {
			return code, done, finished()
		}
		if root == nil && len(alive) == 0 {
			break
		}

		now := time.Now()
		active, measured := false, false
		cur := map[uint32]procs.Usage{}
		pids := map[uint32]bool{}
		for _, p := range alive {
			pids[p.PID] = true
			u, ok := r.sys.usage(p.PID)
			if !ok {
				continue
			}
			cur[p.PID], measured = u, true
			before, seen := prev[p.PID]
			if !seen || busy(before, u, now.Sub(prevAt)) {
				active = true // a new process is news too
			}
		}
		prev, prevAt = cur, now
		switch {
		case active:
			lastActive, idleSince = now, time.Time{}
		case measured && idleSince.IsZero():
			idleSince = now
		case !measured:
			idleSince = time.Time{}
		}
		idle := func(d time.Duration) bool { return !idleSince.IsZero() && now.Sub(idleSince) >= d }

		wins := r.sys.windows(pids)
		var win *procs.Window
		for i := range wins {
			if win == nil || len(wins[i].Text) > len(win.Text) {
				win = &wins[i]
			}
		}
		if win == nil {
			winSince = time.Time{}
		} else if winSince.IsZero() {
			winSince = now
		}

		next := WaitWorking
		switch {
		case win != nil && (!w.quiet || idle(r.t.userIdle) || !measured && now.Sub(winSince) >= r.t.userIdle):
			next = WaitUser
		case win == nil && w.quiet && w.stallable && idle(r.t.stallIdle):
			next = WaitStalled
		}
		if next == WaitUser {
			lastActive = now // the user may take their time
		} else if now.Sub(lastActive) >= r.t.idleMax {
			fmt.Fprintf(w.log, "[AppShelf] 卸载程序 %s 没有任何动静，停止等待（它可能还在运行）\n", span(r.t.idleMax))
			return code, done, waitIdle
		}

		nextPrompt := ""
		if win != nil {
			nextPrompt = promptOf(*win)
		}
		nextStage := endNote
		if nextStage == "" {
			nextStage = stageOf(next, w.quiet, alive)
		}
		if next != state || nextPrompt != prompt {
			w.note(next, nextPrompt, alive, now.Sub(idleSince))
		}
		if next != state || nextPrompt != prompt || nextStage != stage || (win != nil) != hasWindow {
			state, prompt, stage, hasWindow = next, nextPrompt, nextStage, win != nil
			source.ReportProgress(w.log, source.Progress{Stage: stage, Wait: state, Prompt: prompt, Window: hasWindow})
		}

		t := time.NewTimer(r.t.poll)
		select {
		case c := <-w.cmds:
			t.Stop()
			switch c {
			case StopWaiting:
				fmt.Fprintln(w.log, "[AppShelf] 你停止了等待，卸载程序如果还在运行，不会被结束")
				return code, done, waitStopped
			case ShowWindow:
				if win != nil && !r.sys.front(win.Handle) {
					fmt.Fprintln(w.log, "[AppShelf] 无法把卸载程序的窗口提到前面，请在任务栏里找到它")
				}
			case EndUninstaller, SwitchToWindow:
				ending = true
				endNote = w.end(alive)
			}
		case <-t.C:
		case <-w.ctx.Done():
			t.Stop()
			return code, done, waitAborted
		}
	}
	// Uninstallers remove their key last.
	if w.gone != nil {
		end := time.Now().Add(r.t.grace)
		for time.Now().Before(end) && !w.gone(true) {
			if !sleep(w.ctx, min(r.t.poll, r.t.grace)) {
				return code, done, waitAborted
			}
		}
	}
	return code, done, finished()
}

// busy reports a process that worked between two readings dt apart.
func busy(before, now procs.Usage, dt time.Duration) bool {
	if dt <= 0 {
		return true
	}
	cpu := float64(now.CPU-before.CPU) / float64(dt)
	io := float64(now.IO-before.IO) / dt.Seconds()
	return cpu > busyCPU || io > busyIO
}

// note writes to the log what the removal now waits for.
func (w *watcher) note(state, prompt string, alive []procs.Proc, idle time.Duration) {
	switch state {
	case WaitWorking:
		if len(alive) > 0 {
			fmt.Fprintf(w.log, "[AppShelf] 等待卸载程序结束：%s\n", names(alive))
		}
	case WaitUser:
		if w.quiet {
			fmt.Fprintf(w.log, "[AppShelf] 卸载程序弹出了窗口，可能在等你操作：%s\n", prompt)
		} else {
			fmt.Fprintf(w.log, "[AppShelf] 等待你在卸载窗口里完成操作：%s\n", prompt)
		}
	case WaitStalled:
		fmt.Fprintf(w.log, "[AppShelf] %s 已经 %s 几乎没有使用处理器和磁盘，可能卡住了\n", names(alive), span(idle))
	}
}

func stageOf(state string, quiet bool, alive []procs.Proc) string {
	switch state {
	case WaitUser:
		if quiet {
			return "卸载程序弹出了窗口，可能在等你操作"
		}
		return "等待你在卸载窗口里完成操作"
	case WaitStalled:
		return "卸载程序长时间没有动静，可能卡住了"
	}
	if len(alive) == 0 {
		return "正在等待卸载程序结束"
	}
	return "正在等待卸载程序结束（" + names(alive) + "）"
}

// promptOf is what a window says: its title, then its labels.
func promptOf(w procs.Window) string {
	s := strings.Join(w.Text, " ")
	switch {
	case w.Title == "":
	case s == "":
		s = "「" + w.Title + "」"
	default:
		s = "「" + w.Title + "」" + s
	}
	if r := []rune(s); len(r) > 160 {
		s = string(r[:160]) + "…"
	}
	return s
}

// end ends the removal's processes, returning why some could not be.
func (w *watcher) end(alive []procs.Proc) string {
	denied := false
	for _, p := range alive {
		if err := w.r.sys.end(p.PID); err != nil {
			denied = denied || errors.Is(err, procs.ErrDenied)
			fmt.Fprintf(w.log, "[AppShelf] 无法结束 %s：%v\n", p.Name, err)
			continue
		}
		fmt.Fprintf(w.log, "[AppShelf] 已结束 %s\n", p.Name)
	}
	if denied {
		return "无法结束卸载程序（它以管理员身份运行），请在它的窗口里关闭"
	}
	return ""
}

// span says a duration for the user, as "35 秒" or "30 分钟".
func span(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d 秒", int(d.Round(time.Second)/time.Second))
	}
	return fmt.Sprintf("%d 分钟", int(d.Round(time.Minute)/time.Minute))
}

// goneCheck returns what tells t gone while the removal waits: its
// registry key, which uninstallers remove last, else winget, asked at most
// every wingetEvery unless fresh; nil when neither.
func (r *Runner) goneCheck(ctx context.Context, t Target) func(fresh bool) bool {
	switch {
	case t.Reg != nil:
		return func(bool) bool {
			_, ok, err := r.lookup(t.Reg.Scope, t.Reg.Arch, t.Reg.Name)
			return err == nil && !ok
		}
	case t.Method == ByWinget && r.winget != nil:
		var asked time.Time
		var gone bool
		return func(fresh bool) bool {
			if !fresh && !asked.IsZero() && time.Since(asked) < r.t.wingetEvery {
				return gone
			}
			asked = time.Now()
			ok, err := r.Present(ctx, t)
			gone = err == nil && !ok
			return gone
		}
	}
	return nil
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
