package view

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"appshelf/internal/catalog"
	"appshelf/internal/core"
	"appshelf/internal/queue"
)

// uncategorized is the filter key of the apps without a category.
const uncategorized = ""

type shelfState struct {
	cats   map[string]bool // category filter, by ID; uncategorized for none
	tags   map[string]bool // tag filter, by ID
	list   ui.ListState
	chosen ui.Selection[string]
	cursor int
	sort   ui.SortOrder

	preview struct {
		open   bool
		action queue.Action
		items  []core.PreviewItem
	}
}

// filtering reports whether any category or tag is chosen.
func (s *shelfState) filtering() bool {
	for _, on := range s.cats {
		if on {
			return true
		}
	}
	for _, on := range s.tags {
		if on {
			return true
		}
	}
	return false
}

// visibleApps returns the apps the filter shows, sorted as the table says:
// with categories or tags chosen, those in any chosen category or with
// any chosen tag.
func (v *View) visibleApps() []catalog.App {
	s := &v.shelf
	cat := v.core.Catalog()
	var rows []catalog.App
	all := !s.filtering()
	for _, a := range cat.Apps {
		show := all || s.cats[a.CategoryID]
		for _, t := range a.TagIDs {
			show = show || s.tags[t]
		}
		if show {
			rows = append(rows, a)
		}
	}
	if s.sort.Column == "" {
		return rows
	}
	key := func(a catalog.App) string {
		switch s.sort.Column {
		case "cat":
			c, _ := cat.Category(a.CategoryID)
			return c.Name
		case "src":
			return sourceName(a.Source.Type) + " " + a.Source.Display()
		}
		return a.Name
	}
	slices.SortStableFunc(rows, func(x, y catalog.App) int {
		var r int
		if s.sort.Column == "status" {
			_, _, rx, _ := v.statusLabel(x)
			_, _, ry, _ := v.statusLabel(y)
			r = cmp.Compare(rx, ry)
		} else {
			r = cmp.Compare(strings.ToLower(key(x)), strings.ToLower(key(y)))
		}
		if s.sort.Descending {
			r = -r
		}
		return r
	})
	return rows
}

func (v *View) shelfPage(c *ui.Context) {
	rows := v.visibleApps()
	// Keep only visible rows chosen: what the buttons act on is what shows.
	visible := map[string]bool{}
	for _, a := range rows {
		visible[a.ID] = true
	}
	var gone []string
	for id := range v.shelf.chosen.All() {
		if !visible[id] {
			gone = append(gone, id)
		}
	}
	for _, id := range gone {
		v.shelf.chosen.Remove(id)
	}

	ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
		v.filterPanel(c)
		ui.Divider(c)
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			v.shelfToolbar(c, len(rows))
			if len(v.core.Catalog().Apps) == 0 {
				v.emptyShelf(c)
			} else {
				v.appTable(c, rows)
			}
			ui.Divider(c)
			v.actionBar(c, rows)
		})
	})
}

func (v *View) filterPanel(c *ui.Context) {
	s := &v.shelf
	cat := v.core.Catalog()
	ui.Scroll(c).Width(210).Shrink(0).Padding(14, 14).Gap(6).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			sectionTitle(c, "分类")
			ui.Spacer(c)
			if s.filtering() && ui.Button(c, "清除").Label("清除筛选").Clicked() {
				clear(s.cats)
				clear(s.tags)
			}
		})
		if len(cat.Categories) == 0 {
			muted(c, "还没有分类").FontSize(12)
		}
		for _, k := range cat.Categories {
			on := s.cats[k.ID]
			ui.Checkbox(c.Key("cat-"+k.ID), &on, fmt.Sprintf("%s（%d）", k.Name, cat.CategoryUsage(k.ID)))
			if on != s.cats[k.ID] {
				s.cats[k.ID] = on
			}
		}
		if n := cat.CategoryUsage(""); n > 0 {
			on := s.cats[uncategorized]
			ui.Checkbox(c.Key("cat-none"), &on, fmt.Sprintf("未分类（%d）", n))
			if on != s.cats[uncategorized] {
				s.cats[uncategorized] = on
			}
		}
		ui.Box(c).Height(10)
		sectionTitle(c, "标签")
		if len(cat.Tags) == 0 {
			muted(c, "还没有标签").FontSize(12)
		}
		for _, k := range cat.Tags {
			on := s.tags[k.ID]
			ui.Checkbox(c.Key("tag-"+k.ID), &on, fmt.Sprintf("%s（%d）", k.Name, cat.TagUsage(k.ID)))
			if on != s.tags[k.ID] {
				s.tags[k.ID] = on
			}
		}
		if len(cat.Categories) == 0 && len(cat.Tags) == 0 {
			ui.Box(c).Height(6)
			if ui.Button(c, "去设置里添加").Clicked() {
				v.tab = tabSettings
			}
		}
	})
}

func (v *View) shelfToolbar(c *ui.Context, shown int) {
	total := len(v.core.Catalog().Apps)
	ui.Row(c).Padding(10, 16).Gap(10).AlignItems(ui.Center).Children(func() {
		if ui.PrimaryButton(c, "添加软件").Clicked() {
			v.openEditor(nil)
		}
		if shown == total {
			muted(c, fmt.Sprintf("共 %d 个软件", total))
		} else {
			muted(c, fmt.Sprintf("显示 %d 个，共 %d 个", shown, total))
		}
	})
}

