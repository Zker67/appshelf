package api

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"appshelf/internal/procs"
	"appshelf/internal/queue"
	"appshelf/internal/state"
)

// checker checks for updates as the settings say, updates the packages set
// to update on their own, tells the user in notifications, and records the
// updates done.
//
// A check reads the catalog's states and the installed programs again; it
// ends once both are read. AppShelf starting is a check too.
type checker struct {
	a   *App
	ctx context.Context // set by App.Start
	now func() time.Time

	mu        sync.Mutex
	checking  bool
	checkedAt time.Time
	nextAt    time.Time
	// told is the version a notification told of, by key: a version is
	// told of once.
	told map[string]string
	// auto are the keys queued by AppShelf itself, until their task ends;
	// results their tasks ended, until all have.
	auto    map[string]bool
	results []state.UpdateRecord
	// seen are the tasks recorded in the history.
	seen    map[int]bool
	paths   []string
	pathsAt time.Time
	// rebooted are the tasks recorded as needing a restart; rebootNew
	// names those not told of yet, told once no task waits or runs.
	rebooted  map[int]bool
	rebootNew []string
	// autoDue is when the first update waiting for its version to have
	// been found long enough is due, zero when none waits.
	autoDue time.Time
}

func newChecker(a *App) *checker {
	return &checker{a: a, ctx: context.Background(), now: time.Now, told: map[string]string{}, auto: map[string]bool{}, seen: map[int]bool{}, rebooted: map[int]bool{}}
}

// checkNow starts a check; one under way goes on, reading again.
//
// The check begins with both readings on the loop, where settle runs: a
// settle in between would see neither reading and end the check with the
// lists of before.
func (a *App) checkNow() {
	a.write([]ChangeKind{ChangedShelf}, func() {
		a.upd.begin()
		a.inv.refresh(0)
		a.core.Refresh()
	})
}

func (u *checker) begin() {
	u.mu.Lock()
	u.checking = true
	u.mu.Unlock()
	u.a.note.mark(ChangedUpdates)
}

func (u *checker) times() (checking bool, checkedAt, nextAt time.Time) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.checking, u.checkedAt, u.nextAt
}

func (u *checker) markAuto(key string) {
	u.mu.Lock()
	u.auto[key] = true
	u.mu.Unlock()
}

// processes returns the executables running, read at most every few
// seconds.
func (u *checker) processes() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	if now := u.now(); now.Sub(u.pathsAt) > 5*time.Second || now.Before(u.pathsAt) {
		p, err := procs.Paths()
		if err != nil {
			p = nil
		}
		u.paths, u.pathsAt = p, now
	}
	return u.paths
}

// afterLoop runs on the loop, after what it ran: tasks ended are recorded,
// and a check whose readings ended is done.
func (u *checker) afterLoop() {
	u.observe()
	u.settle()
}

// settle ends the check when nothing is read any more.
func (u *checker) settle() {
	a := u.a
	if a.core.Refreshing() || a.inv.busy() {
		return
	}
	u.mu.Lock()
	if !u.checking {
		u.mu.Unlock()
		return
	}
	u.checking = false
	u.checkedAt = u.now()
	u.scheduleLocked()
	u.mu.Unlock()
	a.note.mark(ChangedUpdates)
	go u.afterCheck()
}

func (u *checker) every() time.Duration {
	a := u.a
	a.mu.Lock()
	defer a.mu.Unlock()
	return checkIntervals[a.settings.Updates.CheckEvery]
}

func (u *checker) scheduleLocked() {
	every := u.every()
	if every == 0 || u.checkedAt.IsZero() {
		u.nextAt = time.Time{}
		return
	}
	u.nextAt = u.checkedAt.Add(every)
}

func (u *checker) reschedule() {
	u.mu.Lock()
	u.scheduleLocked()
	u.mu.Unlock()
}

// run checks when the next check is due, until ctx ends. It compares the
// wall clock each minute, so that a computer that slept checks once it
// wakes.
func (u *checker) run(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			u.tick()
		}
	}
}

// tick starts the check due, unless one is under way or tasks run, and
// queues the updates whose wait for their version to be found long enough
// has ended.
func (u *checker) tick() {
	u.mu.Lock()
	due := !u.checking && !u.nextAt.IsZero() && !u.now().Before(u.nextAt)
	auto := !u.checking && !u.autoDue.IsZero() && !u.now().Before(u.autoDue)
	if auto {
		u.autoDue = time.Time{}
	}
	u.mu.Unlock()
	if auto && !due {
		u.runAuto()
	}
	if !due {
		return
	}
	if running, _ := u.a.core.Queue.Running(); running {
		return
	}
	u.a.checkNow()
}

