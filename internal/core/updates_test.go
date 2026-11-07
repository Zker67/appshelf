package core

import (
	"context"
	"os"
	"sync/atomic"
	"testing"

	"appshelf/internal/catalog"
	"appshelf/internal/queue"
	"appshelf/internal/source"
)

type batchUpdateSource struct {
	*fakeSource
	batches  atomic.Int32
	rechecks atomic.Int32
}

func (s *batchUpdateSource) Statuses(ctx context.Context, apps []catalog.App) (map[string]source.Status, error) {
	s.batches.Add(1)
	return s.fakeSource.Statuses(ctx, apps)
}
func (s *batchUpdateSource) Recheck(ctx context.Context, app catalog.App) (source.Status, error) {
	s.rechecks.Add(1)
	return s.fakeSource.Recheck(ctx, app)
}

func TestDirectScoopUpdatesBatchPreviewAndUseSharedQueue(t *testing.T) {
	s := &batchUpdateSource{fakeSource: newFake(catalog.Scoop)}
	h := newHarness(t, s)
	apps := []catalog.App{
		{ID: "installed_one", Name: "One", Source: catalog.Source{Type: catalog.Scoop, Bucket: "main", Name: "one"}},
		{ID: "installed_two", Name: "Two", Source: catalog.Source{Type: catalog.Scoop, Bucket: "extras", Name: "two"}},
	}
	for _, a := range apps {
		s.state[a.Source.Key()] = source.Status{State: source.UpdateAvailable, Version: "1", Latest: "2"}
		s.after[a.Source.Key()] = source.Status{State: source.Installed, Version: "2"}
	}
	preview := h.c.PreviewUpdates(context.Background(), apps)
	if len(preview) != 2 || !preview[0].Runnable || !preview[1].Runnable {
		t.Fatalf("preview: %+v", preview)
	}
	if s.batches.Load() != 1 || s.rechecks.Load() != 0 {
		t.Fatalf("preview did not batch: %d batches %d individual checks", s.batches.Load(), s.rechecks.Load())
	}
	if n := h.c.Enqueue(preview, queue.Upgrade); n != 2 {
		t.Fatalf("queued %d", n)
	}
	h.until(func() bool { running, _ := h.c.Queue.Running(); return !running })
	for _, item := range h.c.Queue.Items() {
		if item.Action != queue.Upgrade || item.State != queue.Succeeded {
			t.Fatalf("unsafe or failed update: %+v", item)
		}
	}
	if s.rechecks.Load() != 4 {
		t.Fatalf("missing pre/post checks: %d", s.rechecks.Load())
	}
	if len(h.c.Catalog().Apps) != 0 {
		t.Fatal("updates changed catalog")
	}
	if _, err := os.Stat(h.c.Paths().Catalog); !os.IsNotExist(err) {
		t.Fatal("updates wrote catalog file")
	}
}
