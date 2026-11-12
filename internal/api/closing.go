package api

import (
	"context"
	"fmt"
	"io"

	"appshelf/internal/procs"
	"appshelf/internal/queue"
	"appshelf/internal/source"
)

// closeForUpdate closes the program an update the user started replaces,
// right before it runs, for the installer not to fail on files in use (as
// winget's 0x8A150101) or to close it less gently: what runs from its own
// directory is asked to close, ended once it has no window left, and a
// window that stays open, as one asking whether to save, is left for the
// user to close, the update failing when it is not (procs.Closer). An
// update AppShelf started itself closes nothing: its hold made sure the
// program does not run. A program whose directory is not known well enough
// is left to its installer, as before.
func (a *App) closeForUpdate(ctx context.Context, it queue.Item, log io.Writer) error {
	shut := a.cfg.Close
	if shut == nil || it.Auto {
		return nil
	}
	p := a.packages().byKey[it.App.Source.Key()]
	if p == nil {
		return nil
	}
	dir := installDir(p)
	if dir == "" {
		return nil
	}
	name := it.App.Name
	res, err := shut(ctx, dir, func(e procs.CloseEvent) {
		switch e.Step {
		case procs.CloseAsking:
			fmt.Fprintf(log, "[AppShelf] 「%s」正在运行（%s），先关闭它再更新\n", name, procs.Names(e.Procs))
			source.ReportProgress(log, source.Progress{Stage: "正在关闭「" + name + "」"})
		case procs.CloseEnding:
			fmt.Fprintf(log, "[AppShelf] 结束没有窗口的进程：%s\n", procs.Names(e.Procs))
		case procs.CloseWaiting:
			fmt.Fprintf(log, "[AppShelf] 「%s」的窗口还开着，可能在问你是否保存；关闭它后继续更新\n", orName(e.Window.Title, name))
			source.ReportProgress(log, source.Progress{Stage: "等待你关闭「" + name + "」（它可能在问你是否保存）", Cancelable: true})
			a.notifyTasks("「"+name+"」还开着，更新在等你关闭它", "它可能在问你是否保存。关闭它后会继续更新。")
		}
	})
	if err != nil {
		return err
	}
	if len(res.Denied) > 0 {
		fmt.Fprintf(log, "[AppShelf] 无法关闭 %s（以管理员身份运行），交给安装程序处理\n", procs.Names(res.Denied))
	}
	if len(res.Open) > 0 {
		fmt.Fprintf(log, "[AppShelf] 「%s」一直没有关闭，没有更新\n", name)
		return fmt.Errorf("「%s」还开着（可能在问你是否保存），没有更新；关闭它后重试", name)
	}
	if len(res.Found) > 0 {
		fmt.Fprintf(log, "[AppShelf] 已关闭「%s」\n\n", name)
	}
	return nil
}

func orName(title, name string) string {
	if title != "" {
		return title
	}
	return name
}
