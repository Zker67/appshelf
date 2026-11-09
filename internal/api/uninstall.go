package api

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"appshelf/internal/catalog"
	"appshelf/internal/core"
	"appshelf/internal/inventory"
	"appshelf/internal/queue"
	"appshelf/internal/uninstall"
)

// uninstallTargets resolves keys of the installed list as Go read it: the
// page cannot name a program, a command or a path of its own. The list
// kept by an earlier run is not used: its commands may be stale.
func (s *Installed) uninstallTargets(ctx context.Context, keys []string) ([]uninstall.Target, []inventory.Entry, error) {
	a := s.a
	<-a.ready
	list, err := a.inv.fresh(ctx)
	if err != nil {
		return nil, nil, err
	}
	byKey := make(map[string]inventory.Entry, len(list.Entries))
	for _, e := range list.Entries {
		byKey[e.Key] = e
	}
	var apps []catalog.App
	a.read(func() { apps = a.core.Catalog().Apps })
	match := inventory.Match(list.Entries, apps)
	seen := map[string]bool{}
	var targets []uninstall.Target
	var entries []inventory.Entry
	for _, key := range keys {
		e, ok := byKey[key]
		if !ok {
			return nil, nil, errors.New("软件已不在当前本机列表中，请重新读取")
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		targets = append(targets, targetOf(e, match[e.Key]))
		entries = append(entries, e)
	}
	if len(targets) == 0 {
		return nil, nil, errors.New("没有选择要卸载的软件")
	}
	return targets, entries, nil
}

// targetOf is how a program of the installed list is removed: through the
// package manager that knows it, else by its registry key.
func targetOf(e inventory.Entry, inShelf []string) uninstall.Target {
	t := uninstall.Target{
		Key: e.Key, Name: e.Name, Version: e.Version, Publisher: e.Publisher,
		Location: e.Location, Scope: e.Scope, Reg: e.Reg, CatalogIDs: inShelf,
	}
	switch {
	case e.Kind == inventory.Winget:
		t.Method, t.WingetID, t.WingetSource = uninstall.ByWinget, e.WingetID, "winget"
	case e.Kind == inventory.Scoop:
		t.Method, t.ScoopName, t.Global = uninstall.ByScoop, e.ScoopName, e.Scope == "Machine"
	case e.Kind == inventory.Store && e.PackageFullName != "":
		t.Method, t.PackageFullName = uninstall.ByStore, e.PackageFullName
	case e.Kind == inventory.Store && e.StoreID != "":
		t.Method, t.WingetID, t.WingetSource = uninstall.ByWinget, e.StoreID, "msstore"
	case e.Reg != nil:
		t.Method = uninstall.ByRegistry
	}
	return t
}

// PreviewUninstall says what removing the programs would do: how, with
// what command, and what to know first. Nothing runs.
func (s *Installed) PreviewUninstall(ctx context.Context, keys []string) ([]UninstallPreview, error) {
	targets, entries, err := s.uninstallTargets(ctx, keys)
	if err != nil {
		return nil, err
	}
	items := s.a.core.PreviewUninstall(ctx, targets)
	out := make([]UninstallPreview, len(items))
	for i, it := range items {
		out[i] = uninstallPreviewOf(it, entries[i])
	}
	return out, nil
}

func uninstallPreviewOf(it core.UninstallPreview, e inventory.Entry) UninstallPreview {
	t := it.Target
	p := UninstallPreview{
		Key: t.Key, Name: t.Name, Version: t.Version, Publisher: t.Publisher,
		Icon: entryOf(e, nil).Icon, Kind: InstalledKind(e.Kind),
		Runnable: it.Runnable, Plan: it.Plan, Method: methodText(t.Method), Command: it.Command,
		Quiet: it.Quiet, Notes: it.Notes, Location: t.Location, Size: e.Size,
		Signer: it.Signer, Signed: it.Signed, InShelf: t.CatalogIDs,
	}
	if t.Method == uninstall.ByRegistry && it.Kind != "" {
		p.Installer = uninstall.KindText(it.Kind)
	}
	if p.Notes == nil {
		p.Notes = []string{}
	}
	if p.InShelf == nil {
		p.InShelf = []string{}
	}
	return p
}

func methodText(m uninstall.Method) string {
	switch m {
	case uninstall.ByWinget:
		return "winget"
	case uninstall.ByScoop:
		return "scoop"
	case uninstall.ByStore:
		return "应用商店"
	case uninstall.ByRegistry:
		return "卸载程序"
	}
	return "无"
}

// StartUninstall checks the programs again and queues those that can be
// removed. Each is checked once more right before it runs, and counts as
// removed only when it is then gone. The catalog keeps its apps.
func (s *Installed) StartUninstall(ctx context.Context, keys []string) (int, error) {
	targets, _, err := s.uninstallTargets(ctx, keys)
	if err != nil {
		return 0, err
	}
	items := s.a.core.PreviewUninstall(ctx, targets)
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	var blocked []string
	for _, it := range items {
		if !it.Runnable {
			blocked = append(blocked, fmt.Sprintf("「%s」%s", it.Target.Name, strings.TrimPrefix(it.Plan, "不能卸载：")))
		}
	}
	n := 0
	s.a.write([]ChangeKind{ChangedTasks}, func() { n = s.a.core.EnqueueUninstall(items) })
	if n == 0 {
		if len(blocked) > 0 {
			return 0, errors.New("没有可以卸载的软件：" + strings.Join(blocked, "；"))
		}
		return 0, errors.New("没有可以卸载的软件")
	}
	return n, nil
}

// interactiveRetry reports a removal that can run again with its
// uninstaller's window: by winget, or by an uninstall command that ran
// quietly.
func interactiveRetry(it queue.Item) bool {
	if it.Action != queue.Uninstall || it.Target == nil || it.Interactive {
		return false
	}
	return uninstall.PlanFor(*it.Target, false, uninstall.Options{}).Interactive
}
