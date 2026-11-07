package view

import (
	"github.com/egoist/mygo/ui"

	"appshelf/internal/catalog"
	"appshelf/internal/source"
	"appshelf/internal/source/github"
)

// noCategory is the category option of an app without one.
const noCategory = "（无分类）"

var sourceTabs = []catalog.SourceType{catalog.Winget, catalog.Scoop, catalog.GitHub}

type editorState struct {
	open     bool
	id       string // "" for a new app
	name     string
	category string // the option chosen: a category's name, or noCategory
	tags     map[string]bool
	srcIdx   int

	wingetID  string
	bucket    string
	pkg       string // scoop's app name
	bucketURL string
	repo      string
	pattern   string
	args      string
	ruleName  string
	ruleExe   string

	query     string
	searching bool
	searchGen int
	results   []source.SearchResult
	searchErr string

	ghBusy bool
	ghGen  int
	ghText string
	ghErr  string

	err           string
	confirmDelete bool
}

// openEditor opens the editor on an app, or on a new one for nil.
func (v *View) openEditor(a *catalog.App) {
	e := &v.editor
	*e = editorState{open: true, category: noCategory, tags: map[string]bool{}, searchGen: e.searchGen + 1, ghGen: e.ghGen + 1}
	if a == nil {
		return
	}
	e.id, e.name = a.ID, a.Name
	if k, ok := v.core.Catalog().Category(a.CategoryID); ok {
		e.category = k.Name
	}
	for _, t := range a.TagIDs {
		e.tags[t] = true
	}
	for i, t := range sourceTabs {
		if t == a.Source.Type {
			e.srcIdx = i
		}
	}
	s := a.Source
	e.wingetID, e.bucket, e.pkg, e.bucketURL = s.ID, s.Bucket, s.Name, s.BucketURL
	e.repo, e.pattern, e.args = s.Repo, s.AssetPattern, s.InstallerArgs
	if a.Detect != nil {
		e.ruleName, e.ruleExe = a.Detect.DisplayName, a.Detect.ExePath
	}
}

// draft is the app the editor's fields describe.
func (v *View) draft() catalog.App {
	e := &v.editor
	a := catalog.App{ID: e.id, Name: e.name}
	for _, k := range v.core.Catalog().Categories {
		if k.Name == e.category {
			a.CategoryID = k.ID
		}
	}
	for _, k := range v.core.Catalog().Tags {
		if e.tags[k.ID] {
			a.TagIDs = append(a.TagIDs, k.ID)
		}
	}
	switch sourceTabs[e.srcIdx] {
	case catalog.Winget:
		a.Source = catalog.Source{Type: catalog.Winget, ID: e.wingetID}
	case catalog.Scoop:
		a.Source = catalog.Source{Type: catalog.Scoop, Bucket: e.bucket, Name: e.pkg, BucketURL: e.bucketURL}
	case catalog.GitHub:
		a.Source = catalog.Source{Type: catalog.GitHub, Repo: e.repo, AssetPattern: e.pattern, InstallerArgs: e.args}
	}
	if e.ruleName != "" || e.ruleExe != "" {
		a.Detect = &catalog.Detect{DisplayName: e.ruleName, ExePath: e.ruleExe}
	}
	return a
}

func (v *View) saveEditor() {
	e := &v.editor
	a := v.draft()
	switch a.Source.Type {
	case catalog.Winget:
		if a.Source.ID == "" {
			e.err = "请先搜索，并从结果中选择一个 winget 软件包"
			return
		}
	case catalog.Scoop:
		if a.Source.Bucket == "" || a.Source.Name == "" {
			e.err = "请先搜索，并从结果中选择一个 scoop 软件"
			return
		}
	}
	if _, err := v.core.SaveApp(a); err != nil {
		e.err = err.Error()
		return
	}
	e.open = false
}

