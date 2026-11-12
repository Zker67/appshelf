// Package queue installs, updates and removes apps one at a time. Each item
// checks the app again right before it runs, and again after: an app found
// installed is skipped, an app whose state cannot be read is not touched,
// an install counts only when the app is then found installed, and a
// removal only when it is then gone.
package queue

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"appshelf/internal/catalog"
	"appshelf/internal/source"
	"appshelf/internal/tasklog"
	"appshelf/internal/uninstall"
)

// Action is what an item does.
type Action int

const (
	Install Action = iota
	Upgrade
	Uninstall
)

func (a Action) String() string {
	switch a {
	case Upgrade:
		return "更新"
	case Uninstall:
		return "卸载"
	}
	return "安装"
}

// ItemState is where an item is.
type ItemState int

const (
	Waiting ItemState = iota
	Running
	Succeeded
	Skipped
	Failed
)

func (s ItemState) String() string {
	switch s {
	case Running:
		return "进行中"
	case Succeeded:
		return "成功"
	case Skipped:
		return "跳过"
	case Failed:
		return "失败"
	}
	return "等待"
}

// Done reports whether the item has finished.
func (s ItemState) Done() bool { return s == Succeeded || s == Skipped || s == Failed }

// Item is one install, update or removal.
type Item struct {
	ID      int
	App     catalog.App
	Action  Action
	State   ItemState
	Message string // why it was skipped or failed, or a note on success
	LogPath string
	Started time.Time
	Ended   time.Time
	// From and To are the versions read before and after it ran, when known.
	From, To string
	// Latest is the version an update was to bring, read before it ran;
	// StillOffered means its source still offered that version after it
	// ran, as a source whose version numbers do not match the program's.
	Latest       string
	StillOffered bool
	// Auto marks an update AppShelf queued itself, after a check.
	Auto bool
	// Stage is what a running item does now, and Progress how far its
	// download has come, as far as the source reports it.
	Stage    string
	Progress source.Progress
	// Target is the program a removal removes; Interactive asks for the
	// window of its uninstaller, or of an update's installer.
	Target      *uninstall.Target
	Interactive bool
	// Control carries the user's commands to a removal while it runs.
	Control <-chan uninstall.Command
	// Canceled marks a failed item the user canceled: a removal canceled
	// in its uninstaller, or an item stopped before it changed anything.
	Canceled bool
	// Reboot marks a succeeded item that needs Windows to restart to be
	// done.
	Reboot bool

	log *tasklog.Log
	// retried is a removal queued again after it failed: its program gone
	// before it runs again was removed by the earlier run.
	retried bool
	ctl     chan uninstall.Command
	// toWindow is a removal the user switched to its uninstaller's window:
	// still there once it ends, it runs again so.
	toWindow bool
	// cancel ends the context of an install or update running; canceling
	// marks one the user canceled.
	cancel    context.CancelFunc
	canceling bool
}

// Stages the queue itself sets; sources report the others.
const (
	StageChecking   = "正在检查安装状态"
	StageStarting   = "正在启动"
	StageRechecking = "正在复查"
)

// Executor does the work of the items. Its methods run on the queue's
// goroutine.
type Executor interface {
	// Status reads the state of the item's app now, with its detection
	// rule; of a removal, whether its program is still there.
	Status(ctx context.Context, item Item) source.Status
	// Run does the item, writing its output to log.
	Run(ctx context.Context, item Item, log io.Writer) (source.Result, error)
	// OpenLog creates the log of an item and returns its path.
	OpenLog(item Item) (io.WriteCloser, string, error)
	// Finished reports an item that ran, with the app's state read after
	// it and what the run returned, and returns the state to judge the
	// item by: what it records, as the release a GitHub app installed, can
	// change it.
	Finished(item Item, st source.Status, res source.Result) source.Status
}

