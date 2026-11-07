# 实施报告：Web 前端、本机已装与发现软件

状态：`[partial]`：0.2.0 已实现并通过自动检查，用户验收与真实安装、更新测试待完成。

发布入口：[v0.2.0](https://github.com/Zker67/appshelf/releases/tag/v0.2.0)；发布使用现有私钥签名的 Windows 更新包，签名私钥不入库。

用户在 2026-11-07 确认全部假设，并决定“本机已装”页只显示“可更新”标记、不直接更新清单外的软件；实施中用户提供了应用图标，界面配色随图标调整。

## 实际改动

| 范围 | 改动 |
|---|---|
| 应用图标 | `resources/icon.png` 换成用户提供的图标（深钴蓝圆角方形，外部透明、边缘抗锯齿）；前端的 `src/assets/logo.png` 和 `public/favicon.png` 用同一图标 |
| Go 服务层 | 新增 `internal/api`：六个绑定服务（`Shelf`、`Tasks`、`Sources`、`Discover`、`Installed`、`About`）、一个 `changed` 事件、循环 goroutine 和通知器；`settings.json` 保存外观 |
| 本机已装 | 新增 `internal/inventory`：合并 winget、scoop、注册表和应用商店应用，与清单对照 |
| 发现 | 新增 `internal/discover`：搜索排序、软件详情（winget-pkgs 清单、`scoop info`、GitHub 仓库与 release，带缓存）、GitHub 检测规则建议；`internal/detect/suggest.go` 由程序名或仓库名生成规则 |
| 图标 | 新增 `internal/icons`：UniGetUI 图标库、注册表图标、应用商店应用图标、GitHub 头像，下载限制和缓存，`icon` 协议 |
| 来源 | winget：`Installed`、`LatestVersion`；scoop：`Installed`、`Info`；GitHub：`SearchRepos`、`SearchRate`、`Repository`；只用于展示 |
| core | 新增 `AddApps`（逐项校验、一次保存）、`DeleteApps`、`EditApps`（批量改分类和标签，ID 和来源不可改） |
| main.go | 改为 Web 页面窗口（自绘标题栏，高 44）；绑定服务、注册图标协议、接入 `updater.Plugin`；页面右键菜单只在文字和输入框上出现，去掉后退、刷新等浏览器项 |
| 前端 | 新增 `frontend/`：React 19、TypeScript 7、Tailwind CSS 4、shadcn/ui（radix-nova）、TanStack Query、Vite+、Bun；三个页面、六个对话框、页面图标动画、亮暗两套配色 |
| 删除 | `internal/view`（0.1.0 的原生界面）；`go mod tidy` 去掉了原生 UI 的依赖 |
| 文档 | `README.md`、`AGENTS.md`、`CHANGELOG.md`（0.2.0）、`docs/api.md`、`docs/operations.md`、`docs/architecture/project-structure.md`、`internal/README.md`、`internal/source/README.md`；新增 `internal/api`、`internal/inventory`、`internal/discover`、`internal/icons`、`frontend/` 的 README |
| 版本 | `mygo.json` 和 `frontend/package.json` 为 0.2.0 |

## 验证结果（2026-11-07）

| 检查 | 结果 |
|---|---|
| `gofmt -l .` | 无输出 |
| `go vet ./...`、`go tool mygo vet` | 通过 |
| `CGO_ENABLED=0 go test ./...` | 全部通过；含真实只读测试：本机已装 318 项（本机程序 70、应用商店 101、winget 147），VS Code 的 winget 详情，图标库 5945 个图标 |
| `bun run --cwd frontend check` | 格式、lint、类型检查全部通过 |
| `bun run --cwd frontend test` | 12 项通过（筛选并集、未分类、选择裁剪、状态文字、文字匹配、时间格式等） |
| `go tool mygo build` | 生成 `build/windows-amd64/AppShelf.exe` 和 `appshelf-setup-0.2.0-amd64.exe`（未带签名私钥，不能作为更新发布） |
| 启动检查（不截图） | 构建出的程序启动后持续运行；通过 WebView2 调试端口只读页面文字和控制台：页面正常渲染，启动时没有脚本异常；在“发现”搜索 `7zip` 得到 winget 的 11 个结果，7-Zip 排第一；本机没有 scoop，scoop 标为不可用 |

界面效果没有截图自验，由用户在真实窗口里检查，见 [用户测试清单](./06-design-and-acceptance.md#用户测试清单)。

## 偏离原计划的地方

| 计划 | 实际 | 原因 |
|---|---|---|
| 一次 `Get-WinGetPackage` 全量查询同时用于状态和本机已装（03） | 按 03 的回退方案分开查询：状态只查清单里的 ID；本机已装单独查询并缓存，启动时与状态刷新并行，刷新结束后 1 分钟内复用 | 不改动已测试的状态查询；本机已装只在需要时读取 |
| 用 `GetPackageVersionInfo` 补发布者和 ProductCodes（03）；搜索结果显示发布者（04） | 不调用；winget 项按名称（版本相同的优先）关联注册表项取发布者和图标，`ARP\` 项按键名关联；搜索结果不显示发布者，详情里有 | 147 项要多花约 1.6 秒，名称关联已足够展示 |
| 检测规则“去掉版本号和架构” | 版本号换成 `v?\d[\w.+-]*`，其余原样转义，整体 `^…$` 锚定，例如 `7-Zip 23.01 (x64)` → `^7-Zip v?\d[\w.+-]* \(x64\)$`；按仓库名的规则只锚定开头 | 保留架构更精确，版本变化后仍能命中 |
| 本机图标用 `ExtractIconExW`（05） | 用 MyGo 的 `Shell.FileIcon` 绘制；应用商店应用用开始菜单里应用的图标 | MyGo 已提供，免去自己调用 Win32 |
| 下载时检查 `Content-Type`（05） | 按文件内容识别格式 | 不依赖服务器声明的类型，按内容判断更可靠 |
| 四个事件 `shelf:changed` 等，100 毫秒合并（02） | 一个 `changed` 事件带 `kinds`，80 毫秒合并 | 一次操作常同时改几类数据，合并成一次更省 |
| `Catalog`、`Discover.Add`、`Installed.Add` 等服务（02、03、04） | 导入导出并入 `Shelf`；加入清单统一为 `Shelf.AddPackages`；`Tasks.Enqueue` 叫 `Start`，`Items` 叫 `List`；`Sources.Availability`、`GitHubRate` 合为 `Overview` | 同一件事一个入口 |
| 删除软件 | 一步删除，提示里可撤销（`Shelf.UndoDelete`），不弹确认框 | 遵循用户的界面约定：能撤销的删除不层层确认 |
| 只能逐个改分类和标签 | 多选后批量设分类、加或去标签（`core.EditApps`） | 遵循用户的界面约定 |
| 左侧固定导航（06） | 页面切换放在自绘标题栏里（我的清单、发现、本机已装），任务、使用说明、设置、外观在标题栏右侧；设置和使用说明是弹窗，任务是从右侧滑出的面板 | 用户的界面约定：只有一层自绘顶栏、写中文名，设置做成弹窗，使用说明单独放右上角 |
| 顶部全局搜索框（04） | 每页各有自己的搜索框；清单里搜不到时给出“在发现中搜索” | 避免标题栏和页面各有一个搜索框 |
| “只看未加入清单”开关、来源筛选在顶部（03） | “本机已装”与“我的清单”一样用左侧栏筛选：来源（winget、scoop、本机程序、应用商店）和状态（可更新、未加入清单、已在清单） | 同类筛选放在一处，工具条一行排完 |
| GitHub 搜索按星标排序（04） | 按 GitHub 默认的相关度排序，结果里显示星标数 | 相关度更贴近输入的名称；星标仍可用来判断 |
| 详情统一缓存 7 天（04） | winget 7 天、scoop 1 天、GitHub 1 小时 | GitHub release 更新频繁 |
| 设置页管理分类、标签和导入导出（02） | 分类和标签在“我的清单”侧栏原处新建、重命名、删除；导入导出在侧栏底部和空清单页；设置只剩“安装来源”和“关于” | 用户的界面约定：编辑紧跟对象，常驻工具贴底 |
| 外观只跟随系统 | 标题栏可切换跟随系统、浅色、深色，保存在 `settings.json`；去掉了模板自带的 `next-themes` | 外观由 Go 设置窗口主题，页面跟随 `prefers-color-scheme` |
| 安装预览的“说明”是一段文字 | `PreviewEntry` 改为结构化字段（来源、安装包、release、附注） | 界面分块显示，不用点号串信息 |

## 0.2.0 发布检查（2026-11-07）

- Go：`gofmt -l .` 无输出，`go vet ./...`、`go tool mygo vet`、`CGO_ENABLED=0 go test ./...` 通过。
- 前端：`bun run --cwd frontend check` 通过，12 项单元测试通过。
- 文档：42 份 Markdown 从顶层 README 逐层可达，本地文件链接无失效；计划索引、总览和阶段状态已同步。
- 发布构建使用 `go tool mygo build -o build/release-v0.2.0`，避免覆盖被运行窗口占用的常规构建目录；生成 Windows 安装包、签名更新包、0.1.0 增量更新和校验清单。
- 发布验证覆盖标签对应提交、附件和校验清单；没有执行真实安装或将用户验收标为通过。

## 遗留项

1. 用户验收：按 [用户测试清单](./06-design-and-acceptance.md#用户测试清单) 检查界面和流程，视觉问题记到这里或另开计划。
2. 真实安装与更新：winget、scoop、GitHub 的实际安装和更新、GitHub 安装程序的 UAC 行为仍待指定测试软件后验证。
3. 自更新：从已安装的 0.1.0 升到 0.2.0 的完整流程待用户实测。
4. scoop 仍未在装了 scoop 的机器上实测（本机没有 scoop）。
5. `go test ./...` 会把 `frontend/node_modules` 里依赖自带的一个 Go 包也算进来（目前没有测试文件，不影响结果）。
