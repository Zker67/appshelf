package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"appshelf/internal/queue"
	"appshelf/internal/source"
	"appshelf/internal/uninstall"
)

// Uninstaller removes programs and tells whether they are still there:
// uninstall.Runner.
type Uninstaller interface {
	Inspect(ctx context.Context, targets []uninstall.Target) []uninstall.Inspection
	Present(ctx context.Context, t uninstall.Target) (bool, error)
	Run(ctx context.Context, t uninstall.Target, interactive bool, log io.Writer) (source.Result, error)
}

// UninstallPreview is what removing a program would do.
type UninstallPreview struct {
	Target uninstall.Target
	uninstall.Inspection
	Runnable bool
	// Plan is "卸载", or why it will not run.
	Plan string
}

// PreviewUninstall says what removing the programs would do. It runs off
// the loop, reading only the queue and the machine; the queue checks each
// program again right before it runs.
func (c *Core) PreviewUninstall(ctx context.Context, targets []uninstall.Target) []UninstallPreview {
	out := make([]UninstallPreview, len(targets))
	if c.uninstaller == nil {
		for i, t := range targets {
			out[i] = UninstallPreview{Target: t, Plan: "不可用：这个版本不能卸载软件"}
		}
		return out
	}
	ins := c.uninstaller.Inspect(ctx, targets)
	for i, t := range targets {
		p := UninstallPreview{Target: t, Inspection: ins[i]}
		app := queue.UninstallApp(t)
		switch {
		case c.Queue.Pending(app.ID) || c.Queue.PendingSource(app.Source):
			p.Plan = "跳过：已在任务中"
		case p.Blocked != "":
			p.Plan = "不能卸载：" + p.Blocked
		default:
			p.Runnable, p.Plan = true, "卸载"
		}
		out[i] = p
	}
	return out
}

// EnqueueUninstall queues the runnable programs of a preview and returns
// how many.
func (c *Core) EnqueueUninstall(items []UninstallPreview) int {
	var targets []uninstall.Target
	for _, it := range items {
		if it.Runnable {
			targets = append(targets, it.Target)
		}
	}
	if len(targets) == 0 {
		return 0
	}
	return c.Queue.AddUninstall(targets)
}

// presence reads whether a removal's program is still there, as a state:
// Installed or NotInstalled, Unknown when it cannot be read.
func (c *Core) presence(ctx context.Context, t *uninstall.Target) source.Status {
	if t == nil || c.uninstaller == nil {
		return source.UnknownStatus("没有要卸载的程序")
	}
	ok, err := c.uninstaller.Present(ctx, *t)
	switch {
	case err != nil:
		return source.UnknownStatus(err.Error())
	case ok:
		return source.Status{State: source.Installed, Version: t.Version}
	}
	return source.Status{State: source.NotInstalled}
}

func (c *Core) runUninstall(ctx context.Context, it queue.Item, log io.Writer) (source.Result, error) {
	if it.Target == nil || c.uninstaller == nil {
		return source.Result{}, errors.New("没有要卸载的程序")
	}
	t := *it.Target
	fmt.Fprintf(log, "AppShelf 卸载「%s」\n", t.Name)
	if t.Version != "" {
		fmt.Fprintf(log, "版本：%s\n", t.Version)
	}
	if t.Location != "" {
		fmt.Fprintf(log, "位置：%s\n", t.Location)
	}
	fmt.Fprintf(log, "开始：%s\n\n", time.Now().Format(time.DateTime))
	return c.uninstaller.Run(ctx, t, it.Interactive, log)
}

// afterUninstall reads again the states of the catalog's apps that were
// the program removed: they stay in the catalog.
func (c *Core) afterUninstall(t *uninstall.Target) {
	if t == nil || len(t.CatalogIDs) == 0 {
		return
	}
	ids := t.CatalogIDs
	c.post(func() {
		for _, id := range ids {
			if a, ok := c.cat.App(id); ok {
				delete(c.statuses, id)
				c.checkOne(a)
			}
		}
	})
}
