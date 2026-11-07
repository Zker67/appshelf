# 阶段 02：Web 前端骨架、Go 服务层、迁移 0.1.0 界面功能

状态：`[partial]`：已实现，用户验收与运行验证边界见 [实施报告](./09-implementation-report.md)。

## 目标

把界面从 MyGo 原生 UI 换成 MyGo Web 页面窗口。本阶段结束时：

- Web 版具备 0.1.0 的全部功能：清单、分类和标签筛选、安装和更新预览、任务、设置、导入导出、自更新；
- `internal/view` 已删除；
- 之后的阶段只增加页面和服务，不再动骨架。

## 技术选型

| 层 | 选择 | 理由 | 实施时核验 |
|---|---|---|---|
| 桌面容器 | MyGo v0.8.1 Web 页面窗口，与现在同一个 Go 模块 | Go 端代码全部复用 | 官方 Web 模板的目录和配置 |
| 包管理与脚本 | Bun | MyGo 的 Web 前端要求 Bun | `devCommand`、`buildCommand` 在 Windows 上能运行 |
| 前端基础 | React 19 + TypeScript 7 + Tailwind CSS 4 | 作者偏好 | 版本互相兼容，锁定版本 |
| 构建 | Vite+（`vite-plus`，作为项目依赖，不全局安装） | 作者偏好：Vite 体系项目优先 | 在 MyGo 的 `devCommand` 下能热更新，`build` 输出能被 `frontendDist` 嵌入；不行就用 Vite 8 |
| 组件 | shadcn/ui（Nova 风格）+ Lucide 图标 | 作者偏好；组件源码在项目里，可改 | 用 shadcn CLI 初始化，选 Nova 风格 |
| 调用 Go | `mygo generate` 生成的客户端 + TanStack Query | 缓存调用结果；收到事件时让相关查询失效重取 | — |
| 共享状态 | 先用 React 局部状态；跨页面的选择确实需要共享时再加 Zustand | 不提前引入依赖 | — |
| 页面切换 | 侧边栏切换，不引入路由库 | 桌面应用不需要网址 | — |

## 目录

```text
appshelf/
├── main.go                 # 改为 Web 页面窗口；绑定服务；注册 icon 协议（阶段 05）
├── mygo.json               # 增加 devUrl、devCommand、buildCommand、frontendDist
├── internal/api/           # 新增：绑定给前端的服务和事件
├── internal/view/          # 本阶段结束时删除
└── frontend/
    ├── package.json        # 依赖 mygo-runtime、react、tailwindcss 等
    ├── vite.config.ts
    ├── index.html
    ├── components.json     # shadcn 配置
    └── src/
        ├── main.tsx
        ├── mygo.ts         # 生成的客户端，不手改；是否入库按官方模板的 .gitignore
        ├── app/            # 外框：侧边导航、页面切换、全局提示
        ├── pages/          # shelf、tasks、settings；后续加 discover、installed
        ├── components/     # 业务组件：状态标签、软件行、加入清单弹层等
        ├── components/ui/  # shadcn 组件
        └── lib/            # 查询键、事件订阅、格式化
```

`.gitignore` 增加 `frontend/node_modules/`、`frontend/dist/`。

## Go 服务层（`internal/api`）

### 线程模型

`core` 现在假定所有状态都在“界面线程”上修改，后台结果通过 `Config.Post` 交回。Web 版的绑定方法会在各自的 goroutine 上并发运行，所以：

1. `internal/api` 启动一个专用 goroutine，按顺序执行 `Post` 交回的函数，作为原来的“界面线程”。
2. 每个绑定方法把对 `core` 的读写包成一个函数交给它执行，并等待结果。
3. 函数执行后，按变化的类别广播事件，100 毫秒内的多次变化合并成一次。

`core` 本身不改线程模型，现有测试继续有效。

### 服务

| 服务 | 方法 | 对应 0.1.0 |
|---|---|---|
| `Shelf` | `Snapshot()`：清单、各软件状态、是否在刷新、上次刷新时间、打开时的提示；`SaveApp`、`DeleteApp`；分类和标签的增删改；`Refresh()` | 软件页、编辑框、设置页的分类和标签 |
| `Catalog` | `Export(ctx)`：弹出保存对话框；`ReadImport(ctx)`：弹出打开对话框，返回合并预览；`ApplyImport(id)` | 设置页的导入导出 |
| `Tasks` | `Preview(ids, action)`、`Enqueue(ids, action)`、`Items()`、`Stop()`、`Resume()`、`Retry(itemID)`、`ClearDone()`、`OpenLog(itemID)` | 预览框、任务页 |
| `Sources` | `Availability()`、`InstallModule(ctx)`、`GitHubRate()`、`DescribeGitHub(repo, pattern)`、`Search(ctx, source, query)` | 设置页的来源、编辑框的搜索和 GitHub“检查” |
| `About` | `Info()`：版本、能否自更新、日志目录；`CheckForUpdates()`；`OpenLogs()` | 设置页的关于 |

