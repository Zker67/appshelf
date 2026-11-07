package view

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"appshelf/internal/catalog"
	"appshelf/internal/core"
	"appshelf/internal/source/winget"
)

type settingsState struct {
	newCategory, newTag string
	catErr, tagErr      string

	rename struct {
		open     bool
		isTag    bool
		id, name string
		err      string
	}
	del struct {
		open        bool
		isTag       bool
		id          string
		title, text string
	}
	moduleConfirm bool
	moduleBusy    bool

	importOpen bool
	importing  *core.Import
	busyFile   bool
}

func (v *View) settingsPage(c *ui.Context) {
	ui.Scroll(c).Grow(1).Padding(18, 24).Gap(26).Children(func() {
		ui.Column(c).MaxWidth(760).Gap(26).Children(func() {
			v.sourcesSection(c)
			v.namesSection(c, false)
			v.namesSection(c, true)
			v.catalogSection(c)
			v.aboutSection(c)
		})
	})
}

func (v *View) sourcesSection(c *ui.Context) {
	s := &v.settings
	t := c.Theme()
	ui.Column(c).Gap(10).Children(func() {
		sectionTitle(c, "安装来源")
		for _, typ := range v.core.SourceTypes() {
			ui.Row(c.Key("src-" + string(typ))).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, sourceName(typ)).Bold().Width(80).Shrink(0)
				av, ok := v.core.Availability(typ)
				switch {
				case !ok && v.core.Refreshing():
					muted(c, "检查中…")
				case !ok:
					muted(c, "尚未检查")
				case av.OK:
					ui.Text(c, "可用").TextColor(t.Success)
					muted(c, av.Reason).Grow(1).MinWidth(0)
				default:
					ui.Text(c, "不可用").TextColor(t.Warning)
					muted(c, av.Reason).Grow(1).MinWidth(0)
				}
				if typ == catalog.Winget && ok && !av.OK && av.Fixable && v.core.CanInstallModule() {
					if s.moduleBusy {
						ui.Spinner(c).Label("正在安装模块")
						muted(c, "正在安装模块…")
					} else if ui.PrimaryButton(c, "安装模块").Clicked() {
						s.moduleConfirm = true
					}
				}
			})
		}
		rate, token := v.core.GitHubRate()
		var lines []string
		if !token {
			lines = append(lines, "GitHub 未设置 token，每小时最多 60 次请求。需要更多时，可以设置环境变量 GH_TOKEN 或 GITHUB_TOKEN 后重新打开 AppShelf。")
		}
		if rate.Known {
			lines = append(lines, fmt.Sprintf("GitHub 剩余请求 %d / %d，%s 重置。", rate.Remaining, rate.Limit, rate.Reset.Local().Format("15:04")))
		}
		for _, l := range lines {
			muted(c, l).FontSize(12)
		}
		if ui.Button(c, "重新检查").Disabled(v.core.Refreshing()).Clicked() {
			v.core.Refresh()
		}
	})
}

// namesSection lists the categories, or the tags, to add, rename and
// delete.
func (v *View) namesSection(c *ui.Context, tags bool) {
	s := &v.settings
	cat := v.core.Catalog()
	type entry struct{ id, name string }
	var list []entry
	title, input, errp := "分类", &s.newCategory, &s.catErr
	if tags {
		title, input, errp = "标签", &s.newTag, &s.tagErr
		for _, k := range cat.Tags {
			list = append(list, entry{k.ID, k.Name})
		}
	} else {
		for _, k := range cat.Categories {
			list = append(list, entry{k.ID, k.Name})
		}
	}
	ui.Column(c.Key("names-" + title)).Gap(8).Children(func() {
		sectionTitle(c, title)
		if len(list) == 0 {
			muted(c, "还没有"+title)
		}
		for _, k := range list {
			n := cat.CategoryUsage(k.id)
			if tags {
				n = cat.TagUsage(k.id)
			}
			ui.Row(c.Key(k.id)).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, k.name).Grow(1).MinWidth(0).SingleLine()
				muted(c, fmt.Sprintf("%d 个软件", n))
				if ui.Button(c, "重命名").Clicked() {
					r := &s.rename
					r.open, r.isTag, r.id, r.name, r.err = true, tags, k.id, k.name, ""
				}
				if ui.Button(c, "删除").Clicked() {
					d := &s.del
					d.open, d.isTag, d.id = true, tags, k.id
					d.title = "删除" + title + "「" + k.name + "」？"
					switch {
					case n == 0:
						d.text = "没有软件使用它。"
					case tags:
						d.text = fmt.Sprintf("%d 个软件将去掉这个标签，软件本身不会删除。", n)
					default:
						d.text = fmt.Sprintf("%d 个软件将变为未分类，软件本身不会删除。", n)
					}
				}
			})
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			submitted := ui.TextInput(c, input).Placeholder("新" + title + "名称").Label("新" + title).Width(260).Submitted()
			if ui.Button(c, "添加").Clicked() || submitted {
				var err error
				if tags {
					err = v.core.AddTag(*input)
				} else {
					err = v.core.AddCategory(*input)
				}
				if err != nil {
					*errp = err.Error()
				} else {
					*input, *errp = "", ""
				}
			}
		})
		errorText(c, *errp)
	})
}

