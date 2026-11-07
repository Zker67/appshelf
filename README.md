# AppShelf

> **AI 助手**：请先阅读 [AGENTS.md](./AGENTS.md)，再开始任何操作。

## 项目简介

AppShelf 是个人软件商城：在一份可导入导出的软件清单里，用分类和标签整理自己收藏的 Windows 软件。它会对照本机已装的软件，按分类或标签多选，再逐个批量安装，之后持续检查更新。

## 目录

- [需求概述](#需求概述)
- [功能特性](#功能特性)
- [使用方法](#使用方法)
- [配置说明](#配置说明)
- [文档结构](#文档结构)
- [开发路线](#开发路线)

## 需求概述

### 核心需求

以下是实现状态；用户实际测试按 [测试清单](./plans/2026-11-07-basic-implementation/07-acceptance.md) 进行。

- [x] 软件清单：添加软件，配置分类和标签（一个软件一个分类、多个标签）
- [x] 导入导出：清单就是一份 JSON 文件，可迁移、可备份；导入时合并
- [x] 本机检测：识别本机已有的软件（无论是否经 AppShelf 安装），已安装的不重复装，也不装错包
- [x] 批量安装：按任意分类或标签多选，预览后逐个安装，每项单独显示结果，失败可单独重试
- [x] 检查更新：winget 和 scoop 用来源自己的更新信息（参考 UniGetUI）；GitHub 软件对比最新 release（参考 GitHub Store）
- [ ] 自更新：代码和签名构建已完成；要在公开仓库 `Zker67/appshelf` 发布第一个 release 后才能使用
- [x] 安装来源：winget、scoop、GitHub Releases；不做离线安装包（scoop 还没有在装了 scoop 的机器上实测）

### 技术选型

| 层级 | 技术 | 说明 |
|---|---|---|
| 桌面框架 | [MyGo](https://mygo.egoist.dev/)（`github.com/egoist/mygo`）v0.8.1 原生 UI | 界面用 Go 编写，Windows 上用 Direct3D 11 绘制，不启动 WebView；版本由 `go.mod` 锁定 |
| 语言 | Go | 版本见 `go.mod`；另外只依赖 `golang.org/x/sys`（读注册表、提权启动安装程序） |
| 安装来源 | winget / scoop / GitHub Releases | 调用各自的官方能力，不自己写安装逻辑；winget 状态通过 PowerShell 模块 `Microsoft.WinGet.Client` 读取 |
| 数据存储 | JSON | 软件清单（可导入导出）和本机状态分开存放在 `%AppData%\AppShelf\` |
| 自更新 | MyGo updates | 发布到公开仓库 `Zker67/appshelf` 的 GitHub Release，更新包由私钥签名 |
| 测试 | `go test` | 视图用 MyGo 的 `ui.NewTester` 在没有窗口的情况下测试；来源有直接调用本机 winget、GitHub API 的只读集成测试 |

## 功能特性

| 页面 | 功能 |
|---|---|
| 软件 | 左侧勾选分类、标签（取并集，含“未分类”）；表格显示名称、分类、标签、来源和状态，可排序、多选、全选当前筛选结果；选中后“安装”或“更新”，先预览每一项将执行还是跳过及原因，确认后加入任务 |
| 添加 / 编辑软件 | winget、scoop 先搜索再从结果中选择，保存精确 ID；GitHub 填写仓库后可“检查”将选中的安装包；可填写检测规则，认出不是从该来源装的同一软件 |
| 任务 | 逐个执行，显示每项的状态和原因；停止（当前项完成后）、继续、重试失败项、清除已完成、打开日志 |
| 设置 | 各来源是否可用（winget 缺模块时可一键安装）、GitHub 额度；分类和标签的添加、重命名、删除；导出、导入清单；版本和自更新 |

防误装：状态读不准就不装；队列执行每一项前再查一次，已装的跳过；装完再查一次，查不到就记为失败；GitHub 安装包无法唯一确定时拒绝安装，下载后校验 SHA-256。

## 使用方法

只支持 Windows。需要 Go（版本见 `go.mod`）和 winget；scoop 可选。

```bash
go tool mygo dev                # 开发模式，热重载
CGO_ENABLED=0 go test ./...     # 运行测试
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
├── main.go                   # 应用入口：组装来源、核心和窗口
├── internal/                 # 业务代码：界面、核心、队列、来源、清单等
├── go.mod / go.sum           # Go 模块，锁定 MyGo 版本和 CLI 工具
├── mygo.json                 # MyGo 应用配置
├── resources/                # 应用图标等资源
├── docs/                     # 当前代码和程序的长期文档
├── references/               # 外部文档和外部仓库参考
├── plans/                    # 计划、设计和改进追踪
└── .agents/                  # AI 项目规则与 Skill
```

详细职责见 [项目结构总览](./docs/architecture/project-structure.md) 和 [internal/README.md](./internal/README.md)。

## 开发路线

[基本实现计划](./plans/2026-11-07-basic-implementation/00-overview.md) 的 7 个阶段已经实现，等待用户测试；实施情况见 [实施报告](./plans/2026-11-07-basic-implementation/09-implementation-report.md)。