func (v *View) editorDialog(c *ui.Context) {
	e := &v.editor
	if !e.open {
		e.confirmDelete = false
	}
	ui.Modal(c, &e.open, func() {
		ui.Column(c).Width(600).Gap(14).Children(func() {
			title := "添加软件"
			if e.id != "" {
				title = "编辑软件"
			}
			ui.Text(c, title).FontSize(16).Bold()
			ui.Scroll(c).MaxHeight(520).Gap(14).Children(func() {
				v.editorFields(c)
			})
			errorText(c, e.err)
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if e.id != "" && ui.Button(c, "删除").Clicked() {
					e.confirmDelete = true
				}
				ui.Spacer(c)
				if ui.Button(c, "取消").Clicked() {
					e.open = false
				}
				if ui.PrimaryButton(c, "保存").Clicked() {
					v.saveEditor()
				}
			})
		})
	})
	if e.open {
		switch ui.AlertDialog(c, &e.confirmDelete, "删除「"+e.name+"」？", "只从清单中删除，不会卸载软件。", "取消", "删除") {
		case 1:
			if err := v.core.DeleteApp(e.id); err != nil {
				e.err = err.Error()
			} else {
				v.shelf.chosen.Remove(e.id)
				e.open = false
			}
		}
	}
}

func (v *View) editorFields(c *ui.Context) {
	e := &v.editor
	cat := v.core.Catalog()
	field(c, "名称", func() {
		ui.TextInput(c, &e.name).Label("软件名称").Placeholder("软件名称")
	})
	ui.Row(c).Gap(16).AlignItems(ui.Start).Children(func() {
		ui.Column(c).Width(200).Children(func() {
			field(c, "分类", func() {
				opts := []string{noCategory}
				for _, k := range cat.Categories {
					opts = append(opts, k.Name)
				}
				ui.Select(c, &e.category, opts).Label("分类")
			})
		})
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			field(c, "标签", func() {
				if len(cat.Tags) == 0 {
					muted(c, "还没有标签，可以在设置里添加").FontSize(12)
					return
				}
				ui.Row(c).Wrap().GapX(12).GapY(6).Children(func() {
					for _, k := range cat.Tags {
						on := e.tags[k.ID]
						ui.Checkbox(c.Key("etag-"+k.ID), &on, k.Name)
						if on != e.tags[k.ID] {
							e.tags[k.ID] = on
						}
					}
				})
			})
		})
	})
	field(c, "安装来源", func() {
		before := e.srcIdx
		ui.Segmented(c, &e.srcIdx, "winget", "scoop", "GitHub").Label("安装来源")
		if e.srcIdx != before {
			e.results, e.searchErr, e.err = nil, "", ""
			e.searchGen++
			e.searching = false
		}
	})
	switch sourceTabs[e.srcIdx] {
	case catalog.Winget:
		v.searchFields(c, catalog.Winget)
	case catalog.Scoop:
		v.searchFields(c, catalog.Scoop)
	case catalog.GitHub:
		v.githubFields(c)
	}
	v.ruleFields(c)
}

// searchFields find the exact package of winget or scoop: the user picks
// a search result rather than typing an ID.
func (v *View) searchFields(c *ui.Context, t catalog.SourceType) {
	e := &v.editor
	chosen := ""
	switch t {
	case catalog.Winget:
		chosen = e.wingetID
	case catalog.Scoop:
		if e.pkg != "" {
			chosen = e.bucket + "/" + e.pkg
		}
	}
	field(c, "已选择的软件包", func() {
		if chosen == "" {
			muted(c, "还没有选择，请在下面搜索")
		} else {
			ui.Text(c, chosen).Bold().Selectable()
		}
	})
	av, checked := v.core.Availability(t)
	if checked && !av.OK {
		muted(c, "本机无法搜索 "+sourceName(t)+"："+av.Reason)
		if t == catalog.Scoop {
			muted(c, "可以在装有 scoop 的电脑上添加，或者通过导入清单添加。").FontSize(12)
		}
		return
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		submitted := ui.TextInput(c, &e.query).Placeholder("搜索 " + sourceName(t)).Label("搜索关键词").Grow(1).Submitted()
		if ui.Button(c, "搜索").Disabled(e.searching).Clicked() || submitted {
			v.search(t)
		}
		if e.searching {
			ui.Spinner(c).Label("搜索中")
		}
	})
	errorText(c, e.searchErr)
	if t == catalog.Scoop {
		field(c, "bucket 地址（仅自定义 bucket 需要）", func() {
			ui.TextInput(c, &e.bucketURL).Placeholder("https://github.com/…").Label("bucket 地址")
		})
	}
	if len(e.results) == 0 {
		return
	}
	ui.Column(c).Gap(4).Children(func() {
		for _, r := range e.results {
			ui.Row(c.Key("r-"+r.ID)).Gap(10).AlignItems(ui.Center).Padding(4, 0).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Children(func() {
					ui.Text(c, r.Name).SingleLine()
					muted(c, r.ID+"  "+r.Version).FontSize(12).SingleLine()
				})
				if ui.Button(c, "选择").Clicked() {
					switch t {
					case catalog.Winget:
						e.wingetID = r.ID
					case catalog.Scoop:
						e.bucket, e.pkg = r.Source, r.Name
					}
					if e.name == "" {
						e.name = r.Name
					}
					e.err = ""
				}
			})
		}
	})
}

