# 软件备注（Markdown 即写即预览）

状态：`[in-progress]`：实现与自动检查完成后，等待用户在真实窗口验收。

## 用户诉求

- “我的清单”里除了分类和标签，每个软件还能写自己的备注。
- 备注是 Markdown，边写边预览，界面要现代化，配套改动要完善。

## 设计

- **存放位置**：备注是清单的一部分，存在 `catalog.json` 的 `apps[].note`（可省略的字符串，Markdown 原文）。随导入导出、跨设备共享；与本机状态无关，不参与安装和检测判断。
- **兼容**：只新增可省略字段，按 `docs/api.md` 的兼容规则不升级 `schemaVersion`。旧版本读取会忽略备注，但旧版本保存清单会丢掉备注（在更新说明里提示）。
- **限制**：最长 20000 个字，不能含 NUL；保存时统一换行为 `\n`，去掉首尾空行和行尾空白；空备注即删除。
- **修改入口**：只有 `Shelf.SetNote(id, note)` 改备注。`Shelf.SaveApp`（编辑软件弹窗）保留原有备注，新建时可带备注；这样编辑弹窗不会覆盖刚自动保存的备注。改备注不重新读取本机状态。
- **导入合并**：已在清单的软件没有备注时取导入文件里的备注；已有备注时保留自己的，不拼接。
- **渲染**：前端用 `react-markdown` + `remark-gfm`（表格、任务清单、删除线、自动链接）。不渲染原始 HTML；链接经 MyGo 用系统浏览器打开，只放行 `http(s)` 和 `mailto`。

## 界面

1. 详情栏「备注」页签：渲染后的 Markdown；任务清单的复选框可直接勾选并保存；没有备注时显示空状态和“写备注”按钮。
2. 备注编辑弹窗（宽）：左边编辑、右边实时预览，可切换「编辑 / 分栏 / 预览」；工具栏（标题、粗体、斜体、删除线、行内代码、链接、列表、编号、任务、引用、代码块、分隔线）；快捷键 Ctrl+B / Ctrl+I / Ctrl+K / Ctrl+S，Tab 缩进列表，回车自动续列表；停笔约 0.6 秒自动保存，标题栏显示“保存中 / 已保存 / 保存失败”和字数，关闭时立即保存未保存内容。
3. 清单行：有备注时名称旁显示备注图标，悬停预览首行；右键菜单新增“编辑备注”。
4. 搜索：清单搜索框也匹配备注内容。

## 实现清单

- `internal/catalog`：`App.Note`、校验、`normalizeApp` 规范化、`Merge` 补备注；测试。
- `internal/core`：`SaveApp` 保留原备注、`SetNote`；测试。
- `internal/api`：`Shelf.SetNote`；测试；`go tool mygo generate`。
- `frontend/`：`lib/markdown.ts`（编辑操作、任务清单切换，纯函数带测试）、`components/markdown.tsx`（渲染）、`components/dialogs/note-editor.tsx`、`pages/shelf/note.tsx`（页签内容）、`state/actions.tsx`（`editNote`）、`pages/shelf/row.tsx`、`pages/shelf/detail.tsx`、`lib/shelf.ts`。
- 文档：`docs/api.md`、`internal/api/README.md`、`frontend/README.md`、`CHANGELOG.md`、`README.md`、`plans/README.md`。

## 不变量

- 备注只是展示和记录，不影响是否已装、安装、更新的判断。
- 清单与本机状态分离：备注只在 `catalog.json`。
- 前端只经绑定服务读写。

## 验收

- `gofmt -l .`、`go vet ./...`、`go tool mygo vet`、`CGO_ENABLED=0 go test ./...` 通过。
- `bun run --cwd frontend check`、`bun run --cwd frontend test` 通过；`go tool mygo build` 能打包。
- 用户在真实窗口确认：写备注、实时预览、自动保存、勾选任务、行内图标、搜索、导出再导入、亮暗主题。

## 回滚

删除 `App.Note` 及 `SetNote`、前端备注组件和依赖即可；已写入 `catalog.json` 的 `note` 字段会被旧代码忽略。
