package api

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"appshelf/internal/leftover"
	"appshelf/internal/queue"
	"appshelf/internal/uninstall"
)

// leftoverScan is what FindLeftovers found, kept for CleanLeftovers: the
// page chooses among its finds by ID, never by path.
type leftoverScan struct {
	name   string
	cfg    leftover.Config
	prog   leftover.Program
	others []leftover.Other
	items  []leftover.Item
	// backup is where CleanLeftovers saved the registry, once it ran.
	backup string
}

// canFindLeftovers reports a task whose leftovers can be looked for: a
// removal that succeeded.
func canFindLeftovers(it queue.Item) bool {
	return it.Action == queue.Uninstall && it.State == queue.Succeeded && it.Target != nil
}

// FindLeftovers looks for what a program removed by a task left behind:
// its directories, settings, shortcuts and startup entries, each scored
// with its reasons. Nothing is cleaned.
func (t *Tasks) FindLeftovers(ctx context.Context, id int) (LeftoverScan, error) {
	a := t.a
	<-a.ready
	var target *uninstall.Target
	for _, it := range a.core.Queue.Items() {
		if it.ID == id {
			if !canFindLeftovers(it) {
				return LeftoverScan{}, errors.New("只能为卸载成功的软件查找残留")
			}
			target = it.Target
		}
	}
	if target == nil {
		return LeftoverScan{}, errors.New("找不到这个任务")
	}
	scan := &leftoverScan{
		name: target.Name,
		cfg:  leftover.DefaultConfig(a.cfg.Protect...),
		prog: leftover.Program{Name: target.Name, Publisher: target.Publisher, Location: target.Location, ScoopName: target.ScoopName},
	}
	list, _, _ := a.inv.get()
	for _, e := range list.Entries {
		// The list may still have the program removed.
		if e.Key == target.Key || target.Location != "" && strings.EqualFold(filepath.Clean(e.Location), filepath.Clean(target.Location)) {
			continue
		}
		scan.others = append(scan.others, leftover.Other{Name: e.Name, Publisher: e.Publisher, Location: e.Location})
	}
	scan.items = leftover.Find(ctx, scan.cfg, scan.prog, scan.others)
	if err := ctx.Err(); err != nil {
		return LeftoverScan{}, err
	}
	token := a.remember(func(token string) { a.scans[token] = scan })
	out := LeftoverScan{Token: token, Name: scan.name, Items: make([]LeftoverItem, 0, len(scan.items))}
	for _, it := range scan.items {
		out.Items = append(out.Items, leftoverItemOf(it))
	}
	return out, nil
}

func leftoverItemOf(it leftover.Item) LeftoverItem {
	path := it.Path
	if it.Kind == leftover.RegKey || it.Kind == leftover.RegValue {
		path = `HKEY_CURRENT_USER\` + path
	}
	out := LeftoverItem{
		ID: it.ID, Kind: LeftoverKind(it.Kind), Path: path, Value: it.Value,
		Score: it.Score, Level: LeftoverLevel(it.Level), Checked: it.Level == leftover.High,
		Reasons: make([]LeftoverReason, 0, len(it.Reasons)),
	}
	for _, r := range it.Reasons {
		out.Reasons = append(out.Reasons, LeftoverReason{Text: r.Text, Delta: r.Delta})
	}
	return out
}

func (a *App) scan(token string) (*leftoverScan, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.scans[token]
	if !ok {
		return nil, errors.New("这次查找已过期，请重新查找残留")
	}
	return s, nil
}

// CleanLeftovers cleans the finds chosen of a FindLeftovers: files and
// directories go to the Recycle Bin, registry keys and values are exported
// to the backups first. Each is checked again before it is cleaned.
func (t *Tasks) CleanLeftovers(ctx context.Context, token string, ids []string) (LeftoverResult, error) {
	a := t.a
	<-a.ready
	scan, err := a.scan(token)
	if err != nil {
		return LeftoverResult{}, err
	}
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	var chosen []leftover.Item
	for _, it := range scan.items {
		if want[it.ID] {
			chosen = append(chosen, it)
		}
	}
	if len(chosen) == 0 {
		return LeftoverResult{}, errors.New("没有选择要清理的残留")
	}
	backup := filepath.Join(a.cfg.DataDir, "backups", "leftovers-"+time.Now().Format("20060102-150405")+"-"+safeName(scan.name))
	outcomes := leftover.Clean(ctx, scan.cfg, chosen, scan.others, scan.prog.Location, backup)
	res := LeftoverResult{Outcomes: make([]LeftoverOutcome, 0, len(outcomes))}
	for _, o := range outcomes {
		res.Outcomes = append(res.Outcomes, LeftoverOutcome{ID: o.ID, OK: o.OK, Error: o.Error})
		if o.OK {
			res.Cleaned++
		} else {
			res.Failed++
		}
		if o.Backup != "" {
			res.BackupDir = backup
		}
	}
	if res.BackupDir != "" {
		a.mu.Lock()
		scan.backup = res.BackupDir
		a.mu.Unlock()
	}
	a.inv.refresh(0)
	return res, nil
}

// OpenBackup opens the directory CleanLeftovers saved the registry to.
func (t *Tasks) OpenBackup(token string) error {
	a := t.a
	scan, err := a.scan(token)
	if err != nil {
		return err
	}
	a.mu.Lock()
	dir := scan.backup
	a.mu.Unlock()
	if dir == "" {
		return errors.New("这次清理没有备份注册表")
	}
	return a.openPath(dir)
}

// safeName makes a program's name fit in a directory's name.
func safeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < 0x20 || strings.ContainsRune(`<>:"/\|?*`, r) {
			r = '_'
		}
		b.WriteRune(r)
	}
	out := strings.Trim(b.String(), ". ")
	if r := []rune(out); len(r) > 40 {
		out = string(r[:40])
	}
	if out == "" {
		return "program"
	}
	return out
}
