# AppShelf

> **AI 助手**：请先阅读 [AGENTS.md](./AGENTS.md)，再开始任何操作。

## 项目简介

AppShelf 是个人软件图书馆：在一份可导入导出的软件清单里，用分类和标签整理自己收藏的 Windows 软件。在“发现”页一次搜索 winget、scoop 和 GitHub，一键加进清单；“本机已装”页列出电脑上已有的程序。它会对照本机已装的软件，按分类或标签多选，再逐个批量安装，之后持续检查更新。

## 目录

- [需求概述](#需求概述)
- [功能特性](#功能特性)
- [使用方法](#使用方法)
- [配置说明](#配置说明)
- [文档结构](#文档结构)
- [开发路线](#开发路线)

## 文档入口

- [下载与版本说明](https://github.com/Zker67/appshelf/releases/tag/v0.3.0) · [更新日志](./CHANGELOG.md)
- [项目文档](./docs/README.md) · [前端](./frontend/README.md) · [Go 业务代码](./internal/README.md)
- [计划与验收](./plans/README.md) · [外部参考](./references/README.md) · [AI 项目规则](./.agents/README.md)

## 需求概述

### 核心需求

以下是实现状态；0.2.0 的用户测试按 [测试清单](./plans/2026-11-07-web-ui-and-discovery/06-design-and-acceptance.md#用户测试清单) 进行。

- [x] 软件清单：添加软件，配置分类和标签（一个软件一个分类、多个标签）
- [x] 导入导出：清单就是一份 JSON 文件，可迁移、可备份；导入时合并
- [x] 本机检测：识别本机已有的软件（无论是否经 AppShelf 安装），已安装的不重复装，也不装错包
- [x] 批量安装：按任意分类或标签多选，预览后逐个安装，每项单独显示结果，失败可单独重试
- [x] 检查更新：winget 和 scoop 用来源自己的更新信息（参考 UniGetUI）；GitHub 软件对比最新 release（参考 GitHub Store）
- [x] 自更新：v0.1.0 已发布到公开仓库 `Zker67/appshelf`，安装版检查新 release 后签名校验并升级；从 0.1.0 升到 0.2.0 的完整流程待实测
- [x] 安装来源：winget、scoop、GitHub Releases；不做离线安装包（scoop 还没有在装了 scoop 的机器上实测）
- [x] 发现软件：winget 和 scoop 边输边搜，GitHub 按需搜索；结果标出“已在清单”“本机已装”，单个或多选一步加入清单并设好分类、标签，也可以加入后直接安装；查不到时手动添加
- [x] 本机已装：列出 winget、scoop、注册表和应用商店里的程序，标出已在清单的和可更新的；有明确 winget / scoop 来源且可更新时可直接预览更新，也可加入清单；没有来源的“查找来源”后带着检测规则加入
- [x] 软件信息：图标、简介、发布者、主页、许可证、最新版本和发行说明

### 技术选型

| 层级 | 技术 | 说明 |
|---|---|---|
| 桌面框架 | [MyGo](https://mygo.egoist.dev/)（`github.com/egoist/mygo`）v0.8.1 Web 页面窗口 | Go 绑定服务给页面调用，Windows 上用 WebView2 显示；版本由 `go.mod` 锁定 |
| 后端 | Go | 版本见 `go.mod`；另外依赖 `golang.org/x/sys`（读注册表、提权启动安装程序）和 `go.yaml.in/yaml/v3`（读 winget 清单文件） |
| 前端 | React 19 + TypeScript 7 + Tailwind CSS 4 + shadcn/ui | 在 `frontend/`，Vite+ 构建，Bun 管理依赖；只做展示，读写都通过 Go 服务 |
| 安装来源 | winget / scoop / GitHub Releases | 调用各自的官方能力，不自己写安装逻辑；winget 状态通过 PowerShell 模块 `Microsoft.WinGet.Client` 读取 |
| 数据存储 | JSON | 软件清单（可导入导出）和本机状态分开存放在 `%AppData%\AppShelf\` |
| 自更新 | MyGo updates | 发布到公开仓库 `Zker67/appshelf` 的 GitHub Release，更新包由私钥签名 |
| 测试 | `go test`、`vp test` | 绑定服务用假来源测试，不需要窗口；来源、本机已装、详情和图标有直接调用本机 winget、注册表、GitHub 的只读集成测试 |

## 功能特性

| 页面 | 功能 |
|---|---|
| 我的清单 | 左侧按状态、分类、标签筛选（分类和标签可多选、取并集，含“未分类”），在原处新建、重命名、删除分类和标签，底部导入、导出清单；列表可搜索、排序、Shift 连选、右键菜单，悬停直接安装或更新；多选后批量安装、更新、设分类、加标签、删除（可撤销）；右侧详情显示来源信息、检测规则和软件介绍 |
| 发现 | 输入即搜 winget 和 scoop，回车或“搜 GitHub”再搜 GitHub；结果标出“已在清单”“本机已装”，右侧看详情；单个或多选“加入清单”“加入并安装”；GitHub 软件自动建议检测规则；查不到就“手动添加” |
| 本机已装 | 按来源（winget、scoop、本机程序、应用商店）和状态（可更新、未加入清单、已在清单）筛选；单项或多选预览更新，无需先加入清单；也可“加入清单”或“查找来源” |
| 安装预览 | 每一项将安装、更新还是跳过及原因；刚加入的软件读到状态后自动更新预览；确认后加入任务 |
| 编辑软件 | 名称、分类、标签；winget、scoop 搜索后选择精确的包；GitHub 填仓库、安装包匹配和安装参数，可“检查最新版本”；检测规则（GitHub 必填） |
| 任务（标题栏） | 逐个执行，显示每项的状态、原因和时间；停止（当前项完成后）、继续、重试失败项、清除已完成、打开日志 |
| 设置（标题栏） | 各来源是否可用（winget 缺模块时可一键安装）、GitHub 请求额度；版本、自更新、数据和日志目录 |
| 使用说明（标题栏） | 分页签的上手步骤、状态说明、检测规则和快捷键 |

标题栏右侧还可以切换外观：跟随系统、浅色、深色。

防误装：状态读不准就不装；队列执行每一项前再查一次，已装的跳过；装完再查一次，查不到就记为失败；GitHub 安装包无法唯一确定时拒绝安装，下载后校验 SHA-256。

## 使用方法

只支持 Windows。需要 Go（版本见 `go.mod`）、Bun、WebView2 和 winget；scoop 可选。

```bash
bun install --cwd frontend      # 第一次运行前安装前端依赖
go tool mygo dev                # 开发模式，前端热更新
CGO_ENABLED=0 go test ./...     # Go 测试
bun run --cwd frontend check    # 前端格式、lint、类型检查
go tool mygo build              # 打包到 build/windows-amd64/：exe 和安装包
```

GitHub 软件较多时，建议设置环境变量 `GH_TOKEN`（或 `GITHUB_TOKEN`），否则每小时只有 60 次 GitHub 请求。完整说明见 [运行与部署说明](./docs/operations.md)。

## 配置说明

### AI 行为配置

- **协作入口**：`AGENTS.md`，AI 首次进入项目时先读取。
- **规则文件**：`.agents/rules/*.md`，使用 YAML frontmatter + Markdown。
- **MyGo 维护 Skill**：`.agents/skills/mygo-maintenance/`，由 `mygo init` 生成，用于升级 MyGo 版本。
- **目录说明**：`.agents/README.md`。

### 应用配置

- `mygo.json`：应用名、标识符、版本号、输出目录，以及自更新的公钥和 GitHub 仓库。
- `CHANGELOG.md`：每个版本的更新说明，显示在自更新窗口里。
- 环境变量 `GH_TOKEN` / `GITHUB_TOKEN`：可选的 GitHub token，只从环境变量读取。

### 项目文档

`docs/` 承载当前代码和程序的长期文档。同一类信息的主维护位置以 [单一信息源说明](./docs/architecture/source-of-truth.md) 为准。数据格式见 [API 与接口说明](./docs/api.md)。

### 外部参考

`references/` 记录 UniGetUI、GitHub Store、winget、scoop 等外部资料和借鉴点，见 [外部参考索引](./references/README.md)。

## 文档结构

```text
appshelf/
├── AGENTS.md                 # AI 协作入口
├── README.md                 # 项目说明
├── CHANGELOG.md              # 更新说明（自更新窗口显示）
├── main.go                   # 应用入口：组装来源、核心、服务和窗口
├── internal/                 # Go 代码：绑定服务、核心、队列、来源、本机已装、发现、图标、清单等
├── frontend/                 # Web 前端（React），见 frontend/README.md
├── go.mod / go.sum           # Go 模块，锁定 MyGo 版本和 CLI 工具
├── mygo.json                 # MyGo 应用配置
├── resources/                # 应用图标等资源
├── docs/                     # 当前代码和程序的长期文档
├── references/               # 外部文档和外部仓库参考
├── plans/                    # 计划、设计和改进追踪
└── .agents/                  # AI 项目规则与 Skill
```

详细职责见 [项目结构总览](./docs/architecture/project-structure.md)、[internal/README.md](./internal/README.md) 和 [frontend/README.md](./frontend/README.md)。

## 开发路线

[基本实现计划](./plans/2026-11-07-basic-implementation/00-overview.md) 的 7 个阶段已经实现并发布 v0.1.0，实施情况见 [实施报告](./plans/2026-11-07-basic-implementation/09-implementation-report.md)。

[Web 前端、本机已装与发现软件](./plans/2026-11-07-web-ui-and-discovery/00-overview.md) 已实现为 [0.2.0](https://github.com/Zker67/appshelf/releases/tag/v0.2.0)（用户验收与真实安装测试待完成）：界面改为 Web 前端，新增“发现”和“本机已装”页，实施情况见 [实施报告](./plans/2026-11-07-web-ui-and-discovery/09-implementation-report.md)。

[图标优化](./plans/2026-11-07-installed-icons/00-overview.md) 改为本机图标优先，补齐多种系统图标来源并优化缓存和重试；验证范围和回退限制见计划。

[软件图书馆与本机直接更新](./plans/2026-11-07-library-and-installed-updates/00-overview.md)：本机已装软件可直接单项或多选更新，无需先加入清单；切页时保留已加载图标状态。
