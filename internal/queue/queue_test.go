package queue

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"appshelf/internal/catalog"
	"appshelf/internal/source"
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

func (f *fake) Status(_ context.Context, a catalog.App) source.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ran[a.ID] {
		return f.after[a.ID]
	}
	return f.before[a.ID]
}

func (f *fake) Run(_ context.Context, a catalog.App, _ Action, _ io.Writer) (source.Result, error) {
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