// Queue runs its items in order, one at a time.
type Queue struct {
	exec     Executor
	notify   func()
	ctx      context.Context
	progress throttle

	mu       sync.Mutex
	hold     func(ctx context.Context, it Item) string
	items    []*Item
	nextID   int
	running  bool // the worker goroutine is running
	stopping bool // stop after the current item
}

// New returns a queue that runs items with exec and calls notify, from its
// goroutine, after every change of an item. ctx ends the queue's work.
func New(ctx context.Context, exec Executor, notify func()) *Queue {
	if notify == nil {
		notify = func() {}
	}
	q := &Queue{exec: exec, notify: notify, ctx: ctx}
	q.progress.every = progressEvery
	return q
}

// progressEvery is how often progress and new output are announced.
const progressEvery = 250 * time.Millisecond

// SetProgress sets what is called, at most every 250 milliseconds, when a
// running item reports progress or writes output. Call it before Add.
func (q *Queue) SetProgress(fn func()) { q.progress.fn = fn }

// SetHold sets what tells, right before an update AppShelf queued itself
// runs, why it must not run now, as its program the user opened while it
// waited; "" lets it run. It runs on the queue's goroutine.
func (q *Queue) SetHold(fn func(ctx context.Context, it Item) string) {
	q.mu.Lock()
	q.hold = fn
	q.mu.Unlock()
}

// Add queues the apps with the action and starts the queue. An app that
// already has an item waiting or running is not queued again. It returns
// how many items it added.
func (q *Queue) Add(apps []catalog.App, action Action) int { return q.add(apps, action, false) }

// AddAuto queues updates AppShelf starts itself, as Add does: each is
// held back, right before it runs, by what SetHold set.
func (q *Queue) AddAuto(apps []catalog.App) int { return q.add(apps, Upgrade, true) }

func (q *Queue) add(apps []catalog.App, action Action, auto bool) int {
	q.mu.Lock()
	added := 0
	for _, a := range apps {
		if q.pendingLocked(a.ID) || q.pendingSourceLocked(a.Source) {
			continue
		}
		q.nextID++
		q.enqueueLocked(&Item{ID: q.nextID, App: a.Clone(), Action: action, State: Waiting, Auto: auto})
		added++
	}
	q.startLocked()
	q.mu.Unlock()
	q.notify()
	return added
}

// enqueueLocked adds a waiting item: one the user started goes before the
// updates AppShelf queued itself that wait, which go last.
func (q *Queue) enqueueLocked(it *Item) {
	if !it.Auto {
		if i := slices.IndexFunc(q.items, func(c *Item) bool { return c.State == Waiting && c.Auto }); i >= 0 {
			q.items = slices.Insert(q.items, i, it)
			return
		}
	}
	q.items = append(q.items, it)
}

func (q *Queue) pendingLocked(appID string) bool {
	for _, it := range q.items {
		if it.App.ID == appID && !it.State.Done() {
			return true
		}
	}
	return false
}

// AddUninstall queues the removal of programs and starts the queue. A
// program already waiting or running, or whose package is, is not queued
// again. It returns how many it added.
func (q *Queue) AddUninstall(targets []uninstall.Target) int {
	q.mu.Lock()
	added := 0
	for _, t := range targets {
		app := UninstallApp(t)
		if q.pendingLocked(app.ID) || q.pendingSourceLocked(app.Source) {
			continue
		}
		q.nextID++
		q.items = append(q.items, &Item{ID: q.nextID, App: app, Action: Uninstall, State: Waiting, Target: &t})
		added++
	}
	q.startLocked()
	q.mu.Unlock()
	q.notify()
	return added
}

// UninstallApp is the app a removal stands for in the queue: its ID is the
// program's key, its source the package a source knows it by, so that it
// does not run beside an install or update of the same package.
func UninstallApp(t uninstall.Target) catalog.App {
	app := catalog.App{ID: "uninstall:" + t.Key, Name: t.Name}
	switch t.Method {
	case uninstall.ByWinget:
		app.Source = catalog.Source{Type: catalog.Winget, ID: t.WingetID}
	case uninstall.ByScoop:
		app.Source = catalog.Source{Type: catalog.Scoop, Name: t.ScoopName}
	}
	return app
}

