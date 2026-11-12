package queue

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"appshelf/internal/catalog"
	"appshelf/internal/source"
	"appshelf/internal/uninstall"
)

// fake plays a source: before[id] and after[id] are the states it reports
// before and after running an app, and runs counts the runs.
type fake struct {
	mu       sync.Mutex
	before   map[string]source.Status
	after    map[string]source.Status
	ran      map[string]bool
	runs     []string
	runErr   map[string]error
	result   map[string]source.Result
	finished []string
	gate     chan struct{} // when set, Run waits on it
}

func newFake() *fake {
	return &fake{before: map[string]source.Status{}, after: map[string]source.Status{}, ran: map[string]bool{}, runErr: map[string]error{}, result: map[string]source.Result{}}
}

func (f *fake) Status(_ context.Context, it Item) source.Status {
	a := it.App
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ran[a.ID] {
		return f.after[a.ID]
	}
	return f.before[a.ID]
}

func (f *fake) Run(_ context.Context, it Item, _ io.Writer) (source.Result, error) {
	a := it.App
	if f.gate != nil {
		<-f.gate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ran[a.ID] = true
	f.runs = append(f.runs, a.ID)
	return f.result[a.ID], f.runErr[a.ID]
}

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }

func (f *fake) OpenLog(Item) (io.WriteCloser, string, error) {
	return nopCloser{&bytes.Buffer{}}, "test.log", nil
}

func (f *fake) Finished(it Item, st source.Status, _ source.Result) source.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.finished = append(f.finished, it.App.ID)
	return st
}

func appOf(id string) catalog.App { return catalog.App{ID: id, Name: id} }

func TestSameSourceCannotQueueUnderDifferentIDs(t *testing.T) {
	f := newFake()
	f.gate = make(chan struct{})
	first := catalog.App{ID: "catalog_app", Name: "App", Source: catalog.Source{Type: catalog.Winget, ID: "Vendor.App"}}
	second := first
	second.ID = "installed_app"
	f.before[first.ID] = source.Status{State: source.UpdateAvailable, Version: "1", Latest: "2"}
	f.after[first.ID] = source.Status{State: source.Installed, Version: "2"}
	q := New(context.Background(), f, nil)
	if n := q.Add([]catalog.App{first}, Upgrade); n != 1 {
		t.Fatalf("first add: %d", n)
	}
	if n := q.Add([]catalog.App{second}, Upgrade); n != 0 {
		t.Fatalf("same source queued %d more times", n)
	}
	close(f.gate)
	wait(t, q)
	if len(q.Items()) != 1 {
		t.Fatal("duplicate tasks created")
	}
}

func wait(t *testing.T, q *Queue) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if running, _ := q.Running(); !running {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the queue did not finish")
}

func waitState(t *testing.T, q *Queue, appID string, st ItemState) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if states(q)[appID].State == st {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("%s never became %s", appID, st)
}

// TestStopBeforeStart stops a queue before its first item starts: nothing
// runs, and Resume runs them.
func TestStopBeforeStart(t *testing.T) {
	f := newFake()
	f.gate = make(chan struct{}, 1)
	f.before["a"] = source.Status{State: source.NotInstalled}
	f.after["a"] = source.Status{State: source.Installed}
	q := New(context.Background(), f, nil)
	q.mu.Lock() // hold the worker until Stop has been asked
	q.nextID++
	q.items = append(q.items, &Item{ID: q.nextID, App: appOf("a"), State: Waiting})
	q.startLocked()
	q.stopping = true
	q.mu.Unlock()
	wait(t, q)
	if got := states(q)["a"]; got.State != Waiting || len(f.runs) != 0 {
		t.Fatalf("stopped before starting: %s, runs %v", got.State, f.runs)
	}
	f.gate <- struct{}{}
	q.Resume()
	wait(t, q)
	if got := states(q)["a"]; got.State != Succeeded {
		t.Errorf("after resume: %s (%s)", got.State, got.Message)
	}
}

func states(q *Queue) map[string]Item {
	out := map[string]Item{}
	for _, it := range q.Items() {
		out[it.App.ID] = it
	}
	return out
}

