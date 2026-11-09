package api

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"

	"appshelf/internal/catalog"
	"appshelf/internal/source"
	"appshelf/internal/uninstall"
)

// fakeUninstaller plays removing programs: a program run is gone unless
// kept, and the runs are remembered.
type fakeUninstaller struct {
	mu   sync.Mutex
	gone map[string]bool // by key
	keep map[string]bool // programs that stay after their run
	runs []string        // key and "i" or "q"
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

func (f *fakeUninstaller) Run(_ context.Context, t uninstall.Target, interactive bool, log io.Writer) (source.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	mode := "q"
	if interactive {
		mode = "i"
	}
	f.runs = append(f.runs, t.Key+"|"+mode)
	fmt.Fprintln(log, "removing", t.Name)
	if f.keep[t.Key] {
		return source.Result{}, fmt.Errorf("卸载程序以退出码 2 结束")
	}
	f.gone[t.Key] = true
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

	if _, err := e.tasks.Start([]string{saved.ID}, ActionUninstall); err == nil {
		t.Fatal("Tasks.Start must not take a removal")
	}
	if got := e.tasks.Preview([]string{saved.ID}, ActionUninstall); len(got) != 0 {
		t.Fatalf("Tasks.Preview of a removal = %+v", got)
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
