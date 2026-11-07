// Package view is AppShelf's window, in MyGo's native UI: the software
// page, the tasks of the install queue, and the settings.
package view

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"appshelf/internal/catalog"
	"appshelf/internal/core"
	"appshelf/internal/queue"
	"appshelf/internal/source"
)

// Hooks are what the view asks of the desktop. They block, so the view
// calls them on goroutines of their own and hands results back with Post.
type Hooks struct {
	// Post runs a function on the UI thread.
	Post func(func())
	// OpenFile asks for a file to open; "" when canceled.
	OpenFile func(title string) (string, error)
	// SaveFile asks where to save a file; "" when canceled.
	SaveFile func(title, name string) (string, error)
	// OpenPath opens a file with its app, as a log in the editor.
	OpenPath func(path string) error
	// CheckUpdates checks for a newer AppShelf; nil when it cannot.
	CheckUpdates func()
	// Version is AppShelf's version, "" in development.
	Version string
}

const (
	tabShelf = iota
	tabTasks
	tabSettings
)

// View is the window's state.
type View struct {
	core  *core.Core
	hooks Hooks
	tab   int

	shelf    shelfState
	editor   editorState
	settings settingsState

	alertOpen  bool
	alertTitle string
	alertText  string
	notesShown bool
}

// New returns the view of the application.
func New(c *core.Core, h Hooks) *View {
	if h.Post == nil {
		h.Post = func(fn func()) { fn() }
	}
	v := &View{core: c, hooks: h}
	v.shelf.cats = map[string]bool{}
	v.shelf.tags = map[string]bool{}
	return v
}

// Build builds the window.
func (v *View) Build(c *ui.Context) {
	if !v.notesShown && len(v.core.Notes) > 0 && !v.alertOpen {
		v.notesShown = true
		v.showAlert("打开时的提示", strings.Join(v.core.Notes, "\n"))
	}
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Padding(10, 16).Gap(16).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "AppShelf").FontSize(16).Bold()
			ui.Tabs(c, &v.tab, "软件", v.tasksTabLabel(), "设置")
			ui.Spacer(c)
			v.refreshStatus(c)
		})
		ui.Divider(c)
		ui.Column(c).Grow(1).MinHeight(0).Children(func() {
			switch v.tab {
			case tabTasks:
				v.tasksPage(c)
			case tabSettings:
				v.settingsPage(c)
			default:
				v.shelfPage(c)
			}
		})
	})
	v.previewDialog(c)
	v.editorDialog(c)
	v.settingsDialogs(c)
	if ui.AlertDialog(c, &v.alertOpen, v.alertTitle, v.alertText, "好") >= 0 {
		v.alertOpen = false
	}
}

func (v *View) showAlert(title, text string) {
	v.alertTitle, v.alertText, v.alertOpen = title, text, true
}

func (v *View) tasksTabLabel() string {
	n := 0
	for _, it := range v.core.Queue.Items() {
		if !it.State.Done() {
			n++
		}
	}
	if n == 0 {
		return "任务"
	}
	return fmt.Sprintf("任务（%d）", n)
}

// refreshStatus shows whether the states are being read, and when they
// were.
func (v *View) refreshStatus(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		switch {
		case v.core.Refreshing():
			ui.Spinner(c).Label("正在检查")
			ui.Text(c, "正在检查安装状态和更新…").TextColor(t.TextMuted)
		case !v.core.LastRefresh().IsZero():
			ui.Text(c, "上次检查 "+v.core.LastRefresh().Format("15:04")).TextColor(t.TextMuted)
		}
		if ui.Button(c, "检查更新").Disabled(v.core.Refreshing()).Clicked() {
			v.core.Refresh()
		}
	})
}

// sectionTitle starts a part of a page.
func sectionTitle(c *ui.Context, s string) {
	ui.Text(c, s).FontSize(15).Bold()
}

// field puts a label above a control.
func field(c *ui.Context, label string, fn func()) {
	ui.Column(c).Gap(4).Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(c.Theme().TextMuted)
		fn()
	})
}

// muted writes secondary text.
func muted(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).TextColor(c.Theme().TextMuted)
}

// errorText writes an error.
func errorText(c *ui.Context, s string) {
	if s != "" {
		ui.Text(c, s).TextColor(c.Theme().Danger)
	}
}

// sourceName is a source as the interface names it.
func sourceName(t catalog.SourceType) string {
	switch t {
	case catalog.Winget:
		return "winget"
	case catalog.Scoop:
		return "scoop"
	case catalog.GitHub:
		return "GitHub"
	}
	return string(t)
}

// statusLabel describes an app's state in a few words, with the reason to
// show on hover, and its rank for sorting: what needs action first.
func (v *View) statusLabel(a catalog.App) (text, tip string, rank int, st source.Status) {
	if v.core.Queue.Pending(a.ID) {
		return "任务中", "", 0, st
	}
	st, ok := v.core.Status(a.ID)
	if !ok {
		if v.core.Checking(a.ID) {
			return "检查中…", "", 6, st
		}
		return "未检查", "点“检查更新”读取安装状态", 6, st
	}
	switch st.State {
	case source.UpdateAvailable:
		return "可更新 " + orQ(st.Version) + " → " + st.Latest, st.Note, 1, st
	case source.NotInstalled:
		return "未安装", st.Note, 2, st
	case source.Installed:
		text := "已安装"
		if st.External {
			text = "已安装（来源外）"
		} else if st.Version != "" {
			text += " " + st.Version
		}
		if st.Note != "" && !st.External {
			text += " ⚠"
		}
		return text, st.Note, 3, st
	case source.Unavailable:
		return "不可用：" + st.Note, st.Note, 5, st
	}
	return "未知：" + st.Note, st.Note, 4, st
}

func orQ(s string) string {
	if s == "" {
		return "?"
	}
	return s
}

// statusColor colors a state, as the theme writes success and warnings.
func statusColor(t *ui.Theme, st source.Status, known bool) ui.Color {
	if !known {
		return t.TextMuted
	}
	switch st.State {
	case source.UpdateAvailable:
		return t.Accent
	case source.Installed:
		return t.Success
	case source.Unknown:
		return t.Warning
	case source.Unavailable:
		return t.TextMuted
	}
	return t.Text
}

// itemColor colors a task's state.
func itemColor(t *ui.Theme, s queue.ItemState) ui.Color {
	switch s {
	case queue.Succeeded:
		return t.Success
	case queue.Failed:
		return t.Danger
	case queue.Running:
		return t.Accent
	}
	return t.TextMuted
}
