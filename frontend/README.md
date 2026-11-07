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

| 路径                                                 | 内容                                                                                                                                                                                                                                                         |
| ---------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| [src/main.tsx](./src/main.tsx)、`src/App.tsx`        | 入口：主题同步、查询客户端、提示；标题栏和三个页面（第一次打开时挂载，之后保留状态）                                                                                                                                                                         |
| [src/mygo.ts](./src/mygo.ts)                         | 生成的 Go 服务客户端和类型，不要手改                                                                                                                                                                                                                         |
| [src/pages/shelf/](./src/pages/shelf/)               | 我的清单：侧栏筛选（状态、分类、标签，就地新建、重命名、删除）、导入导出、列表、右键菜单、批量操作、详情（含「备注」页签 `note.tsx`）                                                                                                                        |
| [src/pages/discover/](./src/pages/discover/)         | 发现：winget 和 scoop 边输边搜，GitHub 按需搜索，按搜索框旁“加入到”的分类和标签一键加入清单，winget 与 scoop 同名结果合并、查看详情、手动添加入口                                                                                                            |
| [src/pages/installed/](./src/pages/installed/)       | 本机已装：按来源和状态筛选，加入清单、查找来源                                                                                                                                                                                                               |
| [src/components/dialogs/](./src/components/dialogs/) | 加入清单、安装预览、编辑软件、备注编辑（`note-editor.tsx`：编辑 / 分栏 / 预览、工具栏、自动保存）、任务面板、设置、使用说明                                                                                                                                  |
| [src/components/](./src/components/)                 | 标题栏、页面左侧栏（筛选行和底部的设置入口）、页面图标、软件图标、来源和状态标记、分类和标签选择、“加入到”选择（`add-target.tsx`）、行尾的加入和已在清单按钮（`row-actions.tsx`）、详情栏、软件详情、Markdown 渲染（`markdown.tsx`）等共用组件               |
| [src/components/ui/](./src/components/ui/)           | shadcn 生成的基础组件，不参与格式和 lint 检查                                                                                                                                                                                                                |
| [src/lib/](./src/lib/)                               | 查询与事件（`api.ts`）、筛选和排序（`shelf.ts`）、文字和时间（`text.ts`）、多选（`selection.ts`、`use-list.ts`）、主题（`theme.ts`）、默认加入位置（`add-target.ts`）、winget 与 scoop 同名结果配对（`twins.ts`）、备注的 Markdown 编辑操作（`markdown.ts`） |
| [src/state/](./src/state/)                           | 页面切换和页面之间的请求（`nav.tsx`），各页共用的对话框（`actions.tsx`），一键加入（`quick-add.ts`）                                                                                                                                                         |

## 约定

图标切页缓存的浏览器回归见 [tests/README.md](./tests/README.md)。

- 外观跟随 Go 设置的窗口主题（`prefers-color-scheme`），亮暗两套颜色都在 `src/index.css`，主色取自应用图标的深钴蓝。品牌渐变（钴蓝→天蓝）、玻璃质感（半透明侧栏和顶栏、菜单和弹窗的背景模糊）和动效（切页按页签方向滑入、列表级联入场等）也都定义在 `src/index.css`；生成的 `components/ui/` 组件通过 `data-slot` 选择器换装，不改其结构。
- 图标只走 Go 的 `icon` 协议（`About.info().iconBase` 加路径，路径格式见 `docs/api.md`），页面不加载外部网址；链接用 `About.openURL` 在浏览器中打开。
- 文案用简体中文，简短；时间写成“年-月-日 时:分”（24 小时制）。
- 改了 Go 服务或类型后先运行 `go tool mygo generate`，再运行 `check`。