// Retry queues a failed item again, at the end, and starts the queue. The
// user retries it: an update AppShelf started itself is the user's then.
func (q *Queue) Retry(id int) bool { return q.retry(id, false) }

// RetryInteractive queues a failed removal or update again, with the
// window of its uninstaller or installer.
func (q *Queue) RetryInteractive(id int) bool { return q.retry(id, true) }

func (q *Queue) retry(id int, interactive bool) bool {
	q.mu.Lock()
	i := slices.IndexFunc(q.items, func(it *Item) bool { return it.ID == id })
	if i < 0 || q.items[i].State != Failed || q.pendingLocked(q.items[i].App.ID) || q.pendingSourceLocked(q.items[i].App.Source) {
		q.mu.Unlock()
		return false
	}
	if interactive && q.items[i].Action == Install {
		q.mu.Unlock()
		return false
	}
	it := q.items[i]
	if interactive {
		it.Interactive = true
	}
	it.retried = it.Action == Uninstall
	it.Auto = false
	q.items = slices.Delete(q.items, i, i+1)
	it.State, it.Message, it.LogPath, it.Canceled = Waiting, "", "", false
	it.From, it.To, it.Latest, it.StillOffered = "", "", "", false
	it.Stage, it.Progress, it.log = "", source.Progress{}, nil
	it.Started, it.Ended, it.Reboot = time.Time{}, time.Time{}, false
	q.enqueueLocked(it)
	q.startLocked()
	q.mu.Unlock()
	q.notify()
	return true
}

// Control passes a command of the user to the removal id while it runs,
// and reports whether it took it. SwitchToWindow also runs it again with
// its uninstaller's window when its program is still there after.
func (q *Queue) Control(id int, c uninstall.Command) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, it := range q.items {
		if it.ID != id || it.State != Running || it.ctl == nil {
			continue
		}
		select {
		case it.ctl <- c:
			if c == uninstall.SwitchToWindow {
				it.toWindow = true
			}
			return true
		default:
			return false
		}
	}
	return false
}

// Cancel cancels an item the user no longer wants, and reports whether it
// did: one waiting fails as canceled, without running; one running only
// while its progress says it can stop without changing anything, as while
// it downloads or waits for another install, its run then ending.
func (q *Queue) Cancel(id int) bool {
	q.mu.Lock()
	done := false
	for _, it := range q.items {
		if it.ID != id {
			continue
		}
		switch {
		case it.State == Waiting:
			it.State, it.Canceled, it.Message, it.Ended = Failed, true, "已取消，没有开始", time.Now()
			done = true
		case it.State == Running && it.cancel != nil && !it.canceling && it.log != nil && it.log.Progress().Cancelable:
			it.canceling = true
			it.cancel()
			done = true
		}
	}
	q.mu.Unlock()
	if done {
		q.notify()
	}
	return done
}

// CanCancel reports whether Cancel would cancel the item, as Items copies
// it.
func (it Item) CanCancel() bool {
	return it.State == Waiting || it.State == Running && it.cancel != nil && !it.canceling && it.Progress.Cancelable
}

// Stop lets the item running finish and leaves the others waiting.
func (q *Queue) Stop() {
	q.mu.Lock()
	if q.running {
		q.stopping = true
	}
	q.mu.Unlock()
	q.notify()
}

// Resume starts the waiting items again after Stop.
func (q *Queue) Resume() {
	q.mu.Lock()
	q.startLocked()
	q.mu.Unlock()
	q.notify()
}

// ClearDone removes the finished items.
func (q *Queue) ClearDone() {
	q.mu.Lock()
	q.items = slices.DeleteFunc(q.items, func(it *Item) bool { return it.State.Done() })
	q.mu.Unlock()
	q.notify()
}

