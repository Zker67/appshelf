# Web 前端、本机已装与发现软件：总览

状态：`[partial]`：0.2.0 已实现并通过自动检查，用户验收与真实安装、更新测试待完成。实施情况见 [实施报告](./09-implementation-report.md)

## 背景

0.1.0 用 MyGo 原生 UI 实现。用户试用后的结论（2026-11-07）：

- 界面太简陋。
- 加软件太不方便：要先打开编辑框、搜索、选结果、再保存，一次只能加一个。
- 看不到本机已经装了哪些软件。

用户决定：

1. 界面换成 MyGo 的 Web 前端。
2. 在原始需求的基础上，增加 UniGetUI 那样的“识别本机已有软件”。
3. 能方便地搜索软件并加进清单，查不到时才需要手动添加。
4. 不是要学全 UniGetUI。

用户于 2026-11-07 确认全部假设并要求实施。

## 目标

1. **界面**：改用 MyGo Web 页面窗口 + React，做到美观、精致、信息清楚，跟随系统深浅色。
2. **发现软件**：一个搜索框同时搜 winget、scoop、GitHub。结果直接标出“本机已装”和“已在清单”，单个或多选后一步加入清单，同时设好分类和标签，也可以加入后直接进入安装预览。
3. **本机已装**：列出本机全部已装软件，包括 winget 能识别的、scoop 装的、只在注册表里的；标出哪些已在清单，没在清单的可以直接加入。
4. **软件信息**：图标、简介、发布者、主页、许可证、最新版本。
5. **保留 0.1.0 的全部能力**和防误装不变量；手动添加保留，作为查不到时的入口。

## 范围

| 功能 | 说明 | 阶段 |
|---|---|---|
| UniGetUI 功能对照 | 逐项列出采纳、已有、不做 | [01](./01-unigetui-comparison.md) |
| Web 前端骨架与服务层 | 前端工程、Go 绑定服务、事件；把 0.1.0 的全部界面功能迁过来，然后删除原生界面 | [02](./02-web-foundation.md) |
| 本机已装 | 汇总 winget、scoop、注册表三处的已装软件，与清单对照，一键加入 | [03](./03-installed.md) |
| 发现软件 | 统一搜索、软件详情、单个和批量加入、手动添加兜底、GitHub 检测规则自动建议 | [04](./04-discover.md) |
| 图标 | 图标库、注册表图标、GitHub 头像，最后用首字母兜底；Go 端下载并缓存 | [05](./05-icons.md) |
| 视觉打磨与验收 | 统一视觉、空状态、键盘操作；自测和用户测试清单 | [06](./06-design-and-acceptance.md) |

## 非目标

以下仍然不做，后续 AI 不要自行加入：

- 卸载；忽略或跳过某个更新；后台定时检查、通知、托盘；自动更新软件。
- 安装选项（指定版本、架构、安装范围、安装位置）；截图；分享链接；云备份；命令行接口。
- 更多包管理器；msstore 源；离线安装包；多设备同步。
- 对清单外的软件执行安装或更新：本机已装页只负责展示和加入清单（见“用户决定”第 1 项）。

## 当前事实（2026-11-07 核验）

