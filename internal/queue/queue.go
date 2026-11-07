// Package queue installs and updates apps one at a time. Each item checks
// the app again right before it runs, and again after: an app found
// installed is skipped, an app whose state cannot be read is not touched,
// and an install counts only when the app is then found installed.
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
)

// Action is what an item does.
type Action int

const (
	Install Action = iota
	Upgrade
)

func (a Action) String() string {
	if a == Upgrade {
		return "更新"
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

// Item is one install or update.
type Item struct {
	ID      int
	App     catalog.App
	Action  Action
	State   ItemState
	Message string // why it was skipped or failed, or a note on success
	LogPath string
	Started time.Time
	Ended   time.Time
}

// Executor does the work of the items. Its methods run on the queue's
// goroutine.
type Executor interface {
	// Status reads the app's state now, with its detection rule.
	Status(ctx context.Context, app catalog.App) source.Status
	// Run installs or updates the app, writing its output to log.
	Run(ctx context.Context, app catalog.App, action Action, log io.Writer) (source.Result, error)
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
	exec   Executor
	notify func()
	ctx    context.Context

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
	return &Queue{exec: exec, notify: notify, ctx: ctx}
}

// Add queues the apps with the action and starts the queue. An app that
// already has an item waiting or running is not queued again. It returns
// how many items it added.
func (q *Queue) Add(apps []catalog.App, action Action) int {
	q.mu.Lock()
	added := 0
	for _, a := range apps {
		if q.pendingLocked(a.ID) {
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

// Retry queues a failed item again, at the end, and starts the queue.
func (q *Queue) Retry(id int) bool {
	q.mu.Lock()
	i := slices.IndexFunc(q.items, func(it *Item) bool { return it.ID == id })
	if i < 0 || q.items[i].State != Failed || q.pendingLocked(q.items[i].App.ID) {
		q.mu.Unlock()
		return false
	}
	it := q.items[i]
	q.items = slices.Delete(q.items, i, i+1)
	it.State, it.Message, it.LogPath = Waiting, "", ""
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
	}
	return out
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
		it.State, it.Started = Running, time.Now()
		snapshot := *it
		q.mu.Unlock()
		q.notify()

		state, msg, logPath := q.runItem(snapshot)

		q.mu.Lock()
		it.State, it.Message, it.LogPath, it.Ended = state, msg, logPath, time.Now()
		q.mu.Unlock()
		q.notify()
	}
}

// runItem does one item: check, run, check again.
func (q *Queue) runItem(it Item) (ItemState, string, string) {
	ctx := q.ctx
	before := q.exec.Status(ctx, it.App)
	switch {
	case before.Blocked():
		return Failed, "无法确认安装状态，没有执行：" + noteOr(before, "原因未知"), ""
	case it.Action == Install && before.IsInstalled():
		if before.External {
			return Skipped, "已安装（不是通过这个来源安装的）", ""
		}
		return Skipped, "已安装", ""
	case it.Action == Upgrade && before.State == source.NotInstalled:
		return Skipped, "未安装，无法更新", ""
	case it.Action == Upgrade && before.External:
		return Skipped, "不是通过这个来源安装的，请用原来的方式更新", ""
	case it.Action == Upgrade && before.State != source.UpdateAvailable:
		if before.Note != "" {
			// The check may not have been able to look for updates.
			return Skipped, "没有发现更新（" + before.Note + "）", ""
		}
		return Skipped, "已是最新版本", ""
	}

	log, logPath, err := q.exec.OpenLog(it)
	if err != nil {
		return Failed, "无法创建日志：" + err.Error(), ""
	}
	res, runErr := q.exec.Run(ctx, it.App, it.Action, log)
	if runErr != nil {
		fmt.Fprintf(log, "\n[AppShelf] %s失败：%v\n", it.Action, runErr)
	}
	after := q.exec.Status(ctx, it.App)
	it.LogPath = logPath
	after = q.exec.Finished(it, after, res)
	fmt.Fprintf(log, "\n[AppShelf] 之后的状态：%s %s %s\n", after.State, after.Version, after.Note)
	log.Close()

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

func noteOr(st source.Status, fallback string) string {
	if st.Note != "" {
		return st.Note
	}
	return fallback
}