func TestInstallFlow(t *testing.T) {
	f := newFake()
	installed := source.Status{State: source.Installed, Version: "1.0"}
	f.before["new"] = source.Status{State: source.NotInstalled}
	f.after["new"] = installed
	f.before["have"] = installed
	f.before["external"] = source.Status{State: source.Installed, External: true}
	f.before["unknown"] = source.UnknownStatus("winget 出错")
	f.before["unavailable"] = source.Status{State: source.Unavailable, Note: "未安装 scoop"}
	f.before["vanish"] = source.Status{State: source.NotInstalled}
	f.after["vanish"] = source.Status{State: source.NotInstalled}
	f.before["broken"] = source.Status{State: source.NotInstalled}
	f.runErr["broken"] = errors.New("退出码 1603")
	f.before["already"] = source.Status{State: source.NotInstalled}
	f.after["already"] = installed
	f.result["already"] = source.Result{AlreadyInstalled: true}
	f.before["reboot"] = source.Status{State: source.NotInstalled}
	f.after["reboot"] = installed
	f.result["reboot"] = source.Result{RebootRequired: true}

	q := New(context.Background(), f, nil)
	ids := []string{"new", "have", "external", "unknown", "unavailable", "vanish", "broken", "already", "reboot"}
	var apps []catalog.App
	for _, id := range ids {
		apps = append(apps, appOf(id))
	}
	if n := q.Add(apps, Install); n != len(ids) {
		t.Fatalf("added %d", n)
	}
	wait(t, q)
	got := states(q)
	want := map[string]ItemState{
		"new": Succeeded, "have": Skipped, "external": Skipped, "unknown": Failed,
		"unavailable": Failed, "vanish": Failed, "broken": Failed, "already": Skipped, "reboot": Succeeded,
	}
	for id, st := range want {
		if got[id].State != st {
			t.Errorf("%s: %s (%s), want %s", id, got[id].State, got[id].Message, st)
		}
	}
	// Only the apps not installed and not blocked ran.
	ran := map[string]bool{}
	for _, id := range f.runs {
		ran[id] = true
	}
	for _, id := range []string{"have", "external", "unknown", "unavailable"} {
		if ran[id] {
			t.Errorf("%s ran", id)
		}
	}
	if got["reboot"].Message == "" {
		t.Error("the reboot note is missing")
	}
	if len(f.finished) != 5 {
		t.Errorf("finished %v", f.finished)
	}
}

func TestUpgradeFlow(t *testing.T) {
	f := newFake()
	f.before["old"] = source.Status{State: source.UpdateAvailable, Version: "1.0", Latest: "2.0"}
	f.after["old"] = source.Status{State: source.Installed, Version: "2.0"}
	f.before["current"] = source.Status{State: source.Installed, Version: "2.0"}
	f.before["absent"] = source.Status{State: source.NotInstalled}
	f.before["stuck"] = source.Status{State: source.UpdateAvailable, Version: "1.0", Latest: "2.0"}
	f.after["stuck"] = source.Status{State: source.UpdateAvailable, Version: "1.0", Latest: "2.0"}
	f.before["external"] = source.Status{State: source.Installed, External: true}
	f.before["unchecked"] = source.Status{State: source.Installed, Version: "1.0", Note: "无法检查更新：GitHub 请求额度已用完"}

	q := New(context.Background(), f, nil)
	q.Add([]catalog.App{appOf("old"), appOf("current"), appOf("absent"), appOf("stuck"), appOf("external"), appOf("unchecked")}, Upgrade)
	wait(t, q)
	got := states(q)
	want := map[string]ItemState{"old": Succeeded, "current": Skipped, "absent": Skipped, "stuck": Failed, "external": Skipped, "unchecked": Skipped}
	for id, st := range want {
		if got[id].State != st {
			t.Errorf("%s: %s (%s), want %s", id, got[id].State, got[id].Message, st)
		}
	}
	if got["current"].Message != "已是最新版本" || got["unchecked"].Message != "没有发现更新（无法检查更新：GitHub 请求额度已用完）" {
		t.Errorf("messages: %q, %q", got["current"].Message, got["unchecked"].Message)
	}
	if got["old"].Latest != "2.0" || got["old"].StillOffered || !got["stuck"].StillOffered {
		t.Errorf("still offered: %+v, %+v", got["old"], got["stuck"])
	}
}

