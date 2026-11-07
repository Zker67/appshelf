package view

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"appshelf/internal/queue"
)

func (v *View) tasksPage(c *ui.Context) {
	q := v.core.Queue
	items := q.Items()
	running, stopping := q.Running()
	waiting, done := 0, 0
	for _, it := range items {
		switch {
		case it.State == queue.Waiting:
			waiting++
		case it.State.Done():
			done++
		}
	}
	t := c.Theme()
	ui.Column(c).Grow(1).MinHeight(0).Children(func() {
		ui.Row(c).Padding(10, 16).Gap(8).AlignItems(ui.Center).Children(func() {
			switch {
			case running && stopping:
				muted(c, "当前这一项完成后停止")
			case running:
				muted(c, fmt.Sprintf("正在逐个执行，还有 %d 项等待", waiting))
			case waiting > 0:
				muted(c, fmt.Sprintf("已停止，还有 %d 项等待", waiting))
			case len(items) == 0:
				muted(c, "没有任务")
			default:
				muted(c, "全部完成")
			}
			ui.Spacer(c)
			if running && !stopping && ui.Button(c, "停止").Clicked() {
				q.Stop()
			}
			if !running && waiting > 0 && ui.PrimaryButton(c, "继续").Clicked() {
				q.Resume()
			}
			if ui.Button(c, "清除已完成").Disabled(done == 0).Clicked() {
				q.ClearDone()
			}
		})
		ui.Divider(c)
		if len(items) == 0 {
			ui.Column(c).Grow(1).Center().Children(func() {
				muted(c, "在“软件”页选择软件后，点“安装”或“更新”")
			})
			return
		}
		ui.Scroll(c).Grow(1).Padding(8, 16).Gap(2).Children(func() {
			for _, it := range items {
				ui.Row(c.Key(it.ID)).Gap(14).Padding(8, 0).AlignItems(ui.Center).Children(func() {
					ui.Column(c).Width(220).Shrink(0).Gap(2).Children(func() {
						ui.Text(c, it.App.Name).Bold().SingleLine()
						muted(c, it.Action.String()+" · "+sourceName(it.App.Source.Type)+" "+it.App.Source.Display()).FontSize(12).SingleLine()
					})
					ui.Row(c).Width(90).Shrink(0).Gap(6).AlignItems(ui.Center).Children(func() {
						if it.State == queue.Running {
							ui.Spinner(c).Label("进行中")
						}
						ui.Text(c, it.State.String()).TextColor(itemColor(t, it.State))
					})
					msg := ui.Text(c, it.Message).Grow(1).MinWidth(0)
					if it.State == queue.Failed {
						msg.TextColor(t.Danger)
					} else {
						msg.TextColor(t.TextMuted)
					}
					if it.LogPath != "" && v.hooks.OpenPath != nil {
						path := it.LogPath
						if ui.Button(c, "日志").Clicked() {
							go func() {
								if err := v.hooks.OpenPath(path); err != nil {
									v.hooks.Post(func() { v.showAlert("无法打开日志", err.Error()) })
								}
							}()
						}
					}
					if it.State == queue.Failed && ui.Button(c, "重试").Clicked() {
						q.Retry(it.ID)
					}
				})
				ui.Divider(c)
			}
		})
	})
}
