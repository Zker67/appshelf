# frontend

AppShelf 的界面：MyGo 的 Web 页面窗口里运行的 React 应用。它只做展示和交互，读写都通过 Go 绑定的服务（`src/mygo.ts`，由 `go tool mygo generate` 生成）；是否已装、能不能装、怎么装都由 Go 端决定。

## 技术栈

| 层         | 技术                                                                                         |
| ---------- | -------------------------------------------------------------------------------------------- |
| 框架       | React 19、TypeScript 7                                                                       |
| 样式与组件 | Tailwind CSS 4、shadcn/ui（radix-nova）、lucide-react；分栏用 react-resizable-panels         |
| 数据       | TanStack Query：数据永不过期，收到 Go 的 `changed` 事件后按种类重新读取                      |
| 提示       | sonner，固定在顶部正中、标题栏下方                                                           |
| Markdown   | react-markdown + remark-gfm 渲染备注：不渲染原始 HTML，链接只放行 http(s) 并用系统浏览器打开 |
| 构建与检查 | Vite+（`vp`）：开发服务器、构建、格式化、lint、类型检查和测试；包管理用 Bun                  |

## 命令

在仓库根目录运行（也可以进入 `frontend/` 去掉 `--cwd frontend`）：

```bash
bun install --cwd frontend      # 安装依赖
bun run --cwd frontend dev      # 开发服务器 http://localhost:6173（go tool mygo dev 会自动启动）
bun run --cwd frontend check    # 格式、lint、类型检查
bun run --cwd frontend fmt      # 整理格式
bun run --cwd frontend test     # 单元测试
bun run --cwd frontend build    # 构建到 dist/（go tool mygo build 会自动运行）
```

## 目录

| 路径                                                 | 内容                                                                                                                                                                                                                                                                                                                                                                                                  |
| ---------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [src/main.tsx](./src/main.tsx)、`src/App.tsx`        | 入口：主题同步、查询客户端、提示；标题栏和三个页面（第一次打开时挂载，之后保留状态）                                                                                                                                                                                                                                                                                                                  |
| [src/mygo.ts](./src/mygo.ts)                         | 生成的 Go 服务客户端和类型，不要手改                                                                                                                                                                                                                                                                                                                                                                  |
| [src/pages/shelf/](./src/pages/shelf/)               | 我的清单：侧栏筛选（状态、分类、标签，就地新建、重命名、删除）、导入导出、列表、右键菜单、批量操作、详情（含「备注」页签 `note.tsx`）                                                                                                                                                                                                                                                                 |
| [src/pages/discover/](./src/pages/discover/)         | 发现：winget 和 scoop 边输边搜，GitHub 按需搜索，按搜索框旁“加入到”的分类和标签一键加入清单，winget 与 scoop 同名结果合并、查看详情、手动添加入口                                                                                                                                                                                                                                                     |
| [src/pages/installed/](./src/pages/installed/)       | 本机已装：按来源、更新、清单三组组合筛选，按名称、占用空间、安装日期排序（`installed.sort` 存在 localStorage），加入清单、查找来源、设置更新方式、卸载；只有注册表结果时显示“正在补充来源”                                                                                                                                                                                                            |
| [src/pages/updates/](./src/pages/updates/)           | 更新：可更新、自动更新、已忽略、最近更新四个视图；AppShelf 自身新版本卡片、全部更新、每个软件的更新方式菜单（`components/update-menu.tsx`），侧栏显示自动检查的设置                                                                                                                                                                                                                                   |
| [src/components/dialogs/](./src/components/dialogs/) | 加入清单、安装预览、编辑软件、备注编辑（`note-editor.tsx`：编辑 / 分栏 / 预览、工具栏、自动保存）、任务面板（`tasks-sheet.tsx`：阶段、进度、用时、实时日志）、卸载预览（`uninstall-dialog.tsx`）、残留清理（`leftovers-dialog.tsx`）、设置、使用说明                                                                                                                                                  |
| [src/components/](./src/components/)                 | 标题栏、页面左侧栏（筛选行和底部的设置入口）、页面图标、软件图标、来源和状态标记、分类和标签选择、“加入到”选择（`add-target.tsx`）、行尾的加入和已在清单按钮（`row-actions.tsx`）、详情栏、软件详情、Markdown 渲染（`markdown.tsx`）等共用组件                                                                                                                                                        |
| [src/components/ui/](./src/components/ui/)           | shadcn 生成的基础组件，不参与格式和 lint 检查                                                                                                                                                                                                                                                                                                                                                         |
| [src/lib/](./src/lib/)                               | 查询与事件（`api.ts`）、筛选和排序（`shelf.ts`）、文字、时间和字节大小（`text.ts`）、任务的进度和日志行分类（`tasks.ts`）与日志的增量读取（`use-task-log.ts`）、多选（`selection.ts`、`use-list.ts`）、主题（`theme.ts`）、默认加入位置（`add-target.ts`）、更新的文字、本机已装的筛选组和排序（`updates.ts`）、winget 与 scoop 同名结果配对（`twins.ts`）、备注的 Markdown 编辑操作（`markdown.ts`） |
| [src/state/](./src/state/)                           | 页面切换和页面之间的请求（`nav.tsx`），各页共用的对话框（`actions.tsx`），一键加入（`quick-add.ts`）                                                                                                                                                                                                                                                                                                  |