// An update whose version changed while its source still offers the
// version it was to bring succeeds, saying so.
func TestUpgradeStillOffered(t *testing.T) {
	f := newFake()
	f.before["app"] = source.Status{State: source.UpdateAvailable, Version: "1.0", Latest: "2.0"}
	f.after["app"] = source.Status{State: source.UpdateAvailable, Version: "2.0.0", Latest: " 2.0 "}
	f.before["newer"] = source.Status{State: source.UpdateAvailable, Version: "1.0", Latest: "2.0"}
	f.after["newer"] = source.Status{State: source.UpdateAvailable, Version: "2.0", Latest: "3.0"}
	q := New(context.Background(), f, nil)
	q.Add([]catalog.App{appOf("app"), appOf("newer")}, Upgrade)
	wait(t, q)
	got := states(q)
	if it := got["app"]; it.State != Succeeded || !it.StillOffered || !strings.Contains(it.Message, "来源仍提示可以更新到 2.0") {
		t.Errorf("app: %+v", it)
	}
	if it := got["newer"]; it.State != Succeeded || it.StillOffered || it.Message != "" {
		t.Errorf("a newer version came out: %+v", it)
	}
}

// A failed update can run again with its installer's window, as the
// user's, not AppShelf's own; an install cannot.
func TestRetryUpdateWithAWindow(t *testing.T) {
	f := newFake()
	for _, id := range []string{"up", "in"} {
		f.runErr[id] = errors.New("安装程序运行失败")
	}
	f.before["up"] = source.Status{State: source.UpdateAvailable, Version: "1.0", Latest: "2.0"}
	f.after["up"] = f.before["up"]
	f.before["in"] = source.Status{State: source.NotInstalled}
	q := New(context.Background(), f, nil)
	q.SetHold(func(context.Context, Item) string { return "" })
	q.AddAuto([]catalog.App{appOf("up")})
	q.Add([]catalog.App{appOf("in")}, Install)
	wait(t, q)
	got := states(q)
	if got["up"].State != Failed || got["in"].State != Failed {
		t.Fatalf("%+v", got)
	}
	if q.RetryInteractive(got["in"].ID) {
		t.Error("an install retried with a window")
	}
	if !q.RetryInteractive(got["up"].ID) {
		t.Fatal("not retried")
	}
	wait(t, q)
	if it := states(q)["up"]; !it.Interactive || it.Auto {
		t.Errorf("retried: %+v", it)
	}
}

// The hold keeps back only the updates AppShelf queued itself, right
// before they run, once they are found to have an update.
func TestAutoHold(t *testing.T) {
	f := newFake()
	for _, id := range []string{"manual", "held", "free", "current"} {
		f.before[id] = source.Status{State: source.UpdateAvailable, Version: "1.0", Latest: "2.0"}
		f.after[id] = source.Status{State: source.Installed, Version: "2.0"}
	}
	f.before["current"] = source.Status{State: source.Installed, Version: "2.0"}
	var mu sync.Mutex
	var asked []string
	q := New(context.Background(), f, nil)
	q.SetHold(func(_ context.Context, it Item) string {
		mu.Lock()
		defer mu.Unlock()
		asked = append(asked, it.App.ID)
		if !it.Auto || it.Latest != "2.0" {
			t.Errorf("held %+v", it)
		}
		if it.App.ID == "held" {
			return "它正在运行"
		}
		return ""
	})
	if n := q.AddAuto([]catalog.App{appOf("held"), appOf("free"), appOf("current")}); n != 3 {
		t.Fatalf("added %d", n)
	}
	q.Add([]catalog.App{appOf("manual")}, Upgrade)
	wait(t, q)
	got := states(q)
	if it := got["held"]; it.State != Skipped || it.Message != "它正在运行" || it.LogPath != "" || !it.Auto {
		t.Errorf("held: %+v", it)
	}
	if got["free"].State != Succeeded || got["manual"].State != Succeeded || got["manual"].Auto {
		t.Errorf("free %+v, manual %+v", got["free"], got["manual"])
	}
	f.mu.Lock()
	runs := slices.Clone(f.runs)
	f.mu.Unlock()
	// The user's goes before the updates AppShelf queued that wait.
	if !slices.Equal(runs, []string{"manual", "free"}) {
		t.Errorf("runs %v", runs)
	}
	mu.Lock()
	defer mu.Unlock()
	// Not asked of one already current, nor of one the user started.
	if !slices.Equal(asked, []string{"held", "free"}) {
		t.Errorf("asked %v", asked)
	}
}