// Items returns a copy of the items, in order.
func (q *Queue) Items() []Item {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]Item, len(q.items))
	for i, it := range q.items {
		out[i] = *it
		if it.State == Running && it.log != nil {
			p := it.log.Progress()
			out[i].Progress = p
			if p.Stage != "" {
				out[i].Stage = p.Stage
			}
		}
	}
	return out
}

// Log returns the output of an item from line from on, and whether the
// item has output.
func (q *Queue) Log(id int, from int) (tasklog.Chunk, bool) {
	q.mu.Lock()
	var l *tasklog.Log
	for _, it := range q.items {
		if it.ID == id {
			l = it.log
		}
	}
	q.mu.Unlock()
	if l == nil {
		return tasklog.Chunk{}, false
	}
	return l.Lines(from), true
}

// Running reports whether the queue is working through its items, and
// whether it will stop after the current one.
func (q *Queue) Running() (running, stopping bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.running, q.stopping
}

// Pending reports whether the app has an item waiting or running.
func (q *Queue) Pending(appID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.pendingLocked(appID)
}

// PendingSource also covers the same package queued under a catalog ID or
// a temporary installed-update ID.
func (q *Queue) PendingSource(src catalog.Source) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.pendingSourceLocked(src)
}

func (q *Queue) pendingSourceLocked(src catalog.Source) bool {
	if src.Type == "" {
		return false
	}
	for _, it := range q.items {
		if !it.State.Done() && it.App.Source.Key() == src.Key() {
			return true
		}
	}
	return false
}

func (q *Queue) startLocked() {
	q.stopping = false
	if q.running || !slices.ContainsFunc(q.items, func(it *Item) bool { return it.State == Waiting }) {
		return
	}
	q.running = true
	go q.work()
}

func (q *Queue) work() {
	for {
		q.mu.Lock()
		var it *Item
		if !q.stopping && q.ctx.Err() == nil {
			for _, cand := range q.items {
				if cand.State == Waiting {
					it = cand
					break
				}
			}
		}
		if it == nil {
			q.running, q.stopping = false, false
			q.mu.Unlock()
			q.notify()
			return
		}
		it.State, it.Started, it.Stage = Running, time.Now(), StageChecking
		runCtx, cancel := context.WithCancel(q.ctx)
		if it.Action == Uninstall {
			it.ctl = make(chan uninstall.Command, 4)
			it.Control = it.ctl
		} else {
			it.cancel = cancel
		}
		snapshot := *it
		q.mu.Unlock()
		q.notify()

		state, msg, logPath := q.runItem(runCtx, &snapshot, live{q, it})
		cancel()

		q.mu.Lock()
		it.State, it.Message, it.LogPath, it.Ended = state, msg, logPath, time.Now()
		it.From, it.To, it.Canceled = snapshot.From, snapshot.To, snapshot.Canceled
		it.Latest, it.StillOffered, it.Reboot = snapshot.Latest, snapshot.StillOffered, snapshot.Reboot
		it.Stage, it.Progress = "", source.Progress{}
		again := it.toWindow && state == Failed && !it.Interactive
		it.ctl, it.Control, it.toWindow = nil, nil, false
		it.cancel, it.canceling = nil, false
		q.mu.Unlock()
		q.notify()
		if again {
			q.retry(it.ID, true)
		}
	}
}

// live is the item the worker runs, as the window sees it: its stage and
// its log, which the worker sets while it runs a copy.
type live struct {
	q  *Queue
	it *Item
}

func (l live) stage(s string) {
	l.q.mu.Lock()
	l.it.Stage = s
	l.q.mu.Unlock()
	l.q.progress.poke()
}

// canceled reports whether the user canceled the item.
func (l live) canceled() bool {
	l.q.mu.Lock()
	defer l.q.mu.Unlock()
	return l.it.canceling
}

func (l live) attach(log *tasklog.Log) {
	l.q.mu.Lock()
	l.it.log = log
	l.q.mu.Unlock()
}

