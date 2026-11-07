package api

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"appshelf/internal/catalog"
	"appshelf/internal/source"
)

const installedObsidian = "winget:obsidian.obsidian"

func updateState(e *env, st source.Status) {
	e.winget.mu.Lock()
	e.winget.state[(catalog.Source{Type: catalog.Winget, ID: "Obsidian.Obsidian"}).Key()] = st
	e.winget.mu.Unlock()
}

func availableUpdate() source.Status {
	return source.Status{State: source.UpdateAvailable, Version: "1.8", Latest: "1.9"}
}

func TestInstalledUpdateDoesNotAddToCatalog(t *testing.T) {
	e := newEnv(t)
	updateState(e, availableUpdate())
	e.winget.mu.Lock()
	e.winget.after[(catalog.Source{Type: catalog.Winget, ID: "Obsidian.Obsidian"}).Key()] = source.Status{State: source.Installed, Version: "1.9"}
	e.winget.mu.Unlock()
	preview, err := e.installed.PreviewUpdates(context.Background(), []string{installedObsidian, installedObsidian})
	if err != nil || len(preview) != 1 || !preview[0].Runnable || preview[0].Source.ID != "Obsidian.Obsidian" {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	n, err := e.installed.StartUpdates(context.Background(), []string{installedObsidian, installedObsidian})
	if err != nil || n != 1 {
		t.Fatalf("start: %d %v", n, err)
	}
	e.until(func() bool {
		items := e.tasks.List()
		return len(items.Items) == 1 && items.Items[0].State == TaskSucceeded
	})
	if len(e.shelf.Snapshot().Apps) != 0 {
		t.Fatal("direct update added an app to the catalog")
	}
	for _, name := range []string{"catalog.json", "state.json"} {
		if _, err := os.Stat(filepath.Join(e.dir, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("direct update wrote %s", name)
		}
	}
	e.winget.mu.Lock()
	defer e.winget.mu.Unlock()
	if len(e.winget.actions) != 1 || !strings.HasPrefix(e.winget.actions[0], "upgrade:") {
		t.Fatalf("unexpected install or duplicate action: %v", e.winget.actions)
	}
}

func TestInstalledUpdateRejectsUnknownKeysAndLocalPrograms(t *testing.T) {
	e := newEnv(t)
	list := e.installed.List()
	var localKey string
	for _, entry := range list.Entries {
		if entry.Kind == "local" {
			localKey = entry.Key
		}
	}
	for _, key := range []string{"winget:injected.package", localKey} {
		if _, err := e.installed.PreviewUpdates(context.Background(), []string{key}); err == nil {
			t.Errorf("accepted key %q", key)
		}
		if _, err := e.installed.StartUpdates(context.Background(), []string{key}); err == nil {
			t.Errorf("queued key %q", key)
		}
	}
	if len(e.tasks.List().Items) != 0 {
		t.Fatal("invalid source queued")
	}
}

func TestInstalledUpdateSkipsUnsafeStates(t *testing.T) {
	for _, st := range []source.Status{
		{State: source.NotInstalled},
		{State: source.Unknown, Note: "source failed"},
		{State: source.Installed, Version: "1.9"},
		{State: source.UpdateAvailable, External: true, Version: "1.8", Latest: "1.9"},
	} {
		t.Run(st.State.String()+st.Note, func(t *testing.T) {
			e := newEnv(t)
			updateState(e, st)
			p, err := e.installed.PreviewUpdates(context.Background(), []string{installedObsidian})
			if err != nil || len(p) != 1 || p[0].Runnable {
				t.Fatalf("unsafe preview: %+v %v", p, err)
			}
			if _, err := e.installed.StartUpdates(context.Background(), []string{installedObsidian}); err == nil {
				t.Fatal("unsafe state queued")
			}
			if len(e.tasks.List().Items) != 0 {
				t.Fatal("no-op update queued")
			}
		})
	}
}

func TestInstalledUpdateRechecksAfterPreviewAndHonorsCancel(t *testing.T) {
	e := newEnv(t)
	updateState(e, availableUpdate())
	p, err := e.installed.PreviewUpdates(context.Background(), []string{installedObsidian})
	if err != nil || !p[0].Runnable {
		t.Fatal("initial preview failed")
	}
	updateState(e, source.Status{State: source.NotInstalled})
	if _, err := e.installed.StartUpdates(context.Background(), []string{installedObsidian}); err == nil {
		t.Fatal("stale preview caused an install")
	}
	updateState(e, availableUpdate())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.installed.StartUpdates(ctx, []string{installedObsidian}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	if len(e.tasks.List().Items) != 0 {
		t.Fatal("canceled update queued")
	}
}

func TestInstalledUpdateRetryUsesUpgrade(t *testing.T) {
	e := newEnv(t)
	updateState(e, availableUpdate())
	e.winget.mu.Lock()
	e.winget.upgradeErrors = 1
	e.winget.after[(catalog.Source{Type: catalog.Winget, ID: "Obsidian.Obsidian"}).Key()] = source.Status{State: source.Installed, Version: "1.9"}
	e.winget.mu.Unlock()
	if _, err := e.installed.StartUpdates(context.Background(), []string{installedObsidian}); err != nil {
		t.Fatal(err)
	}
	e.until(func() bool { items := e.tasks.List().Items; return len(items) == 1 && items[0].State == TaskFailed })
	id := e.tasks.List().Items[0].ID
	if err := e.tasks.Retry(id); err != nil {
		t.Fatal(err)
	}
	e.until(func() bool { return e.tasks.List().Items[0].State == TaskSucceeded })
	e.winget.mu.Lock()
	defer e.winget.mu.Unlock()
	if len(e.winget.actions) != 2 {
		t.Fatalf("retry count: %v", e.winget.actions)
	}
	for _, a := range e.winget.actions {
		if !strings.HasPrefix(a, "upgrade:") {
			t.Fatalf("retry installed: %s", a)
		}
	}
}

func TestInstalledUpdateReusesCatalogIdentity(t *testing.T) {
	e := newEnv(t)
	updateState(e, availableUpdate())
	saved, err := e.shelf.SaveApp(catalog.App{Name: "Obsidian", Source: catalog.Source{Type: catalog.Winget, ID: "Obsidian.Obsidian"}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := e.installed.PreviewUpdates(context.Background(), []string{installedObsidian})
	if err != nil || len(p) != 1 || p[0].AppID != saved.ID {
		t.Fatalf("catalog identity not reused: %+v %v", p, err)
	}
}