func TestRetryStopAndDuplicates(t *testing.T) {
	f := newFake()
	f.gate = make(chan struct{})
	for _, id := range []string{"a", "b", "c"} {
		f.before[id] = source.Status{State: source.NotInstalled}
		f.after[id] = source.Status{State: source.Installed}
	}
	f.runErr["a"] = errors.New("失败一次")

	q := New(context.Background(), f, nil)
	q.Add([]catalog.App{appOf("a"), appOf("b"), appOf("c")}, Install)
	// The same app again while it waits is not queued twice.
	if n := q.Add([]catalog.App{appOf("b")}, Install); n != 0 {
		t.Errorf("b queued twice")
	}
	waitState(t, q, "a", Running)
	q.Stop()
	f.gate <- struct{}{} // a runs, fails; the queue stops after it
	wait(t, q)
	got := states(q)
	if got["a"].State != Failed || got["b"].State != Waiting || got["c"].State != Waiting {
		t.Fatalf("after stop: a %s, b %s, c %s", got["a"].State, got["b"].State, got["c"].State)
	}

	delete(f.runErr, "a")
	f.mu.Lock()
	f.ran["a"] = false
	f.mu.Unlock()
	if !q.Retry(got["a"].ID) {
		t.Fatal("retry refused")
	}
	for range 3 {
		f.gate <- struct{}{}
	}
	wait(t, q)
	got = states(q)
	for _, id := range []string{"a", "b", "c"} {
		if got[id].State != Succeeded {
			t.Errorf("%s: %s (%s)", id, got[id].State, got[id].Message)
		}
	}
	items := q.Items()
	if items[len(items)-1].App.ID != "a" {
		t.Errorf("the retried item is not last: %+v", items)
	}
	q.ClearDone()
	if len(q.Items()) != 0 {
		t.Errorf("left %+v", q.Items())
	}
}

// talker is a fake whose runs write output and report progress, then wait
// on gate.
type talker struct {
	*fake
	wrote chan struct{}
}

func (t *talker) Run(ctx context.Context, it Item, log io.Writer) (source.Result, error) {
	fmt.Fprint(log, "downloading\r\n")
	source.ReportProgress(log, source.Progress{Stage: "正在下载", Done: 10, Total: 100})
	t.wrote <- struct{}{}
	return t.fake.Run(ctx, it, log)
}