- 文件对话框用 `mygo.CallerWindow(ctx)` 作为父窗口。
- 返回给前端的类型都带 `json` 标签；状态的显示文字由前端格式化，Go 只返回原始字段（状态、版本、最新版本、附注、来源外标记）。
- 错误直接返回中文原因，前端显示 `CallError.message`。

### 事件

| 事件 | 何时广播 | 前端动作 |
|---|---|---|
| `shelf:changed` | 清单、分类、标签变化 | 重取 `Snapshot` |
| `statuses:changed` | 状态刷新完成、单项复查完成 | 重取 `Snapshot` |
| `tasks:changed` | 队列项的状态变化 | 重取 `Items` |
| `sources:changed` | 来源可用性、GitHub 额度变化 | 重取 `Availability` |

事件只带一个递增的版本号，前端据此重取数据，不在事件里传大对象。

### 自更新

主窗口变成 Web 页面后，更新器改用 `plugins/updater` 的 Web 版（`updater.Plugin`），不再用 `native.Plugin`。“检查 AppShelf 更新”调用 `About.CheckForUpdates()`；开发版仍然不显示这个按钮。

## 界面（与 0.1.0 功能一致）

| 页面 | 内容 |
|---|---|
| 外框 | 左侧导航：我的清单、任务（显示进行中的数量）、设置；后续阶段加“发现”“本机已装” |
| 我的清单 | 左栏按分类、标签多选筛选（取并集，含“未分类”）；顶部按名称过滤、“检查更新”、“添加软件”；列表显示名称、分类、标签、来源、状态，可排序、多选、全选当前结果；选中后底部出现“安装”“更新”；点一行打开右侧详情面板，可以编辑、删除、单独安装或更新 |
| 安装预览 | 每项显示来源与精确 ID、将执行什么（安装、更新、跳过及原因）；“开始”后切到任务页 |
| 编辑软件 | 与 0.1.0 字段相同：名称、分类、标签、来源（winget、scoop、GitHub）、来源字段、检测规则；winget、scoop 在框内搜索并选择 |
| 任务 | 每项的状态、原因、开始和结束时间；停止、继续、清除已完成、打开日志、重试 |
| 设置 | 来源状态（winget 缺模块时“安装模块”）、GitHub token 和额度；分类、标签管理；导出、导入清单；版本、检查更新、日志目录 |

页面风格在本阶段只做到整洁、一致，统一的视觉打磨放在阶段 06。

## 步骤

1. **核验**（不交付）：在仓库外用官方 Web 模板（`bunx mygo-cli init`）建一个临时项目，确认 `package.json` 脚本、Vite 配置、`mygo.json` 前端字段和生成客户端的位置；在临时项目里换成 Vite+、Tailwind 4、shadcn（Nova），确认 `go tool mygo dev` 热更新和 `go tool mygo build` 都正常。结果写进本文件的“核验结果”。
2. 更新 `AGENTS.md` 的不变量：“MyGo 原生 UI”改为“MyGo Web 前端；前端只做展示，外部访问和防误装逻辑都在 Go 端”。
3. 建 `frontend/` 工程，配置 `mygo.json`，接通一个最小页面和一个绑定方法。
4. 写 `internal/api`：线程模型、五个服务、四个事件；Go 测试覆盖。
5. 按上表逐页实现；`main.go` 切换到 Web 窗口和 `updater.Plugin`。
6. 对照 0.1.0 的 [用户测试清单](../2026-11-07-basic-implementation/07-acceptance.md) 自查功能一致后，删除 `internal/view`，以及 `main.go` 中原生 UI 的代码和依赖。
7. 更新 `docs/operations.md`（Bun、前端命令）、`docs/architecture/project-structure.md`、`internal/README.md`。

## 成功标准

- `internal/api` 的 Go 测试：用假来源调用各服务，检查返回值；修改清单、刷新状态、队列推进时广播对应事件，并且会合并。
- 前端：类型检查、lint、`vp test` 通过；`lib/` 中的筛选（并集、未分类）、选择裁剪、状态格式化有单元测试。
- `go tool mygo build` 生成的安装包包含前端，安装后能启动。
- 0.1.0 用户测试清单的 1–14 项在 Web 版上都能完成；界面效果由用户检查。

## 失败与回滚

- 核验中如果 Vite+ 与 MyGo 的开发模式不兼容，改用 Vite 8，并在本文件记录原因。
- MyGo Web 页面窗口在本机出现阻断问题时（例如 WebView2 无法加载），停下来向用户说明，原生界面保持不动。
- 删除 `internal/view` 之前，切换回原生界面只需还原 `main.go`。

## 核验结果

（执行时填写）
