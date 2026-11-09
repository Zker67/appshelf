package api

import (
	"context"
	"encoding/json"
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
	if _, err := os.Stat(filepath.Join(e.dir, "catalog.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("direct update wrote catalog.json")
	}
	// The state records the update in the history, and nothing else.
	e.until(func() bool { return len(e.core.Store().History()) == 1 })
	data, err := os.ReadFile(filepath.Join(e.dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var st struct {
		GitHub  map[string]any `json:"github"`
		Updates map[string]any `json:"updates"`
		History []struct {
			Key, From, To, Result string
		} `json:"history"`
	}
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatal(err)
	}
	if len(st.GitHub) != 0 || len(st.Updates) != 0 || len(st.History) != 1 ||
		st.History[0].Key != installedObsidian || st.History[0].From != "1.8" || st.History[0].To != "1.9" || st.History[0].Result != "succeeded" {
		t.Fatalf("state after a direct update: %s", data)
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
			if _, err := e.installed.StartUpdates(context.Background(), []string{installedObsidian}); err == nil {
				t.Fatal("unsafe state queued")
			}
			if len(e.tasks.List().Items) != 0 {
				t.Fatal("no-op update queued")
			}
		})
	}
}

func TestInstalledUpdateRechecksAndHonorsCancel(t *testing.T) {
	e := newEnv(t)
	// The list said it had an update; it is gone since.
	updateState(e, source.Status{State: source.NotInstalled})
	if _, err := e.installed.StartUpdates(context.Background(), []string{installedObsidian}); err == nil {
		t.Fatal("a stale state caused an install")
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
	if _, err := e.installed.StartUpdates(context.Background(), []string{installedObsidian}); err != nil {
		t.Fatal(err)
	}
	if items := e.tasks.List().Items; len(items) != 1 || items[0].AppID != saved.ID {
		t.Fatalf("catalog identity not reused: %+v", items)
	}
}

// installedEntry returns the installed list's entry of key.
func installedEntry(t *testing.T, e *env, key string) InstalledEntry {
	t.Helper()
	for _, en := range e.installed.List().Entries {
		if en.Key == key {
			return en
		}
	}
	t.Fatalf("%s is not listed", key)
	return InstalledEntry{}
}

// A package updated leaves the updates as its task ends, not once the
// queue ends and the list is read again; a reading begun before the
// update does not bring it back.
func TestUpdateTakesThePackageOffTheUpdatesAtOnce(t *testing.T) {
	e := newEnv(t)
	updateState(e, availableUpdate())
	e.winget.mu.Lock()
	e.winget.after[(catalog.Source{Type: catalog.Winget, ID: "Obsidian.Obsidian"}).Key()] = source.Status{State: source.Installed, Version: "1.9"}
	e.winget.mu.Unlock()
	if l := e.updates.List(); len(l.Items) != 1 {
		t.Fatalf("no update to begin with: %+v", l.Items)
	}

	// A reading begun before the update is under way, and held.
	stale := make(chan struct{})
	e.wapi.mu.Lock()
	e.wapi.listBlock = stale
	before := e.wapi.lists
	e.wapi.mu.Unlock()
	e.installed.Refresh()
	e.until(func() bool { e.wapi.mu.Lock(); defer e.wapi.mu.Unlock(); return e.wapi.lists == before+1 })

	if _, err := e.installed.StartUpdates(context.Background(), []string{installedObsidian}); err != nil {
		t.Fatal(err)
	}
	e.until(func() bool {
		l := e.tasks.List()
		return len(l.Items) == 1 && l.Items[0].State == TaskSucceeded
	})
	// Up to date as the task ends, while the list is still being read.
	e.until(func() bool { return len(e.updates.List().Items) == 0 })
	if en := installedEntry(t, e, installedObsidian); en.Version != "1.9" || en.Update != "" {
		t.Errorf("entry after the update = %+v", en)
	}
	if !e.installed.List().Loading {
		t.Fatal("the reading held has ended")
	}

	// The stale reading still has the update: it stays gone. The reading
	// asked again once the queue ended begins after the update, and is
	// held.
	fresh := make(chan struct{})
	e.wapi.mu.Lock()
	e.wapi.listBlock = fresh
	e.wapi.mu.Unlock()
	close(stale)
	e.until(func() bool { e.wapi.mu.Lock(); defer e.wapi.mu.Unlock(); return e.wapi.lists == before+2 })
	if l := e.updates.List(); len(l.Items) != 0 {
		t.Fatalf("a reading begun before the update brought it back: %+v", l.Items)
	}

	// The fresh reading reads it as it is, and the update is forgotten.
	e.wapi.mu.Lock()
	e.wapi.pkgs[0].Version, e.wapi.pkgs[0].Update = "1.9", ""
	e.wapi.mu.Unlock()
	close(fresh)
	e.until(func() bool { return !e.installed.List().Loading })
	if en := installedEntry(t, e, installedObsidian); en.Version != "1.9" || en.Update != "" {
		t.Errorf("entry after reading again = %+v", en)
	}
	e.app.inv.mu.Lock()
	kept := len(e.app.inv.updated)
	e.app.inv.mu.Unlock()
	if kept != 0 {
		t.Errorf("%d updates still kept after a reading begun since", kept)
	}
}

// A check begun before a catalog app's update ends does not bring the
// update back with the state it read before.
func TestUpdateOutlivesACheckBegunBefore(t *testing.T) {
	e := newEnv(t)
	updateState(e, availableUpdate())
	e.winget.mu.Lock()
	e.winget.after[(catalog.Source{Type: catalog.Winget, ID: "Obsidian.Obsidian"}).Key()] = source.Status{State: source.Installed, Version: "1.9"}
	e.winget.mu.Unlock()
	if _, err := e.shelf.SaveApp(catalog.App{Name: "Obsidian", Source: catalog.Source{Type: catalog.Winget, ID: "Obsidian.Obsidian"}}); err != nil {
		t.Fatal(err)
	}
	e.check()
	if l := e.updates.List(); len(l.Items) != 1 || l.Items[0].AppID == "" {
		t.Fatalf("no catalog update to begin with: %+v", l.Items)
	}

	// A check reads the states of before, and is held.
	block := make(chan struct{})
	e.winget.mu.Lock()
	e.winget.statusBlock = block
	e.winget.mu.Unlock()
	e.updates.Check()
	e.until(func() bool {
		var refreshing bool
		e.app.read(func() { refreshing = e.core.Refreshing() })
		return refreshing
	})

	if _, err := e.updates.Start(context.Background(), []string{installedObsidian}); err != nil {
		t.Fatal(err)
	}
	e.until(func() bool {
		l := e.tasks.List()
		return len(l.Items) == 1 && l.Items[0].State == TaskSucceeded
	})
	e.until(func() bool { return len(e.updates.List().Items) == 0 })

	e.winget.mu.Lock()
	e.winget.statusBlock = nil
	e.winget.mu.Unlock()
	close(block)
	e.until(func() bool { checking, _, _ := e.app.upd.times(); return !checking })
	if l := e.updates.List(); len(l.Items) != 0 {
		t.Fatalf("a check begun before the update brought it back: %+v", l.Items)
	}
}