func TestRunningItemShowsStageProgressAndLog(t *testing.T) {
	f := newFake()
	f.gate = make(chan struct{})
	f.before["a"] = source.Status{State: source.NotInstalled}
	f.after["a"] = source.Status{State: source.Installed, Version: "1"}
	tk := &talker{fake: f, wrote: make(chan struct{})}
	var pokes atomic.Int32
	q := New(context.Background(), tk, nil)
	q.SetProgress(func() { pokes.Add(1) })
	q.Add([]catalog.App{appOf("a")}, Install)
	<-tk.wrote

	it := q.Items()[0]
	if it.State != Running || it.Stage != "正在下载" || it.Progress.Done != 10 || it.Progress.Total != 100 {
		t.Fatalf("running item = %+v", it)
	}
	c, ok := q.Log(it.ID, 0)
	if !ok || len(c.Lines) != 1 || c.Lines[0] != "downloading" {
		t.Fatalf("log = %+v %v", c, ok)
	}
	close(f.gate)
	wait(t, q)
	it = q.Items()[0]
	if it.State != Succeeded || it.Stage != "" || it.Progress != (source.Progress{}) {
		t.Fatalf("finished item = %+v", it)
	}
	// The log stays readable after the item ends, with the queue's lines.
	c, _ = q.Log(it.ID, 1)
	if len(c.Lines) == 0 || !strings.HasPrefix(c.Lines[len(c.Lines)-1], "[AppShelf] 之后的状态") {
		t.Fatalf("log after = %q", c.Lines)
	}
	if pokes.Load() == 0 {
		t.Fatal("progress was not announced")
	}
	if _, ok := q.Log(999, 0); ok {
		t.Fatal("an unknown item has no log")
	}
}

func TestThrottleDelaysTheLastCall(t *testing.T) {
	var calls atomic.Int32
	th := throttle{fn: func() { calls.Add(1) }, every: 40 * time.Millisecond}
	for range 10 {
		th.poke()
	}
	if n := calls.Load(); n != 1 {
		t.Fatalf("calls at once = %d, want 1", n)
	}
	time.Sleep(80 * time.Millisecond)
	if n := calls.Load(); n != 2 {
		t.Fatalf("calls after the delay = %d, want 2", n)
	}
}

// remover is a fake whose removals take the program away unless kept, and
// remembers whether they ran with the uninstaller's window.
type remover struct {
	*fake
	interactive []bool
}

func (r *remover) Run(ctx context.Context, it Item, log io.Writer) (source.Result, error) {
	r.mu.Lock()
	r.interactive = append(r.interactive, it.Interactive)
	r.mu.Unlock()
	return r.fake.Run(ctx, it, log)
}

func TestUninstall(t *testing.T) {
	f := newFake()
	r := &remover{fake: f}
	gone := uninstall.Target{Key: "arp:gone", Name: "Gone", Method: uninstall.ByRegistry}
	stays := uninstall.Target{Key: "arp:stays", Name: "Stays", Method: uninstall.ByRegistry}
	noisy := uninstall.Target{Key: "arp:noisy", Name: "Noisy", Method: uninstall.ByRegistry}
	already := uninstall.Target{Key: "arp:already", Name: "Already", Method: uninstall.ByRegistry}
	unknown := uninstall.Target{Key: "arp:unknown", Name: "Unknown", Method: uninstall.ByRegistry}
	for _, tg := range []uninstall.Target{gone, stays, noisy} {
		f.before[UninstallApp(tg).ID] = source.Status{State: source.Installed}
	}
	f.after[UninstallApp(gone).ID] = source.Status{State: source.NotInstalled}
	f.after[UninstallApp(stays).ID] = source.Status{State: source.Installed}
	f.runErr[UninstallApp(stays).ID] = errors.New("卸载程序以退出码 2 结束")
	f.after[UninstallApp(noisy).ID] = source.Status{State: source.NotInstalled}
	f.runErr[UninstallApp(noisy).ID] = errors.New("卸载程序以退出码 5 结束")
	f.before[UninstallApp(already).ID] = source.Status{State: source.NotInstalled}
	f.before[UninstallApp(unknown).ID] = source.UnknownStatus("读不到注册表")

	q := New(context.Background(), r, nil)
	if n := q.AddUninstall([]uninstall.Target{gone, stays, noisy, already, unknown, gone}); n != 5 {
		t.Fatalf("added %d, want 5: the same program once", n)
	}
	wait(t, q)
	got := map[string]Item{}
	for _, it := range q.Items() {
		got[it.Target.Key] = it
		if it.Action != Uninstall || it.App.ID != "uninstall:"+it.Target.Key {
			t.Errorf("item = %+v", it)
		}
	}
	check := func(key string, st ItemState, msg string) {
		t.Helper()
		if it := got[key]; it.State != st || !strings.Contains(it.Message, msg) {
			t.Errorf("%s = %s %q, want %s %q", key, it.State, it.Message, st, msg)
		}
	}
	check("arp:gone", Succeeded, "")
	check("arp:stays", Failed, "退出码 2")
	check("arp:noisy", Succeeded, "已卸载（卸载程序报告：卸载程序以退出码 5 结束）")
	check("arp:already", Skipped, "已不在本机")
	check("arp:unknown", Failed, "无法确认")
	if slices.Contains(f.runs, UninstallApp(already).ID) || slices.Contains(f.runs, UninstallApp(unknown).ID) {
		t.Errorf("ran %q: a program gone or unknown must not run", f.runs)
	}

	// A failed removal retries with its window; an install does not.
	if !q.RetryInteractive(got["arp:stays"].ID) {
		t.Fatal("RetryInteractive refused a failed removal")
	}
	wait(t, q)
	if n := len(r.interactive); n == 0 || !r.interactive[n-1] {
		t.Fatalf("interactive runs = %v", r.interactive)
	}
	f.before["x"] = source.Status{State: source.NotInstalled}
	f.runErr["x"] = errors.New("no")
	q.Add([]catalog.App{appOf("x")}, Install)
	wait(t, q)
	for _, it := range q.Items() {
		if it.App.ID == "x" && q.RetryInteractive(it.ID) {
			t.Error("an install retried interactively")
		}
	}
}

