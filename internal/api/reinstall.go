package api

import (
	"context"
	"errors"
	"strings"

	"appshelf/internal/catalog"
	"appshelf/internal/inventory"
	"appshelf/internal/queue"
)

// An update its installer cannot do in place, as winget's 0x8A150114, can
// be done by removing the program and installing the same package again.
// Nothing runs without the user: the removal is previewed and confirmed
// as any other, and the install, of the update's exact package, runs only
// once the removal is done.

// canReinstall reports a failed update its installer could not do, of a
// winget package not in a task meanwhile.
func (a *App) canReinstall(it queue.Item) bool {
	q := a.core.Queue
	return it.State == queue.Failed && it.Action == queue.Upgrade && it.NoDirectUpgrade &&
		it.App.Source.Type == catalog.Winget && !q.Pending(it.App.ID) && !q.PendingSource(it.App.Source)
}

// reinstallTask returns the failed update id, when it can be done again by
// removing its program first.
func (a *App) reinstallTask(id int) (queue.Item, error) {
	for _, it := range a.core.Queue.Items() {
		if it.ID != id {
			continue
		}
		if !a.canReinstall(it) {
			return queue.Item{}, errors.New("这个任务不能卸载后重新安装")
		}
		return it, nil
	}
	return queue.Item{}, errors.New("找不到这个任务")
}

// reinstallKey finds, in the installed list read again, the one program of
// the update's exact winget package. More than one, or none, is not
// guessed at.
func (a *App) reinstallKey(ctx context.Context, app catalog.App) (string, error) {
	list, err := a.inv.fresh(ctx)
	if err != nil {
		return "", err
	}
	var keys []string
	for _, e := range list.Entries {
		if e.Kind == inventory.Winget && strings.EqualFold(e.WingetID, app.Source.ID) {
			keys = append(keys, e.Key)
		}
	}
	switch len(keys) {
	case 0:
		return "", errors.New("本机列表里没有找到它，请重新读取后再试")
	case 1:
		return keys[0], nil
	}
	return "", errors.New("本机有多个程序对应这个软件包，无法确定卸载哪一个")
}

// PreviewReinstall says what removing the program of a failed update its
// installer could not do would do, before it is installed again. Nothing
// runs.
func (t *Tasks) PreviewReinstall(ctx context.Context, id int) ([]UninstallPreview, error) {
	a := t.a
	<-a.ready
	it, err := a.reinstallTask(id)
	if err != nil {
		return nil, err
	}
	key, err := a.reinstallKey(ctx, it.App)
	if err != nil {
		return nil, err
	}
	return (&Installed{a}).PreviewUninstall(ctx, []string{key})
}

// StartReinstall queues the removal of the program key, as the preview
// showed it, then the install of the failed update's package once it is
// gone. key must still be the one program of that package.
func (t *Tasks) StartReinstall(ctx context.Context, id int, key string) error {
	a := t.a
	<-a.ready
	it, err := a.reinstallTask(id)
	if err != nil {
		return err
	}
	found, err := a.reinstallKey(ctx, it.App)
	if err != nil {
		return err
	}
	if found != key {
		return errors.New("本机列表已经变化，请重新打开预览")
	}
	targets, _, err := (&Installed{a}).uninstallTargets(ctx, []string{key})
	if err != nil {
		return err
	}
	p := a.core.PreviewUninstall(ctx, targets)[0]
	if err := ctx.Err(); err != nil {
		return err
	}
	if !p.Runnable {
		return errors.New(p.Plan)
	}
	ok := false
	a.write([]ChangeKind{ChangedTasks}, func() { ok = a.core.Queue.AddReinstall(it.ID, p.Target) })
	if !ok {
		return errors.New("已经开始卸载后重新安装，或它已在任务中")
	}
	return nil
}