func (v *View) search(t catalog.SourceType) {
	e := &v.editor
	if e.query == "" {
		return
	}
	e.searchGen++
	gen := e.searchGen
	e.searching, e.searchErr = true, ""
	v.core.Search(t, e.query, func(res []source.SearchResult, err error) {
		if gen != e.searchGen {
			return
		}
		e.searching, e.results = false, res
		switch {
		case err != nil:
			e.searchErr = err.Error()
		case len(res) == 0:
			e.searchErr = "没有找到"
		}
	})
}

func (v *View) githubFields(c *ui.Context) {
	e := &v.editor
	field(c, "GitHub 仓库", func() {
		ui.TextInput(c, &e.repo).Placeholder("owner/name").Label("仓库 owner/name")
	})
	field(c, "安装包匹配规则（可选，正则表达式）", func() {
		ui.TextInput(c, &e.pattern).Placeholder(`例如 x64\.msi$`).Label("安装包匹配规则")
	})
	field(c, "安装参数（可选，留空时 .msi 用 /passive，.exe 以交互方式安装）", func() {
		ui.TextInput(c, &e.args).Placeholder("例如 /S").Label("安装参数")
	})
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		if ui.Button(c, "检查").Disabled(e.ghBusy || e.repo == "").Clicked() {
			v.checkGitHub()
		}
		if e.ghBusy {
			ui.Spinner(c).Label("检查中")
		}
		if e.ghText != "" {
			ui.Text(c, e.ghText).TextColor(c.Theme().Accent)
		}
	})
	errorText(c, e.ghErr)
}

func (v *View) checkGitHub() {
	e := &v.editor
	a := v.draft()
	if err := catalog.ValidateApp(withRule(a)); err != nil {
		e.ghText, e.ghErr = "", err.Error()
		return
	}
	e.ghGen++
	gen := e.ghGen
	e.ghBusy, e.ghText, e.ghErr = true, "", ""
	v.core.DescribeGitHub(a, func(rel github.Release, asset github.Asset, err error) {
		if gen != e.ghGen {
			return
		}
		e.ghBusy = false
		if err != nil {
			e.ghErr = err.Error()
			return
		}
		e.ghText = "将安装 " + asset.Name + "（" + rel.Tag + "）"
	})
}

// withRule gives an app a placeholder rule, to check its other fields
// before the user wrote one.
func withRule(a catalog.App) catalog.App {
	if a.Detect == nil {
		a.Detect = &catalog.Detect{ExePath: "x"}
	}
	return a
}

func (v *View) ruleFields(c *ui.Context) {
	e := &v.editor
	required := sourceTabs[e.srcIdx] == catalog.GitHub
	title := "检测规则（可选）"
	if required {
		title = "检测规则（必填）"
	}
	ui.Column(c).Gap(8).Children(func() {
		ui.Text(c, title).Bold()
		muted(c, "用来识别本机已有的软件，避免重复安装。两项填一项即可，任一项命中就算已安装。").FontSize(12)
		field(c, "“已安装的应用”里的名称（正则表达式，不区分大小写）", func() {
			ui.TextInput(c, &e.ruleName).Placeholder("例如 ^Visual Studio Code").Label("注册表名称")
		})
		field(c, "可执行文件路径（可以使用 %ProgramFiles% 这类环境变量）", func() {
			ui.TextInput(c, &e.ruleExe).Placeholder(`例如 %LOCALAPPDATA%\Programs\App\App.exe`).Label("可执行文件路径")
		})
	})
}