// runItem does one item: check, run, check again. runCtx is the context of
// its run, which Cancel ends; the checks go on.
func (q *Queue) runItem(runCtx context.Context, it *Item, live live) (ItemState, string, string) {
	ctx := q.ctx
	before := q.exec.Status(ctx, *it)
	it.From = before.Version
	if it.Action == Upgrade {
		it.Latest = before.Latest
	}
	if state, msg, skip := judgeBefore(*it, before); skip {
		if state == Succeeded {
			// A removal the earlier run did: report it as one that ran.
			q.exec.Finished(*it, before, source.Result{})
		}
		return state, msg, ""
	}
	q.mu.Lock()
	hold := q.hold
	q.mu.Unlock()
	if it.Auto && hold != nil {
		if why := hold(ctx, *it); why != "" {
			return Skipped, why, ""
		}
	}

	file, logPath, err := q.exec.OpenLog(*it)
	if err != nil {
		return Failed, "无法创建日志：" + err.Error(), ""
	}
	log := tasklog.New(file, q.progress.poke)
	live.attach(log)
	live.stage(StageStarting)
	res, runErr := q.exec.Run(runCtx, *it, log)
	canceled := runErr != nil && live.canceled() && ctx.Err() == nil
	switch {
	case canceled:
		fmt.Fprintf(log, "\n[AppShelf] 你取消了这个任务（%v）\n", runErr)
	case runErr != nil:
		fmt.Fprintf(log, "\n[AppShelf] %s失败：%v\n", it.Action, runErr)
	}
	log.ReportProgress(source.Progress{})
	live.stage(StageRechecking)
	after := q.exec.Status(ctx, *it)
	it.LogPath = logPath
	after = q.exec.Finished(*it, after, res)
	it.To = after.Version
	if it.Action == Uninstall {
		fmt.Fprintf(log, "\n[AppShelf] 之后的状态：%s\n", presence(after))
	} else {
		fmt.Fprintf(log, "\n[AppShelf] 之后的状态：%s %s %s\n", after.State, after.Version, after.Note)
	}
	log.Close()

	if it.Action == Uninstall {
		state, msg, canceled := judgeRemoval(after, res, runErr)
		it.Canceled = canceled
		it.Reboot = state == Succeeded && res.RebootRequired
		return state, msg, logPath
	}
	if canceled {
		// Stopped while it could stop: what the check after it reads tells
		// whether it got done all the same.
		switch {
		case it.Action == Install && after.IsInstalled():
			return Succeeded, "取消时已经装好了", logPath
		case it.Action == Upgrade && after.IsInstalled() && after.Version != before.Version:
			return Succeeded, "取消时已经更新好了", logPath
		}
		it.Canceled = true
		return Failed, "已取消", logPath
	}
	if runErr != nil {
		return Failed, runErr.Error(), logPath
	}
	if res.AlreadyInstalled {
		return Skipped, "已安装", logPath
	}
	note := ""
	if res.RebootRequired {
		note = "需要重启电脑才能完成"
	}
	if it.Action == Upgrade && after.State == source.UpdateAvailable && it.Latest != "" && strings.EqualFold(strings.TrimSpace(after.Latest), strings.TrimSpace(it.Latest)) {
		it.StillOffered = true
		if after.Version != before.Version {
			note = joinNote(note, "来源仍提示可以更新到 "+it.Latest+"，可能是来源记录的版本号和程序的对不上")
		}
	}
	switch {
	case after.Blocked():
		return Failed, "安装程序已结束，但无法确认结果：" + noteOr(after, "原因未知"), logPath
	case it.Action == Install && !after.IsInstalled():
		return Failed, "安装程序已结束，但没有检测到这个软件", logPath
	case it.Action == Upgrade && after.State == source.UpdateAvailable && after.Version == before.Version:
		return Failed, "更新程序已结束，但版本没有变化", logPath
	case it.Action == Upgrade && !after.IsInstalled():
		return Failed, "更新后没有检测到这个软件", logPath
	}
	it.Reboot = res.RebootRequired
	return Succeeded, note, logPath
}

