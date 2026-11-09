package api

import (
	"context"
	"testing"

	"appshelf/internal/source"
)

func TestTrayStatusAndUpdateAll(t *testing.T) {
	e := newEnv(t)
	updateState(e, availableUpdate())
	ctx := context.Background()
	if s := e.app.TrayStatus(); s.Updates != 1 || s.Runnable != 1 || s.Active != 0 || s.Running != "" || s.Checking {
		t.Fatalf("at first: %+v", s)
	}

	// An update held back is neither counted nor updated.
	if err := e.updates.Skip(installedObsidian); err != nil {
		t.Fatal(err)
	}
	if s := e.app.TrayStatus(); s.Updates != 0 || s.Runnable != 0 {
		t.Errorf("skipped: %+v", s)
	}
	if n, err := e.app.UpdateAll(ctx); err == nil || n != 0 {
		t.Errorf("updated all while held: %d %v", n, err)
	}
	if err := e.updates.Restore(installedObsidian); err != nil {
		t.Fatal(err)
	}

	// Updating all queues it once; while it runs, the icon says so.
	block := make(chan struct{})
	e.winget.mu.Lock()
	e.winget.upgradeBlock = block
	e.winget.after[installedObsidian] = source.Status{State: source.Installed, Version: "1.9"}
	e.winget.mu.Unlock()
	n, err := e.app.UpdateAll(ctx)
	if err != nil || n != 1 {
		t.Fatalf("update all: %d %v", n, err)
	}
	e.until(func() bool { return e.app.TrayStatus().Running != "" })
	if s := e.app.TrayStatus(); s.Active != 1 || s.Action != ActionUpgrade || s.Runnable != 0 {
		t.Errorf("running: %+v", s)
	}
	if n, err := e.app.UpdateAll(ctx); err == nil || n != 0 {
		t.Errorf("queued twice: %d %v", n, err)
	}
	close(block)
	e.until(func() bool { s := e.app.TrayStatus(); return s.Active == 0 && s.Updates == 0 })
}
