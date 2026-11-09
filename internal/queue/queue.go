// Package queue installs, updates and removes apps one at a time. Each item
// checks the app again right before it runs, and again after: an app found
// installed is skipped, an app whose state cannot be read is not touched,
// an install counts only when the app is then found installed, and a
// removal only when it is then gone.
package queue

import (
	"context"
	"fmt"
	"io"
	"slices"
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
	// Stage is what a running item does now, and Progress how far its
	// download has come, as far as the source reports it.
	Stage    string
	Progress source.Progress
	// Target is the program a removal removes; Interactive asks for its
	// uninstaller's window.
	Target      *uninstall.Target
	Interactive bool

	log *tasklog.Log
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

// Add queues the apps with the action and starts the queue. An app that
// already has an item waiting or running is not queued again. It returns
// how many items it added.
func (q *Queue) Add(apps []catalog.App, action Action) int {
	q.mu.Lock()
	added := 0
	for _, a := range apps {
		if q.pendingLocked(a.ID) || q.pendingSourceLocked(a.Source) {
			continue
		}
		q.nextID++
		q.items = append(q.items, &Item{ID: q.nextID, App: a.Clone(), Action: action, State: Waiting})
		added++
	}
	q.startLocked()
	q.mu.Unlock()
	q.notify()
	return added
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

// Retry queues a failed item again, at the end, and starts the queue.
func (q *Queue) Retry(id int) bool { return q.retry(id, false) }

// RetryInteractive queues a failed removal again, with its uninstaller's
// window.
func (q *Queue) RetryInteractive(id int) bool { return q.retry(id, true) }

func (q *Queue) retry(id int, interactive bool) bool {
	q.mu.Lock()
	i := slices.IndexFunc(q.items, func(it *Item) bool { return it.ID == id })
	if i < 0 || q.items[i].State != Failed || q.pendingLocked(q.items[i].App.ID) || q.pendingSourceLocked(q.items[i].App.Source) {
		q.mu.Unlock()
		return false
	}
	if interactive && q.items[i].Action != Uninstall {
		q.mu.Unlock()
		return false
	}
	it := q.items[i]
	if interactive {
		it.Interactive = true
	}
	q.items = slices.Delete(q.items, i, i+1)
	it.State, it.Message, it.LogPath = Waiting, "", ""
	it.From, it.To = "", ""
	it.Stage, it.Progress, it.log = "", source.Progress{}, nil
	it.Started, it.Ended = time.Time{}, time.Time{}
	q.items = append(q.items, it)
	q.startLocked()
	q.mu.Unlock()
	q.notify()
	return true
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
		snapshot := *it
		q.mu.Unlock()
		q.notify()

		state, msg, logPath := q.runItem(&snapshot, live{q, it})

		q.mu.Lock()
		it.State, it.Message, it.LogPath, it.Ended = state, msg, logPath, time.Now()
		it.From, it.To = snapshot.From, snapshot.To
		it.Stage, it.Progress = "", source.Progress{}
		q.mu.Unlock()
		q.notify()
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

func (l live) attach(log *tasklog.Log) {
	l.q.mu.Lock()
	l.it.log = log
	l.q.mu.Unlock()
}

// runItem does one item: check, run, check again.
func (q *Queue) runItem(it *Item, live live) (ItemState, string, string) {
	ctx := q.ctx
	before := q.exec.Status(ctx, *it)
	it.From = before.Version
	if state, msg, skip := judgeBefore(*it, before); skip {
		return state, msg, ""
	}

	file, logPath, err := q.exec.OpenLog(*it)
	if err != nil {
		return Failed, "无法创建日志：" + err.Error(), ""
	}
	log := tasklog.New(file, q.progress.poke)
	live.attach(log)
	live.stage(StageStarting)
	res, runErr := q.exec.Run(ctx, *it, log)
	if runErr != nil {
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
		state, msg := judgeRemoval(after, res, runErr)
		return state, msg, logPath
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
	return Succeeded, note, logPath
}

// judgeBefore tells whether an item must not run, from the state read
// right before it.
func judgeBefore(it Item, before source.Status) (ItemState, string, bool) {
	switch {
	case it.Action == Uninstall && before.Blocked():
		return Failed, "无法确认它是否还在本机，没有执行：" + noteOr(before, "原因未知"), true
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
// uninstaller said only explains.
func judgeRemoval(after source.Status, res source.Result, runErr error) (ItemState, string) {
	switch {
	case after.Blocked():
		return Failed, "卸载程序已结束，但无法确认结果：" + noteOr(after, "原因未知")
	case after.IsInstalled() && runErr != nil:
		return Failed, runErr.Error()
	case after.IsInstalled():
		return Failed, "卸载程序已结束，但这个软件还在本机"
	case res.RebootRequired:
		return Succeeded, "需要重启电脑才能完成"
	case runErr != nil:
		return Succeeded, "已卸载（卸载程序报告：" + runErr.Error() + "）"
	}
	return Succeeded, ""
}

func presence(st source.Status) string {
	switch {
	case st.Blocked():
		return "无法确认（" + noteOr(st, "原因未知") + "）"
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