| 主题 | 事实 |
|---|---|
| 可复用的 Go 代码 | `internal/` 下除 `view` 以外的包都与界面无关：`core`、`queue`、`source/*`、`runner`、`detect`、`catalog`、`state`、`version`、`jsonfile`，测试齐全 |
| `core` 的线程模型 | 清单、状态、检查中标记只在“界面线程”上修改，后台结果通过 `Config.Post` 交回；只有来源可用性有自己的锁 |
| MyGo Web 前端（v0.8.1 文档） | `mygo.Bind` 绑定 Go 服务，`mygo generate` 生成带类型的 TypeScript 客户端；`mygo.NewEvent` 从 Go 推送事件；`*mygo.Channel[T]` 流式返回；`mygo.Protocol.Handle` 注册自定义协议。`frontend/package.json` 依赖 `mygo-runtime` 时，客户端生成到 `frontend/src/mygo.ts`。`mygo.json` 支持 `devUrl`、`devCommand`、`buildCommand`、`frontendDist`。Web 前端需要 Bun，Windows 需要 WebView2 |
| 本机工具 | Bun 1.4.4、Node 26.12.1、WebView2 153；没有安装 Vite+（`vp`） |
| npm 最新稳定版（2026-11-07） | vite-plus 1.5.0、vite 8.3.6、typescript 7.0.2、react 19.3.1、tailwindcss 4.3.5、shadcn 4.23.0、lucide-react 1.70.0、@tanstack/react-query 5.107.1、zustand 5.1.0、mygo-runtime 0.8.1 |
| winget 搜索速度 | `Find-WinGetPackage` 本身 0.1–0.2 秒，算上 PowerShell 启动约 0.8 秒 |
| winget 已装 | `Get-WinGetPackage` 本机 318 项，其中 170 项以 `ARP\` 或 `MSIX\` 开头，表示没能关联到任何源 |
| winget 软件详情 | PowerShell 模块的对象只有名称、发布者、ProductCodes、PackageFamilyNames，没有简介、主页和图标；`winget show` 的文本随系统语言变化（本机是中文标签）；winget-pkgs 仓库的清单文件从本机可以读取（每个 0.8–2 秒），大多只有 en-US |
| UniGetUI 的图标库 | `WebBasedData/screenshot-database-v2.json`，1.5 MB，12423 条；图标托管在各个第三方网站。键名规则：winget 包 ID 去掉发布者前缀后转小写、`.` 换成 `-`；scoop 用软件名 |
| UniGetUI 的已装匹配 | 依次按卸载键、UpgradeCode、ProductCode 匹配，最后按名称加发布者一致来匹配 |

## 关键设计决定

| 决定 | 内容 | 理由 |
|---|---|---|
| 前端技术 | React 19 + TypeScript 7 + Tailwind CSS 4 + shadcn/ui（Nova 风格）+ Lucide；数据请求用 TanStack Query | 符合作者的技术偏好；Nova 偏紧凑，适合列表密集的界面 |
| 构建工具 | 优先 Vite+；在 MyGo 的 `devCommand` 下不能正常开发和构建时，退回 Vite 8 | 偏好要求优先评估 Vite+；MyGo 只需要一个开发地址和一个构建目录 |
| 前端只做展示 | 外部访问（来源命令、GitHub、图标下载）和文件读写都在 Go 端；前端只调用绑定方法，并通过 `icon://` 协议显示图标 | 防误装逻辑留在已测试的 Go 代码里；token 不进前端 |
| `core` 的线程 | 不重写成加锁结构：新增一个专用 goroutine 串行执行 `Post` 交回的函数，绑定方法也经它进出，状态变化后广播事件 | 保留已测试的单线程模型，改动最小 |
| 一次 winget 全量查询 | `Get-WinGetPackage` 的全量结果同时用于“清单状态”和“本机已装” | 避免一次刷新跑两遍 |
| 软件详情来源 | winget 读 winget-pkgs 仓库的 YAML 清单，scoop 用 `scoop info`，GitHub 用仓库信息和最新 release；结果缓存 7 天 | 结构化、不受系统语言影响；详情只用于展示，失败只显示“暂时无法读取” |
| 清单格式 | `schemaVersion` 保持 1，不改字段 | 0.1.0 导出的清单可以直接导入，反过来也一样 |

## 不变量

1. 0.1.0 的防误装不变量全部保留：状态不确定就不装；执行前后都复查；精确 ID 并指定来源；清单与本机状态分离；逐个执行；先预览再执行。
2. 搜索结果、详情和图标都只是展示信息。是否已装只由来源查询和检测规则决定，“本机已装”页的匹配结果不参与安装判断。
3. “加入清单”只写清单，不安装；“加入并安装”加入后进入与批量安装相同的预览，确认后才排队。
4. 前端只通过绑定方法使用 Go 提供的能力，拿不到 token；所有外部命令仍由 Go 按参数数组调用。
5. 界面文案简短，不写说明性长句。