// judgeBefore tells whether an item must not run, from the state read
// right before it.
func judgeBefore(it Item, before source.Status) (ItemState, string, bool) {
	switch {
	case it.Action == Uninstall && before.Blocked():
		return Failed, "无法确认它是否还在本机，没有执行：" + noteOr(before, "原因未知"), true
	case it.Action == Uninstall && !before.IsInstalled() && it.retried:
		// The earlier run's uninstaller finished after the check.
		return Succeeded, "已卸载（上次卸载其实已经完成）", true
	case it.Action == Uninstall && !before.IsInstalled():
		return Skipped, "已不在本机", true
	case it.Action == Uninstall:
		return 0, "", false
	case before.Blocked():
		return Failed, "无法确认安装状态，没有执行：" + noteOr(before, "原因未知"), true
	case it.Action == Install && before.IsInstalled():
		if before.External {
			return Skipped, "已安装（不是通过这个来源安装的）", true
		}
		return Skipped, "已安装", true
	case it.Action == Upgrade && before.State == source.NotInstalled:
		return Skipped, "未安装，无法更新", true
	case it.Action == Upgrade && before.External:
		return Skipped, "不是通过这个来源安装的，请用原来的方式更新", true
	case it.Action == Upgrade && before.State != source.UpdateAvailable:
		if before.Note != "" {
			// The check may not have been able to look for updates.
			return Skipped, "没有发现更新（" + before.Note + "）", true
		}
		return Skipped, "已是最新版本", true
	}
	return 0, "", false
}

// judgeRemoval judges a removal by whether the program is gone: what the
// uninstaller said only explains. canceled marks one the user canceled.
func judgeRemoval(after source.Status, res source.Result, runErr error) (state ItemState, msg string, canceled bool) {
	switch {
	case after.Blocked():
		return Failed, "卸载程序已结束，但无法确认结果：" + noteOr(after, "原因未知"), false
	case after.IsInstalled() && errors.Is(runErr, uninstall.ErrCanceled):
		return Failed, runErr.Error(), true
	case after.IsInstalled() && runErr != nil:
		return Failed, withNote(runErr.Error(), after), false
	case after.IsInstalled():
		return Failed, withNote("卸载程序已结束，但这个软件还在本机", after), false
	case res.RebootRequired:
		return Succeeded, "需要重启电脑才能完成", false
	case runErr != nil:
		return Succeeded, "已卸载（卸载程序报告：" + runErr.Error() + "）", false
	}
	return Succeeded, "", false
}

func joinNote(a, b string) string {
	if a == "" {
		return b
	}
	return a + "；" + b
}

// withNote adds why a program may still be there.
func withNote(msg string, st source.Status) string {
	if st.Note != "" {
		return msg + "：" + st.Note
	}
	return msg
}

func presence(st source.Status) string {
	switch {
	case st.Blocked():
		return "无法确认（" + noteOr(st, "原因未知") + "）"
	case st.IsInstalled() && st.Note != "":
		return "仍在本机（" + st.Note + "）"
	case st.IsInstalled():
		return "仍在本机"
	}
	return "已不在本机"
}

func noteOr(st source.Status, fallback string) string {
	if st.Note != "" {
		return st.Note
	}
	return fallback
}

// throttle calls fn at most once every every, the last call delayed rather
// than dropped.
type throttle struct {
	fn    func()
	every time.Duration

	mu    sync.Mutex
	last  time.Time
	timer *time.Timer
}

func (t *throttle) poke() {
	if t.fn == nil {
		return
	}
	t.mu.Lock()
	if t.timer != nil {
		t.mu.Unlock()
		return
	}
	wait := t.every - time.Since(t.last)
	if wait <= 0 {
		t.last = time.Now()
		t.mu.Unlock()
		t.fn()
		return
	}
	t.timer = time.AfterFunc(wait, func() {
		t.mu.Lock()
		t.timer, t.last = nil, time.Now()
		t.mu.Unlock()
		t.fn()
	})
	t.mu.Unlock()
}