// A removal judged failed whose program its uninstaller removed after the
// check succeeds when retried: it does not run again, nor count as skipped.
func TestRetriedRemovalAlreadyDone(t *testing.T) {
	f := newFake()
	late := uninstall.Target{Key: "winget:upscayl.upscayl", Name: "Upscayl", Method: uninstall.ByWinget, WingetID: "Upscayl.Upscayl"}
	id := UninstallApp(late).ID
	f.before[id] = source.Status{State: source.Installed}
	f.after[id] = source.Status{State: source.Installed}
	q := New(context.Background(), f, nil)
	q.AddUninstall([]uninstall.Target{late})
	wait(t, q)
	it := q.Items()[0]
	if it.State != Failed {
		t.Fatalf("first run = %s %q", it.State, it.Message)
	}

	f.mu.Lock()
	f.after[id] = source.Status{State: source.NotInstalled}
	f.mu.Unlock()
	if !q.Retry(it.ID) {
		t.Fatal("Retry refused a failed removal")
	}
	wait(t, q)
	it = q.Items()[0]
	if it.State != Succeeded || !strings.Contains(it.Message, "上次卸载其实已经完成") {
		t.Errorf("retry = %s %q", it.State, it.Message)
	}
	if len(f.runs) != 1 {
		t.Errorf("runs = %q: the retry must not run again", f.runs)
	}
	if len(f.finished) != 2 {
		t.Errorf("finished = %q: the retry is reported as done", f.finished)
	}
}

func TestUninstallDoesNotRunBesideTheSamePackage(t *testing.T) {
	f := newFake()
	f.gate = make(chan struct{})
	app := catalog.App{ID: "a_git", Name: "Git", Source: catalog.Source{Type: catalog.Winget, ID: "Git.Git"}}
	f.before[app.ID] = source.Status{State: source.UpdateAvailable, Version: "1", Latest: "2"}
	q := New(context.Background(), f, nil)
	q.Add([]catalog.App{app}, Upgrade)
	if n := q.AddUninstall([]uninstall.Target{{Key: "winget:git.git", Name: "Git", Method: uninstall.ByWinget, WingetID: "Git.Git"}}); n != 0 {
		t.Fatal("a removal queued beside an update of the same package")
	}
	close(f.gate)
	wait(t, q)
}

// controlled is a fake whose quiet removals wait for a command of the
// user, and whose removals with the window take the program away.
type controlled struct {
	*fake
	got         chan uninstall.Command
	interactive []bool
}