func (v *View) emptyShelf(c *ui.Context) {
	ui.Column(c).Grow(1).Center().Gap(12).Children(func() {
		ui.Text(c, "清单是空的").FontSize(16).Bold()
		muted(c, "添加软件，或者导入已有的清单文件。")
		ui.Row(c).Gap(8).Children(func() {
			if ui.PrimaryButton(c, "添加软件").Clicked() {
				v.openEditor(nil)
			}
			if ui.Button(c, "导入清单").Clicked() {
				v.startImport()
			}
		})
	})
}

var tableColumns = []ui.TableColumn{
	{Title: "名称", ID: "name", MinWidth: 160, Sortable: true},
	{Title: "分类", ID: "cat", Width: 120, Sortable: true},
	{Title: "标签", ID: "tags", Width: 160},
	{Title: "来源", ID: "src", Width: 240, Sortable: true},
	{Title: "状态", ID: "status", Width: 220, Sortable: true},
}

func (v *View) appTable(c *ui.Context, rows []catalog.App) {
	s := &v.shelf
	t := c.Theme()
	cat := v.core.Catalog()
	if len(rows) == 0 {
		ui.Column(c).Grow(1).Center().Children(func() {
			muted(c, "没有符合筛选的软件")
		})
		return
	}
	s.list.Key = func(i int) any { return rows[i].ID }
	s.list.Label = func(i int) string { return rows[i].Name }
	s.list.Selection = &s.chosen
	s.list.Selected = &s.cursor
	s.list.Sort = &s.sort
	table := ui.Table(c, &s.list, tableColumns, len(rows), func(row, col int) {
		a := rows[row]
		chosen := s.chosen.Has(a.ID)
		switch col {
		case 0:
			ui.Text(c, a.Name).SingleLine()
		case 1:
			name := "—"
			if k, ok := cat.Category(a.CategoryID); ok {
				name = k.Name
			}
			ui.Text(c, name).SingleLine()
		case 2:
			var names []string
			for _, id := range a.TagIDs {
				if k, ok := cat.Tag(id); ok {
					names = append(names, k.Name)
				}
			}
			ui.Text(c, strings.Join(names, "、")).SingleLine()
		case 3:
			ui.Text(c, sourceName(a.Source.Type)+" · "+a.Source.Display()).SingleLine()
		case 4:
			text, tip, _, st := v.statusLabel(a)
			_, known := v.core.Status(a.ID)
			el := ui.Text(c, text).SingleLine()
			if !chosen {
				el.TextColor(statusColor(t, st, known && !v.core.Queue.Pending(a.ID)))
			}
			if tip != "" {
				el.Tooltip(tip)
			}
		}
	}).Grow(1).Label("软件")
	if table.Submitted() && s.cursor >= 0 && s.cursor < len(rows) {
		a := rows[s.cursor]
		v.openEditor(&a)
	}
}

// chosenIDs returns the chosen apps, in the table's order.
func (v *View) chosenIDs(rows []catalog.App) []string {
	var ids []string
	for _, a := range rows {
		if v.shelf.chosen.Has(a.ID) {
			ids = append(ids, a.ID)
		}
	}
	return ids
}

func (v *View) actionBar(c *ui.Context, rows []catalog.App) {
	s := &v.shelf
	ids := v.chosenIDs(rows)
	ui.Row(c).Padding(10, 16).Gap(8).AlignItems(ui.Center).Children(func() {
		muted(c, fmt.Sprintf("已选 %d 项", len(ids)))
		ui.Spacer(c)
		if len(rows) > 0 && len(ids) == len(rows) {
			if ui.Button(c, "取消全选").Clicked() {
				s.chosen.Clear()
			}
		} else if ui.Button(c, "全选").Disabled(len(rows) == 0).Clicked() {
			for _, a := range rows {
				s.chosen.Add(a.ID)
			}
		}
		if ui.Button(c, "更新").Disabled(len(ids) == 0).Clicked() {
			v.openPreview(ids, queue.Upgrade)
		}
		if ui.PrimaryButton(c, "安装").Disabled(len(ids) == 0).Clicked() {
			v.openPreview(ids, queue.Install)
		}
	})
}

func (v *View) openPreview(ids []string, action queue.Action) {
	p := &v.shelf.preview
	p.action, p.items, p.open = action, v.core.Preview(ids, action), true
}

func (v *View) previewDialog(c *ui.Context) {
	p := &v.shelf.preview
	t := c.Theme()
	ui.Modal(c, &p.open, func() {
		runnable := 0
		for _, it := range p.items {
			if it.Runnable {
				runnable++
			}
		}
		ui.Column(c).Width(640).Gap(14).Children(func() {
			ui.Text(c, p.action.String()+"预览").FontSize(16).Bold()
			ui.Scroll(c).MaxHeight(400).Gap(10).Children(func() {
				for _, it := range p.items {
					ui.Row(c.Key(it.App.ID)).Gap(16).AlignItems(ui.Start).Children(func() {
						ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
							ui.Text(c, it.App.Name).Bold()
							muted(c, it.Detail).FontSize(12)
						})
						plan := ui.Text(c, it.Plan).Width(230).TextAlign(ui.End)
						if it.Runnable {
							plan.TextColor(t.Accent)
						} else {
							plan.TextColor(t.TextMuted)
						}
					})
				}
			})
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if runnable > 0 {
					muted(c, fmt.Sprintf("将逐个%s %d 项，执行前会再检查一次安装状态", p.action, runnable))
				} else {
					muted(c, "没有需要执行的项")
				}
				ui.Spacer(c)
				if ui.Button(c, "取消").Clicked() {
					p.open = false
				}
				if ui.PrimaryButton(c, "开始").Disabled(runnable == 0).Clicked() {
					v.core.Enqueue(p.items, p.action)
					p.open = false
					v.shelf.chosen.Clear()
					v.tab = tabTasks
				}
			})
		})
	})
}
