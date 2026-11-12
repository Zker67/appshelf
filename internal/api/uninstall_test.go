package api

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"appshelf/internal/catalog"
	"appshelf/internal/detect"
	"appshelf/internal/source"
	"appshelf/internal/source/winget"
	"appshelf/internal/uninstall"
)

// fakeUninstaller plays removing programs: a program run is gone unless
// kept, and the runs are remembered.
type fakeUninstaller struct {
	mu   sync.Mutex
	gone map[string]bool // by key
	keep map[string]bool // programs that stay after their run
	runs []string        // key and "i" or "q"
	// hold, when set, keeps a quiet run waiting for the user until closed.
	hold chan struct{}
	// then, when set, is called with the key of a program removed.
	then func(key string)
}

func (f *fakeUninstaller) Inspect(_ context.Context, targets []uninstall.Target) []uninstall.Inspection {
	out := make([]uninstall.Inspection, len(targets))
	for i, t := range targets {
		out[i] = uninstall.Inspection{Plan: uninstall.PlanFor(t, false, uninstall.Options{})}
	}
	return out
}

func (f *fakeUninstaller) Present(_ context.Context, t uninstall.Target) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return !f.gone[t.Key], nil
}

func (f *fakeUninstaller) Remains(uninstall.Target) string { return "" }

func (f *fakeUninstaller) Run(_ context.Context, t uninstall.Target, interactive bool, log io.Writer) (source.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	mode := "q"
	if interactive {
		mode = "i"
	}
	f.runs = append(f.runs, t.Key+"|"+mode)
	fmt.Fprintln(log, "removing", t.Name)
	if hold := f.hold; hold != nil && !interactive {
		f.mu.Unlock()
		source.ReportProgress(log, source.Progress{Stage: "卸载程序弹出了窗口，可能在等你操作", Wait: uninstall.WaitUser, Prompt: "「卸载」请先关闭程序", Window: true})
		<-hold
		f.mu.Lock()
	}
	if f.keep[t.Key] {
		return source.Result{}, fmt.Errorf("卸载程序以退出码 2 结束")
	}
	f.gone[t.Key] = true
	if f.then != nil {
		f.then(t.Key)
	}
	return source.Result{}, nil
}

func (f *fakeUninstaller) ran() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.runs)
}