func (v *View) catalogSection(c *ui.Context) {
	s := &v.settings
	ui.Column(c).Gap(8).Children(func() {
		sectionTitle(c, "清单")
		muted(c, "导出的文件可以在其他电脑上导入。导入时按来源合并：已有的软件保留本机的设置，只补充标签和分类。").FontSize(12)
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, "导出清单").Disabled(s.busyFile).Clicked() {
				v.startExport()
			}
			if ui.Button(c, "导入清单").Disabled(s.busyFile).Clicked() {
				v.startImport()
			}
		})
		muted(c, "清单文件："+v.core.Paths().Catalog).FontSize(12).Selectable()
	})
}

func (v *View) aboutSection(c *ui.Context) {
	ui.Column(c).Gap(8).Children(func() {
		sectionTitle(c, "关于")
		ver := v.hooks.Version
		if ver == "" {
			ver = "开发版本"
		}
		ui.Text(c, "AppShelf "+ver)
		if v.hooks.CheckUpdates == nil {
			muted(c, "自更新只在用安装包安装的版本里可用").FontSize(12)
		} else if ui.Button(c, "检查 AppShelf 更新").Clicked() {
			v.hooks.CheckUpdates()
		}
		muted(c, "日志："+v.core.Paths().Logs).FontSize(12).Selectable()
	})
}

func (v *View) startExport() {
	s := &v.settings
	if v.hooks.SaveFile == nil {
		return
	}
	s.busyFile = true
	go func() {
		path, err := v.hooks.SaveFile("导出清单", "appshelf-catalog.json")
		v.hooks.Post(func() {
			s.busyFile = false
			switch {
			case err != nil:
				v.showAlert("导出失败", err.Error())
			case path == "":
			default:
				if !strings.EqualFold(filepath.Ext(path), ".json") {
					path += ".json"
				}
				if err := v.core.Export(path); err != nil {
					v.showAlert("导出失败", err.Error())
				} else {
					v.showAlert("已导出", fmt.Sprintf("%d 个软件已导出到 %s", len(v.core.Catalog().Apps), path))
				}
			}
		})
	}()
}

func (v *View) startImport() {
	s := &v.settings
	if v.hooks.OpenFile == nil {
		return
	}
	s.busyFile = true
	go func() {
		path, err := v.hooks.OpenFile("导入清单")
		v.hooks.Post(func() {
			s.busyFile = false
			switch {
			case err != nil:
				v.showAlert("导入失败", err.Error())
			case path == "":
			default:
				im, err := v.core.ReadImport(path)
				if err != nil {
					v.showAlert("无法导入", err.Error())
					return
				}
				s.importing, s.importOpen = im, true
			}
		})
	}()
}

// settingsDialogs are the dialogs the settings open: renaming, deleting,
// installing the module and confirming an import.
func (v *View) settingsDialogs(c *ui.Context) {
	s := &v.settings
	r := &s.rename
	ui.Modal(c, &r.open, func() {
		what := "分类"
		if r.isTag {
			what = "标签"
		}
		ui.Column(c).Width(360).Gap(12).Children(func() {
			ui.Text(c, "重命名"+what).FontSize(16).Bold()
			submitted := ui.TextInput(c, &r.name).AutoFocus().Label("名称").Submitted()
			errorText(c, r.err)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Clicked() {
					r.open = false
				}
				if ui.PrimaryButton(c, "保存").Clicked() || submitted {
					var err error
					if r.isTag {
						err = v.core.RenameTag(r.id, r.name)
					} else {
						err = v.core.RenameCategory(r.id, r.name)
					}
					if err != nil {
						r.err = err.Error()
					} else {
						r.open = false
					}
				}
			})
		})
	})

	d := &s.del
	if ui.AlertDialog(c, &d.open, d.title, d.text, "取消", "删除") == 1 {
		var err error
		if d.isTag {
			delete(v.shelf.tags, d.id)
			err = v.core.DeleteTag(d.id)
		} else {
			delete(v.shelf.cats, d.id)
			err = v.core.DeleteCategory(d.id)
		}
		if err != nil {
			v.showAlert("删除失败", err.Error())
		}
	}

	if ui.AlertDialog(c, &s.moduleConfirm, "安装 "+winget.ModuleName+"？",
		"AppShelf 用这个 PowerShell 模块读取 winget 的已装软件和更新。将以当前用户身份从 PowerShell Gallery 安装，不需要管理员权限。", "取消", "安装") == 1 {
		s.moduleBusy = true
		v.core.InstallModule(func(err error) {
			s.moduleBusy = false
			if err != nil {
				v.showAlert("安装模块失败", err.Error())
			}
		})
	}

	if s.importing != nil {
		sum := s.importing.Summary
		text := fmt.Sprintf("将新增 %d 个软件，合并 %d 个已有的软件，新增 %d 个分类、%d 个标签。", sum.AddedApps, sum.MergedApps, sum.AddedCategories, sum.AddedTags)
		switch ui.AlertDialog(c, &s.importOpen, "导入「"+filepath.Base(s.importing.Path)+"」？", text, "取消", "导入") {
		case 1:
			im := s.importing
			s.importing = nil
			if _, err := v.core.ApplyImport(im); err != nil {
				v.showAlert("导入失败", err.Error())
			}
		case ui.AlertDismissed, 0:
			s.importing = nil
		}
	}
}
