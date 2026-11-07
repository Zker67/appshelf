package api

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"

	"appshelf/internal/catalog"
	"appshelf/internal/core"
	"appshelf/internal/inventory"
	"appshelf/internal/queue"
)

// updateApps resolves only keys already observed by the inventory service.
// The frontend cannot inject source IDs, installer parameters, or new apps.
func (s *Installed) updateApps(keys []string) ([]catalog.App, error) {
	a := s.a
	<-a.ready
	list, loaded, _ := a.inv.get()
	if !loaded {
		return nil, errors.New("本机列表正在读取，请稍后重试")
	}
	byKey := make(map[string]inventory.Entry, len(list.Entries))
	for _, entry := range list.Entries {
		byKey[entry.Key] = entry
	}
	saved := map[string]catalog.App{}
	a.read(func() {
		for _, app := range a.core.Catalog().Apps {
			saved[app.Source.Key()] = app.Clone()
		}
	})
	seen := map[string]bool{}
	var apps []catalog.App
	for _, key := range keys {
		entry, ok := byKey[key]
		if !ok {
			return nil, errors.New("软件已不在当前本机列表中，请重新读取")
		}
		var src catalog.Source
		switch entry.Kind {
		case inventory.Winget:
			src = catalog.Source{Type: catalog.Winget, ID: entry.WingetID}
		case inventory.Scoop:
			src = catalog.Source{Type: catalog.Scoop, Bucket: entry.Bucket, Name: entry.ScoopName}
		default:
			return nil, fmt.Errorf("「%s」没有可直接更新的 winget 或 scoop 来源", entry.Name)
		}
		if seen[src.Key()] {
			continue
		}
		seen[src.Key()] = true
		app, ok := saved[src.Key()]
		if !ok {
			sum := sha256.Sum256([]byte(src.Key()))
			app = catalog.App{ID: fmt.Sprintf("installed_%x", sum[:16]), Name: entry.Name, Source: src}
		}
		if err := catalog.ValidateApp(app); err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}
	return apps, nil
}

// PreviewUpdates checks current source state without changing the catalog.
func (s *Installed) PreviewUpdates(ctx context.Context, keys []string) ([]PreviewEntry, error) {
	apps, err := s.updateApps(keys)
	if err != nil {
		return nil, err
	}
	return updateEntries(s.a.core.PreviewUpdates(ctx, apps)), nil
}

func updateEntries(items []core.PreviewItem) []PreviewEntry {
	out := make([]PreviewEntry, 0, len(items))
	for _, item := range items {
		out = append(out, PreviewEntry{AppID: item.App.ID, Name: item.App.Name, Icon: iconOf(item.App.Source), Runnable: item.Runnable, Plan: item.Plan, Source: item.App.Source, Note: item.Status.Note})
	}
	return out
}

// StartUpdates resolves and checks the targets again, then uses only Upgrade.
// The shared queue performs its own pre/post checks before changing software.
func (s *Installed) StartUpdates(ctx context.Context, keys []string) (int, error) {
	apps, err := s.updateApps(keys)
	if err != nil {
		return 0, err
	}
	items := s.a.core.PreviewUpdates(ctx, apps)
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	n := 0
	s.a.write([]ChangeKind{ChangedTasks, ChangedShelf}, func() { n = s.a.core.Enqueue(items, queue.Upgrade) })
	if n == 0 {
		return 0, errors.New("没有可以执行的更新，请重新读取本机状态")
	}
	return n, nil
}