// afterCheck updates what is to update on its own, then tells of the
// updates found that were not told of yet.
func (u *checker) afterCheck() {
	a := u.a
	if list := a.updateList(); list.Loaded {
		offers := make(map[string]string, len(list.Items))
		for _, it := range list.Items {
			offers[it.Key] = it.Latest
		}
		_ = a.core.Store().NoteOffered(offers, u.now()) // a missed record only waits again
	}
	queued := u.runAuto()
	list := a.updateList()
	var fresh []string
	u.mu.Lock()
	for _, it := range list.Items {
		// One waiting to be updated on its own is told of when it is.
		if it.Hold != HoldNone || it.Pending || queued[it.Key] || !it.AutoAt.IsZero() || u.told[it.Key] == it.Latest {
			continue
		}
		u.told[it.Key] = it.Latest
		fresh = append(fresh, it.Name)
	}
	u.mu.Unlock()
	if len(fresh) > 0 {
		a.notify(fmt.Sprintf("有 %d 个软件可以更新", len(fresh)), names(fresh)+"。打开软件图书馆查看并更新。")
	}
}

// runAuto queues the updates of the packages set to update on their own
// that can be, and returns their keys. A package whose program runs waits
// for the next check; one whose version has not been found long enough
// waits until it has, which tick sees to.
func (u *checker) runAuto() map[string]bool {
	a := u.a
	var keys []string
	var due time.Time
	for _, it := range a.updateList().Items {
		if it.Mode != UpdateAuto || it.Hold != HoldNone || !it.CanAuto || it.Running || it.Pending {
			continue
		}
		if !it.AutoAt.IsZero() {
			if due.IsZero() || it.AutoAt.Before(due) {
				due = it.AutoAt
			}
			continue
		}
		keys = append(keys, it.Key)
	}
	u.mu.Lock()
	u.autoDue = due
	u.mu.Unlock()
	queued := map[string]bool{}
	if len(keys) > 0 {
		_, _ = a.startUpdates(u.ctx, keys, queued)
	}
	return queued
}

// autoHold tells why an update AppShelf queued itself must not run now,
// read again right before it runs: while it waited behind other tasks, the
// user may have opened the program. One whose state cannot be told waits
// too; the next check queues it again.
func (a *App) autoHold(_ context.Context, it queue.Item) string {
	p := a.packages().byKey[it.App.Source.Key()]
	if p == nil {
		return "本机列表里没有找到它，没有自动更新"
	}
	if block := autoBlock(p); block != "" {
		return "没有自动更新：" + block
	}
	paths, err := procs.Paths()
	if err != nil {
		return "无法确认它是否在运行，没有自动更新：" + err.Error()
	}
	if running(p, paths) {
		return "它正在运行，没有自动更新，下次检查时再更新"
	}
	return ""
}

// autoNote says why AppShelf does not update a package on its own, "" when
// it does: what keeps it from doing so without a window, and, for one set
// to update on its own, an update to its latest version that did not take.
func autoNote(p *pkg, policy state.UpdatePolicy, hist []state.UpdateRecord) string {
	if block := autoBlock(p); block != "" || policy.Mode != state.ModeAuto || !p.update {
		return block
	}
	return autoStuck(hist, p.key, p.latest)
}

// stuckFailures is how many updates to the same version may fail in a row
// before AppShelf stops updating to it on its own.
const stuckFailures = 3

// autoStuck says why AppShelf stops updating key to latest on its own, from
// the history, newest first: an update to latest ran but its source still
// offered latest, as a source whose version numbers do not match the
// program's, or updates to it failed stuckFailures times in a row. A newer
// version is updated to again.
func autoStuck(hist []state.UpdateRecord, key, latest string) string {
	failed := 0
	for _, r := range hist {
		if !strings.EqualFold(r.Key, key) || TaskState(r.Result) == TaskSkipped {
			continue
		}
		if r.Latest == "" || !strings.EqualFold(strings.TrimSpace(r.Latest), strings.TrimSpace(latest)) {
			break
		}
		if r.StillOffered {
			return "更新到 " + latest + " 后，来源仍提示有这个更新（可能是来源记录的版本号和程序的对不上），不再自动更新到这个版本；可以手动更新或跳过它，有新版本时恢复自动更新"
		}
		if TaskState(r.Result) != TaskFailed {
			break
		}
		if failed++; failed >= stuckFailures {
			return fmt.Sprintf("更新到 %s 已连续失败 %d 次，不再自动更新到这个版本；可以手动更新查看原因，有新版本时恢复自动更新", latest, stuckFailures)
		}
	}
	return ""
}