func TestUninstallFromTheInstalledList(t *testing.T) {
	e := newEnv(t)
	saved, err := e.shelf.SaveApp(catalog.App{Name: "Obsidian", Source: catalog.Source{Type: catalog.Winget, ID: "Obsidian.Obsidian"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.installed.PreviewUninstall(context.Background(), []string{"winget:no.such"}); err == nil {
		t.Fatal("a key not in the list was accepted")
	}
	keys := []string{"winget:obsidian.obsidian", `arp:machine\x64\tool`}
	prev, err := e.installed.PreviewUninstall(context.Background(), keys)
	if err != nil || len(prev) != 2 {
		t.Fatalf("preview = %+v, %v", prev, err)
	}
	obs, tool := prev[0], prev[1]
	if !obs.Runnable || obs.Plan != "卸载" || obs.Method != "winget" || !strings.Contains(obs.Command, "winget uninstall --id Obsidian.Obsidian --exact --source winget") || !slices.Equal(obs.InShelf, []string{saved.ID}) {
		t.Errorf("winget preview = %+v", obs)
	}
	// Tool's registry key has no uninstall command.
	if tool.Runnable || !strings.Contains(tool.Plan, "注册表里没有卸载命令") || tool.Method != "卸载程序" {
		t.Errorf("registry preview = %+v", tool)
	}

	if _, err := e.tasks.Start(context.Background(), []string{saved.ID}, ActionUninstall); err == nil {
		t.Fatal("Tasks.Start must not take a removal")
	}

	n, err := e.installed.StartUninstall(context.Background(), keys)
	if err != nil || n != 1 {
		t.Fatalf("start = %d, %v", n, err)
	}
	e.until(func() bool {
		l := e.tasks.List()
		return len(l.Items) == 1 && l.Items[0].State == TaskSucceeded && !l.Running
	})
	task := e.tasks.List().Items[0]
	if task.Action != ActionUninstall || task.Name != "Obsidian" || !task.HasLog || task.Interactive {
		t.Errorf("task = %+v", task)
	}
	lg, _ := e.tasks.Log(task.ID, 0)
	if len(lg.Lines) < 2 || !strings.HasPrefix(lg.Lines[0], "AppShelf 卸载「Obsidian」") || !slices.Contains(lg.Lines, "[AppShelf] 之后的状态：已不在本机") {
		t.Errorf("log = %q", lg.Lines)
	}
	// The catalog keeps the app.
	if _, ok := e.core.Catalog().App(saved.ID); !ok {
		t.Error("removing a program took its app out of the catalog")
	}
	if got := e.uninst.ran(); !slices.Equal(got, []string{"winget:obsidian.obsidian|q"}) {
		t.Errorf("runs = %q", got)
	}

	// Only what can run is queued; nothing that can run is an error.
	if _, err := e.installed.StartUninstall(context.Background(), []string{`arp:machine\x64\tool`}); err == nil || !strings.Contains(err.Error(), "注册表里没有卸载命令") {
		t.Errorf("start of a blocked program: %v", err)
	}
}

// listed reports whether the installed list has key.
func (e *env) listed(key string) bool {
	return slices.ContainsFunc(e.installed.List().Entries, func(en InstalledEntry) bool { return en.Key == key })
}

func TestUninstallTakesTheProgramOffTheListAtOnce(t *testing.T) {
	e := newEnv(t)
	key := "winget:obsidian.obsidian"
	if !e.listed(key) {
		t.Fatal("the program is not listed to begin with")
	}

	// A reading begun before the removal is under way, and held.
	stale := make(chan struct{})
	e.wapi.mu.Lock()
	e.wapi.listBlock = stale
	before := e.wapi.lists
	e.wapi.mu.Unlock()
	e.installed.Refresh()
	e.until(func() bool { e.wapi.mu.Lock(); defer e.wapi.mu.Unlock(); return e.wapi.lists == before+1 })

	if _, err := e.installed.StartUninstall(context.Background(), []string{key}); err != nil {
		t.Fatal(err)
	}
	e.until(func() bool {
		l := e.tasks.List()
		return len(l.Items) == 1 && l.Items[0].State == TaskSucceeded
	})
	// Gone as the task ends, while the list is still being read.
	e.until(func() bool { return !e.listed(key) })
	if !e.installed.List().Loading {
		t.Fatal("the reading held has ended")
	}

	// The stale reading still lists it: it stays off. The reading asked
	// again once the queue ended begins after the removal, and is held.
	fresh := make(chan struct{})
	e.wapi.mu.Lock()
	e.wapi.listBlock = fresh
	e.wapi.mu.Unlock()
	close(stale)
	e.until(func() bool { e.wapi.mu.Lock(); defer e.wapi.mu.Unlock(); return e.wapi.lists == before+2 })
	if e.listed(key) {
		t.Fatal("a reading begun before the removal brought the program back")
	}

	// The fresh reading lists what is left, and the removal is forgotten.
	e.wapi.mu.Lock()
	e.wapi.pkgs = slices.DeleteFunc(slices.Clone(e.wapi.pkgs), func(p winget.Package) bool { return p.ID == "Obsidian.Obsidian" })
	e.wapi.mu.Unlock()
	close(fresh)
	e.until(func() bool { return !e.installed.List().Loading })
	if e.listed(key) || !e.listed(`arp:machine\x64\tool`) {
		t.Errorf("list after reading again = %+v", e.installed.List().Entries)
	}
	e.app.inv.mu.Lock()
	gone := len(e.app.inv.gone)
	e.app.inv.mu.Unlock()
	if gone != 0 {
		t.Errorf("%d removals still kept after a reading begun since", gone)
	}
}

// A removal skipped because its program is gone takes it off the list at
// once too, not once the list is read again.
func TestSkippedUninstallTakesTheProgramOffTheListAtOnce(t *testing.T) {
	e := newEnv(t)
	key := "winget:obsidian.obsidian"
	if !e.listed(key) {
		t.Fatal("the program is not listed to begin with")
	}
	e.uninst.mu.Lock()
	e.uninst.gone[key] = true
	e.uninst.mu.Unlock()
	if _, err := e.installed.StartUninstall(context.Background(), []string{key}); err != nil {
		t.Fatal(err)
	}
	// The reading asked once the queue ends is held.
	hold := make(chan struct{})
	defer close(hold)
	e.wapi.mu.Lock()
	e.wapi.listBlock = hold
	e.wapi.mu.Unlock()
	e.until(func() bool {
		l := e.tasks.List()
		return len(l.Items) == 1 && l.Items[0].State == TaskSkipped
	})
	e.until(func() bool { return !e.listed(key) })
	if r := e.uninst.ran(); len(r) != 0 {
		t.Errorf("ran %q: a program gone must not run", r)
	}
	// The task shows the icon the installed list showed.
	if icon := e.tasks.List().Items[0].Icon; icon != "/i/"+url.PathEscape(key) {
		t.Errorf("task icon = %q", icon)
	}
}

func TestUninstallFailedRetriesInteractively(t *testing.T) {
	e := newEnv(t)
	key := "winget:obsidian.obsidian"
	e.uninst.keep[key] = true
	if _, err := e.installed.StartUninstall(context.Background(), []string{key}); err != nil {
		t.Fatal(err)
	}
	e.until(func() bool {
		l := e.tasks.List()
		return len(l.Items) == 1 && l.Items[0].State == TaskFailed && !l.Running
	})
	task := e.tasks.List().Items[0]
	if !task.Interactive || !strings.Contains(task.Message, "退出码 2") {
		t.Fatalf("failed task = %+v", task)
	}
	e.uninst.mu.Lock()
	e.uninst.keep[key] = false
	e.uninst.mu.Unlock()
	if err := e.tasks.RetryInteractive(task.ID); err != nil {
		t.Fatal(err)
	}
	e.until(func() bool {
		l := e.tasks.List()
		return len(l.Items) == 1 && l.Items[0].State == TaskSucceeded && !l.Running
	})
	if got := e.uninst.ran(); !slices.Equal(got, []string{key + "|q", key + "|i"}) {
		t.Fatalf("runs = %q", got)
	}
	if err := e.tasks.RetryInteractive(task.ID); err == nil {
		t.Error("a task that succeeded retried")
	}
}

// Programs removed quietly go first, those opening their uninstaller's
// window last.
func TestUninstallQuietFirst(t *testing.T) {
	e := newEnv(t)
	exe := filepath.Join(t.TempDir(), "remove.exe")
	os.WriteFile(exe, []byte("MZ"), 0o644)
	e.mu.Lock()
	e.registry = []detect.Entry{{DisplayName: "Windowed", Name: "Windowed", Scope: "Machine", Arch: "X64", Key: `HKLM\Windowed`, UninstallString: `"` + exe + `"`}}
	e.mu.Unlock()
	e.listARP(`ARP\Machine\X64\Windowed`, "Windowed")
	e.installed.Refresh()
	key := `arp:machine\x64\windowed`
	e.until(func() bool { return e.listed(key) })
	if _, err := e.installed.StartUninstall(context.Background(), []string{key, "winget:obsidian.obsidian"}); err != nil {
		t.Fatal(err)
	}
	e.until(func() bool { return !e.tasks.List().Running && len(e.uninst.ran()) == 2 })
	if got := e.uninst.ran(); !slices.Equal(got, []string{"winget:obsidian.obsidian|q", key + "|q"}) {
		t.Errorf("runs = %q", got)
	}
}

// A removal waiting for the user shows it on its task, tells once with a
// notification while the window is away, and takes the user's commands.
func TestUninstallWaitingForTheUser(t *testing.T) {
	e := newEnv(t)
	key := "winget:obsidian.obsidian"
	e.uninst.hold = make(chan struct{})
	if _, err := e.installed.StartUninstall(context.Background(), []string{key}); err != nil {
		t.Fatal(err)
	}
	var task TaskEntry
	e.until(func() bool {
		l := e.tasks.List()
		if len(l.Items) == 1 {
			task = l.Items[0]
		}
		return task.Wait == WaitUser
	})
	if task.Prompt != "「卸载」请先关闭程序" || !task.Window || !task.CanSwitch {
		t.Errorf("task = %+v", task)
	}
	told := func() []string {
		e.mu.Lock()
		defer e.mu.Unlock()
		var out []string
		for _, n := range e.notified {
			if strings.HasPrefix(n, "tasks:") {
				out = append(out, n)
			}
		}
		return out
	}
	e.until(func() bool { return len(told()) > 0 })
	e.app.Progress() // again: told once
	notified := told()
	if len(notified) != 1 || notified[0] != "tasks:「Obsidian」的卸载程序在等你操作|「卸载」请先关闭程序" {
		t.Errorf("notified %q", notified)
	}
	if err := e.tasks.Control(task.ID, "nonsense"); err == nil {
		t.Error("an unknown command was taken")
	}
	if err := e.tasks.Control(task.ID+1, ControlStopWaiting); err == nil {
		t.Error("a command reached no task")
	}
	if err := e.tasks.Control(task.ID, ControlShowWindow); err != nil {
		t.Error(err)
	}
	close(e.uninst.hold)
	e.until(func() bool { return !e.tasks.List().Running })
	if err := e.tasks.Control(task.ID, ControlStopWaiting); err == nil {
		t.Error("a finished task took a command")
	}
}
