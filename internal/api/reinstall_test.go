package api

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"appshelf/internal/catalog"
	"appshelf/internal/source"
)

// An update its installer cannot do in place offers removing the program
// and installing it again: the removal is previewed as any other, and the
// install of the same package runs once it is gone, without adding it to
// the catalog.
func TestReinstallAfterAnUpdateNotDoneInPlace(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	key := (catalog.Source{Type: catalog.Winget, ID: "Obsidian.Obsidian"}).Key()
	updateState(e, availableUpdate())
	e.winget.mu.Lock()
	e.winget.upgradeErrors = 1
	e.winget.upgradeErr = fmt.Errorf("%w（winget 退出码 0x8A150114）", source.ErrNoDirectUpgrade)
	e.winget.after[key] = source.Status{State: source.Installed, Version: "1.9"}
	e.winget.mu.Unlock()
	if _, err := e.installed.StartUpdates(ctx, []string{installedObsidian}); err != nil {
		t.Fatal(err)
	}
	e.until(func() bool { l := e.tasks.List(); return len(l.Items) == 1 && l.Items[0].State == TaskFailed })
	failed := e.tasks.List().Items[0]
	if !failed.Reinstall || !strings.Contains(failed.Message, "不支持直接更新") {
		t.Fatalf("failed update = %+v", failed)
	}

	prev, err := e.tasks.PreviewReinstall(ctx, failed.ID)
	if err != nil || len(prev) != 1 || prev[0].Key != installedObsidian || !prev[0].Runnable || prev[0].Method != "winget" {
		t.Fatalf("preview = %+v, %v", prev, err)
	}
	if err := e.tasks.StartReinstall(ctx, failed.ID, `arp:machine\x64\tool`); err == nil {
		t.Fatal("a program other than the package's was accepted")
	}
	if len(e.tasks.List().Items) != 1 || len(e.uninst.ran()) != 0 {
		t.Fatal("a refused reinstall queued something")
	}

	// The removal takes the package off the machine as winget sees it.
	e.uninst.then = func(string) {
		e.winget.mu.Lock()
		e.winget.state[key] = source.Status{State: source.NotInstalled}
		e.winget.mu.Unlock()
	}
	if err := e.tasks.StartReinstall(ctx, failed.ID, installedObsidian); err != nil {
		t.Fatal(err)
	}
	if err := e.tasks.StartReinstall(ctx, failed.ID, installedObsidian); err == nil {
		t.Fatal("the same update reinstalled twice")
	}
	e.until(func() bool {
		l := e.tasks.List()
		return len(l.Items) == 3 && !l.Running && l.Items[2].State != TaskWaiting && l.Items[2].State != TaskRunning
	})
	items := e.tasks.List().Items
	if items[0].Reinstall {
		t.Error("the failed update still offers to reinstall")
	}
	removal, install := items[1], items[2]
	if removal.Action != ActionUninstall || removal.State != TaskSucceeded || !strings.Contains(removal.Message, "接着重新安装") {
		t.Errorf("removal = %+v", removal)
	}
	if install.Action != ActionInstall || install.State != TaskSucceeded || install.AppID != failed.AppID {
		t.Errorf("install = %+v", install)
	}
	if runs := e.uninst.ran(); !slices.Equal(runs, []string{installedObsidian + "|q"}) {
		t.Errorf("removals = %q", runs)
	}
	e.winget.mu.Lock()
	actions := slices.Clone(e.winget.actions)
	e.winget.mu.Unlock()
	if len(actions) != 2 || !strings.HasPrefix(actions[0], "upgrade:") || actions[1] != "install:"+failed.AppID {
		t.Errorf("source actions = %q", actions)
	}
	if len(e.shelf.Snapshot().Apps) != 0 {
		t.Error("reinstalling added the app to the catalog")
	}
}

// An update that failed otherwise does not offer it.
func TestNoReinstallForOtherFailures(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	updateState(e, availableUpdate())
	e.winget.mu.Lock()
	e.winget.upgradeErrors = 1
	e.winget.mu.Unlock()
	if _, err := e.installed.StartUpdates(ctx, []string{installedObsidian}); err != nil {
		t.Fatal(err)
	}
	e.until(func() bool { l := e.tasks.List(); return len(l.Items) == 1 && l.Items[0].State == TaskFailed })
	failed := e.tasks.List().Items[0]
	if failed.Reinstall {
		t.Fatalf("failed update = %+v", failed)
	}
	if _, err := e.tasks.PreviewReinstall(ctx, failed.ID); err == nil {
		t.Error("PreviewReinstall took an update that failed otherwise")
	}
	if err := e.tasks.StartReinstall(ctx, failed.ID, installedObsidian); err == nil {
		t.Error("StartReinstall took an update that failed otherwise")
	}
	if len(e.uninst.ran()) != 0 {
		t.Error("something was removed")
	}
}