func (c *controlled) Run(ctx context.Context, it Item, log io.Writer) (source.Result, error) {
	c.mu.Lock()
	c.interactive = append(c.interactive, it.Interactive)
	c.mu.Unlock()
	if it.Interactive {
		c.mu.Lock()
		c.after[it.App.ID] = source.Status{State: source.NotInstalled}
		c.mu.Unlock()
		return c.fake.Run(ctx, it, log)
	}
	cmd := <-it.Control
	c.got <- cmd
	c.fake.Run(ctx, it, log)
	return source.Result{}, errors.New("卸载程序已被结束")
}

// Commands reach a removal while it runs, and only then; switching to the
// window runs it again so when the program is still there.
func TestControlAndSwitchToWindow(t *testing.T) {
	f := newFake()
	c := &controlled{fake: f, got: make(chan uninstall.Command, 1)}
	tg := uninstall.Target{Key: "arp:app", Name: "App", Method: uninstall.ByRegistry}
	id := UninstallApp(tg).ID
	f.before[id] = source.Status{State: source.Installed}
	f.after[id] = source.Status{State: source.Installed}
	q := New(context.Background(), c, nil)
	if q.Control(99, uninstall.StopWaiting) {
		t.Error("a command reached no task")
	}
	q.AddUninstall([]uninstall.Target{tg})
	waitState(t, q, id, Running)
	item := q.Items()[0]
	if !q.Control(item.ID, uninstall.SwitchToWindow) {
		t.Fatal("the running removal refused the command")
	}
	if got := <-c.got; got != uninstall.SwitchToWindow {
		t.Fatalf("got %v", got)
	}
	wait(t, q)
	it := q.Items()[0]
	if it.State != Succeeded || len(c.interactive) != 2 || c.interactive[0] || !c.interactive[1] {
		t.Fatalf("item %s %q, runs interactive %v", it.State, it.Message, c.interactive)
	}
	if q.Control(it.ID, uninstall.StopWaiting) {
		t.Error("a finished task took a command")
	}
}

// A removal the user canceled, its program still there, fails marked so;
// a retry clears the mark.
func TestCanceledRemoval(t *testing.T) {
	f := newFake()
	tg := uninstall.Target{Key: "arp:app", Name: "App", Method: uninstall.ByRegistry}
	id := UninstallApp(tg).ID
	f.before[id] = source.Status{State: source.Installed}
	f.after[id] = source.Status{State: source.Installed, Note: "原因"}
	f.runErr[id] = fmt.Errorf("%w：你拒绝了管理员权限，没有卸载", uninstall.ErrCanceled)
	q := New(context.Background(), f, nil)
	q.AddUninstall([]uninstall.Target{tg})
	wait(t, q)
	it := q.Items()[0]
	if it.State != Failed || !it.Canceled || !strings.Contains(it.Message, "拒绝了管理员权限") {
		t.Fatalf("canceled = %s %v %q", it.State, it.Canceled, it.Message)
	}
	f.mu.Lock()
	f.runErr[id] = errors.New("卸载程序以退出码 2 结束")
	f.mu.Unlock()
	q.Retry(it.ID)
	wait(t, q)
	it = q.Items()[0]
	if it.State != Failed || it.Canceled || it.Message != "卸载程序以退出码 2 结束：原因" {
		t.Errorf("failed = %s %v %q", it.State, it.Canceled, it.Message)
	}
}

// The user's items go before the updates AppShelf queued itself that
// wait; the one running goes on.
func TestUserItemsGoFirst(t *testing.T) {
	f := newFake()
	f.gate = make(chan struct{})
	for _, id := range []string{"a1", "a2", "m1", "m2"} {
		f.before[id] = source.Status{State: source.UpdateAvailable, Version: "1", Latest: "2"}
		f.after[id] = source.Status{State: source.Installed, Version: "2"}
	}
	q := New(context.Background(), f, nil)
	q.AddAuto([]catalog.App{appOf("a1"), appOf("a2")})
	waitState(t, q, "a1", Running)
	q.Add([]catalog.App{appOf("m1")}, Upgrade)
	q.Add([]catalog.App{appOf("m2")}, Upgrade)
	var order []string
	for _, it := range q.Items() {
		order = append(order, it.App.ID)
	}
	if !slices.Equal(order, []string{"a1", "m1", "m2", "a2"}) {
		t.Errorf("order %v", order)
	}
	close(f.gate)
	wait(t, q)
	f.mu.Lock()
	defer f.mu.Unlock()
	if !slices.Equal(f.runs, []string{"a1", "m1", "m2", "a2"}) {
		t.Errorf("runs %v", f.runs)
	}
}