## 假设（用户 2026-11-07 确认）

1. **识别已有软件**：新增“本机已装”页，列出本机所有已装软件，可以从这里加入清单。
2. **搜索时机**：输入停顿约 0.4 秒后自动搜 winget 和 scoop（新输入会取消上一次）。GitHub 搜索未登录时每分钟只有 10 次，所以只在按回车或点“搜 GitHub”时执行。
3. **图标来源**：用 UniGetUI 公开的图标库（运行时下载、本地缓存），以及注册表图标和 GitHub 头像。界面不直接加载外部网址。
4. **版本号**：完成后为 0.2.0。发布需要用户另外授权。
5. **删除原生界面**：Web 界面达到 0.1.0 的全部功能后删除 `internal/view`，不保留两套界面。

## 用户决定（2026-11-07）

1. **本机已装页里“可更新”的软件**：只显示“可更新”标记；要更新它，先加入清单。不直接更新清单外的软件。

## 阶段索引

| 阶段 | 文件 | 内容 | 状态 |
|---|---|---|---|
| 01 | [01-unigetui-comparison.md](./01-unigetui-comparison.md) | UniGetUI 功能对照表 | `[done]`（随本计划整理） |
| 02 | [02-web-foundation.md](./02-web-foundation.md) | Web 前端骨架、Go 服务层、迁移 0.1.0 界面功能 | `[partial]`（已实现，待验收） |
| 03 | [03-installed.md](./03-installed.md) | 本机已装 | `[partial]`（已实现，待验收） |
| 04 | [04-discover.md](./04-discover.md) | 统一搜索、详情、快速加入 | `[partial]`（已实现，待验收） |
| 05 | [05-icons.md](./05-icons.md) | 软件图标 | `[partial]`（已实现，待验收） |
| 06 | [06-design-and-acceptance.md](./06-design-and-acceptance.md) | 视觉打磨、自测与用户测试 | `[partial]`（已实现，待验收） |
| 09 | [09-implementation-report.md](./09-implementation-report.md) | 实施报告、发布与遗留项 | `[partial]` |

阶段 02 结束时，Web 版的功能与 0.1.0 一致，可以单独交付；03、04、05 各自增加一块能力，互不阻塞；06 做整体打磨。实施结果、验证和偏离见 [09-implementation-report.md](./09-implementation-report.md)。

## 验证标准

- Go：`gofmt -l .`、`go vet ./...`、`CGO_ENABLED=0 go test ./...` 通过。原有测试保留；新增的包有直接调用本机 winget、注册表和 GitHub API 的只读集成测试。
- 前端：类型检查、lint、`vp test`（或 Vitest）通过；`go tool mygo build` 能构建出带前端的安装包。
- 绑定服务用 Go 测试覆盖：通过 `internal/api` 调用，检查返回值和事件，不依赖窗口。
- 界面效果不由 AI 截图自验，由用户在真实窗口里检查，测试清单见 [06](./06-design-and-acceptance.md)。

## 回滚

- 0.1.0 已发布并打了 tag `v0.1.0`，随时可以 `git checkout v0.1.0` 回到原生界面版本。
- 每个阶段结束时做一次检查点提交，便于单独回退；提交需要用户授权。
- 阶段 02 在删除 `internal/view` 之前，新旧界面并存，切换只改 `main.go`。

## 完成后

更新 `README.md`、`AGENTS.md`（把“MyGo 原生 UI”的不变量改为“MyGo Web 前端，前端只做展示”）、`docs/api.md`（绑定服务和事件）、`docs/architecture/project-structure.md`、`docs/operations.md`（Bun、前端命令），以及 `internal/README.md`；写 `09-implementation-report.md`。