## 约定

图标切页缓存的浏览器回归见 [tests/README.md](./tests/README.md)。

- 外观是「冰川蓝 · 轻玻璃」，跟随 Go 设置的窗口主题（`prefers-color-scheme`），亮暗两套颜色都在 `src/index.css`：主色冰川蓝，品牌渐变冰川蓝→冰青。窗口底是缓慢漂移的冰雾（`App.tsx` 的 `.ice-fog`，弹窗打开时和减少动效时静止）；顶栏透明，各页的侧栏、列表、详情是浮在雾上的半透明玻璃面板（`glass-panel`，列表和详情加 `glass-strong` 更密），面板之间的缝就是拖拽线；浮在内容上的菜单、弹窗、Toast、批量操作条用 `glass-float`（带背景模糊），分段切换用 `seg-track` / `seg-thumb`；弹窗和侧滑面板为不透明底；工具条按钮文字用 `WideLabel`，列表变窄时收成图标；列表行用 `list-row`（内缩分割线和悬停），聚焦行和侧栏选中项是同一张浮起的冰面卡片。颜色只用 token（如 `bg-hover`、`bg-track`、`bg-field`），不在组件里直接写色值。系统要求降低透明度（`prefers-reduced-transparency`）时玻璃变为接近不透明。动效（切页按页签方向滑入、列表级联入场、页签图标动画等）也定义在 `src/index.css`；生成的 `components/ui/` 组件通过 `data-slot` 选择器换装，不改其结构。
- 字体：界面用随程序打包的更纱黑体 UI 简体（`Sarasa UI SC`，常规、半粗、粗体三个字重的子集，`font-medium` 用半粗），来源、子集范围和许可证见 [public/fonts/README.md](./public/fonts/README.md)；子集外的字依次退回 Segoe UI Variable、Noto Sans SC、微软雅黑 UI。版本号用普通字体加 `tabular-nums` 等宽数字；软件 ID、文件名、安装参数等标识符用 Cascadia Mono。正文保持比例数字（`body` 的 `"tnum" 0`），`.tabular-nums` 单独打开等宽数字。
- 图标只走 Go 的 `icon` 协议（`About.info().iconBase` 加路径，路径格式见 `docs/api.md`），页面不加载外部网址；链接用 `About.openURL` 在浏览器中打开。
- 文案用简体中文，简短；时间写成“年-月-日 时:分”（24 小时制）。
- 改了 Go 服务或类型后先运行 `go tool mygo generate`，再运行 `check`。