// stopper reports a stage that can stop, or not, then waits to be
// canceled or released; done is the state it leaves when canceled.
type stopper struct {
	*fake
	cancelable bool
	started    chan struct{}
	release    chan struct{}
	upgraded   bool // the update got done before it stopped
}

func (s *stopper) Run(ctx context.Context, it Item, log io.Writer) (source.Result, error) {
	source.ReportProgress(log, source.Progress{Stage: "正在下载", Cancelable: s.cancelable})
	select {
	case s.started <- struct{}{}:
	default:
	}
	select {
	case <-ctx.Done():
		if s.upgraded {
			s.mu.Lock()
			s.ran[it.App.ID] = true
			s.mu.Unlock()
		}
		return source.Result{}, ctx.Err()
	case <-s.release:
		s.mu.Lock()
		s.ran[it.App.ID] = true
		s.mu.Unlock()
		return source.Result{RebootRequired: true}, nil
	}
}

func TestCancel(t *testing.T) {
	setup := func(cancelable, upgraded bool) (*Queue, *stopper) {
		f := newFake()
		for _, id := range []string{"run", "wait"} {
			f.before[id] = source.Status{State: source.UpdateAvailable, Version: "1", Latest: "2"}
			f.after[id] = source.Status{State: source.Installed, Version: "2"}
		}
		s := &stopper{fake: f, cancelable: cancelable, started: make(chan struct{}, 1), release: make(chan struct{}), upgraded: upgraded}
		q := New(context.Background(), s, nil)
		q.Add([]catalog.App{appOf("run"), appOf("wait")}, Upgrade)
		<-s.started
		return q, s
	}
	idOf := func(q *Queue, app string) int {
		for _, it := range q.Items() {
			if it.App.ID == app {
				return it.ID
			}
		}
		return 0
	}

	// While it downloads: canceled, it changed nothing.
	q, s := setup(true, false)
	items := states(q)
	if !items["run"].CanCancel() || !items["wait"].CanCancel() {
		t.Fatalf("cancelable: %+v", items)
	}
	if !q.Cancel(idOf(q, "wait")) || !q.Cancel(idOf(q, "run")) || q.Cancel(idOf(q, "run")) {
		t.Fatal("not canceled, or canceled twice")
	}
	wait(t, q)
	got := states(q)
	if it := got["run"]; it.State != Failed || !it.Canceled || it.Message != "已取消" || it.CanCancel() {
		t.Errorf("run: %+v", it)
	}
	if it := got["wait"]; it.State != Failed || !it.Canceled || it.LogPath != "" {
		t.Errorf("wait: %+v", it)
	}
	if !q.Retry(idOf(q, "wait")) {
		t.Error("a canceled item cannot retry")
	}
	close(s.release)
	wait(t, q)
	if it := states(q)["wait"]; it.State != Succeeded || it.Canceled || !it.Reboot {
		t.Errorf("retried: %+v", it)
	}

	// Done all the same before it stopped: it succeeded.
	q, s = setup(true, true)
	q.Cancel(idOf(q, "run"))
	close(s.release)
	wait(t, q)
	if it := states(q)["run"]; it.State != Succeeded || it.Canceled || it.Message != "取消时已经更新好了" {
		t.Errorf("done anyway: %+v", it)
	}

	// Past where it can stop, it is not canceled.
	q, s = setup(false, false)
	if q.Cancel(idOf(q, "run")) || states(q)["run"].CanCancel() {
		t.Error("canceled where it cannot stop")
	}
	close(s.release)
	wait(t, q)
	if it := states(q)["run"]; it.State != Succeeded || !it.Reboot {
		t.Errorf("ran: %+v", it)
	}
}