// observe runs on the loop: the updates whose tasks ended go into the
// history, and once all AppShelf started itself have ended, a notification
// says how they went.
func (u *checker) observe() {
	a := u.a
	items := a.core.Queue.Items()
	var recs, summary []state.UpdateRecord
	present := make(map[int]bool, len(items))
	u.mu.Lock()
	for _, it := range items {
		present[it.ID] = true
		if !it.State.Done() {
			delete(u.seen, it.ID) // retried
			continue
		}
		if it.Action != queue.Upgrade || u.seen[it.ID] {
			continue
		}
		u.seen[it.ID] = true
		key := it.App.Source.Key()
		result := taskStateOf(it.State)
		if it.Canceled {
			// Canceled, it did not fail: it does not count as a failure.
			result = TaskSkipped
		}
		rec := state.UpdateRecord{
			Key: key, Name: it.App.Name, From: it.From, To: it.To,
			Result: string(result), Message: it.Message, Auto: u.auto[key], At: it.Ended,
			Latest: it.Latest, StillOffered: it.StillOffered,
		}
		if rec.Auto {
			delete(u.auto, key)
			u.results = append(u.results, rec)
		}
		recs = append(recs, rec)
	}
	for id := range u.seen {
		if !present[id] {
			delete(u.seen, id)
		}
	}
	if len(u.auto) == 0 && len(u.results) > 0 {
		summary, u.results = u.results, nil
	}
	// The tasks that need a restart, recorded once each, told of once
	// none waits or runs.
	var reboots []queue.Item
	active := false
	for _, it := range items {
		switch {
		case !it.State.Done():
			active = true
			delete(u.rebooted, it.ID) // retried
		case it.Reboot && !u.rebooted[it.ID]:
			u.rebooted[it.ID] = true
			reboots = append(reboots, it)
			u.rebootNew = append(u.rebootNew, it.App.Name)
		}
	}
	for id := range u.rebooted {
		if !present[id] {
			delete(u.rebooted, id)
		}
	}
	var tellReboot []string
	if !active && len(u.rebootNew) > 0 {
		tellReboot, u.rebootNew = u.rebootNew, nil
	}
	u.mu.Unlock()
	store := a.core.Store()
	for _, r := range recs {
		_ = store.AddHistory(r) // the history only informs
	}
	for _, it := range reboots {
		_ = store.AddReboot(it.App.Name, it.Ended) // the list only informs
	}
	if len(recs) > 0 || len(reboots) > 0 {
		a.note.mark(ChangedUpdates, ChangedTasks)
	}
	if len(summary) > 0 {
		u.tellAuto(summary)
	}
	if len(tellReboot) > 0 {
		a.notifyTasks(fmt.Sprintf("%d 个软件需要重启电脑", len(tellReboot)), names(tellReboot)+" 要重启电脑后才能完成安装、更新或卸载。")
	}
}

// reboots names the programs whose tasks need Windows to restart, since it
// last started.
func (a *App) reboots() []string {
	recs := a.core.Store().Reboots(procs.BootTime())
	out := make([]string, 0, len(recs))
	for _, r := range recs {
		out = append(out, r.Name)
	}
	return out
}

// tellAuto says how the updates AppShelf started itself went.
func (u *checker) tellAuto(recs []state.UpdateRecord) {
	var ok, failed []string
	for _, r := range recs {
		switch TaskState(r.Result) {
		case TaskSucceeded:
			ok = append(ok, r.Name)
		case TaskFailed:
			failed = append(failed, r.Name)
		}
	}
	switch {
	case len(ok) > 0 && len(failed) == 0:
		u.a.notify(fmt.Sprintf("已自动更新 %d 个软件", len(ok)), names(ok))
	case len(ok) == 0 && len(failed) > 0:
		u.a.notify(fmt.Sprintf("%d 个软件自动更新失败", len(failed)), names(failed)+"。打开软件图书馆查看原因。")
	case len(failed) > 0:
		u.a.notify(fmt.Sprintf("已自动更新 %d 个软件，%d 个失败", len(ok), len(failed)), "失败："+names(failed))
	}
}

// names lists a few names, as "Git、7-Zip 等 5 个".
func names(list []string) string {
	const shown = 3
	if len(list) <= shown {
		return strings.Join(list, "、")
	}
	return strings.Join(list[:shown], "、") + fmt.Sprintf(" 等 %d 个", len(list))
}

// notifyTasks shows a notification of the tasks, unless the window is in
// front, where the page shows the same.
func (a *App) notifyTasks(title, body string) {
	if a.hooks.NotifyTasks == nil || (a.hooks.Active != nil && a.hooks.Active()) {
		return
	}
	go a.hooks.NotifyTasks(title, body)
}

// notify shows a notification, unless the user turned them off or the
// window is in front, where the page shows the same.
func (a *App) notify(title, body string) {
	a.mu.Lock()
	quiet := a.settings.Updates.Quiet
	a.mu.Unlock()
	if quiet || a.hooks.Notify == nil || (a.hooks.Active != nil && a.hooks.Active()) {
		return
	}
	go a.hooks.Notify(title, body)
}
