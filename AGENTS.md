# AGENTS.md

本文件是 AI 助手的项目入口文档。每次新会话应先阅读本文件，再阅读 `README.md` 和相关计划文档。

## 项目定位

AppShelf：Windows 个人软件商城，桌面应用；自更新使用公开仓库 `Zker67/appshelf` 的 GitHub Release。软件清单支持分类和标签，可以导入导出；能检测本机已装软件，按分类或标签多选逐个批量安装，并检查更新（包括 AppShelf 自身）。安装来源为 winget、scoop、GitHub Releases。技术栈：Go + MyGo（`github.com/egoist/mygo`）原生 UI，版本以 `go.mod` 为准。

## 项目不变量

- **绝不误装**：安装前必须检测本机是否已有该软件（包括不是经 AppShelf 安装的），已安装的跳过；包 ID 精确匹配并指定来源（如 winget `--exact --source`），不按名称模糊安装。检测结果不确定时停下来让用户确认，不猜测。
- **清单与本机状态分离**：软件清单（分类、标签、来源，可导入导出、跨设备共享）和本机状态（已安装、版本、忽略的更新）分开存放；卸载软件不会让它从清单里消失。
- **批量安装逐个执行**：安装前先预览，确认后按队列逐项执行，每项单独记录结果，失败的可以单独重试。
- **复用来源自身的能力**：安装、检测、更新都调用 winget、scoop 和 GitHub API 的官方能力，不自己实现安装器。
- **不做离线安装包**：不缓存或管理本地 EXE/MSI 安装包。
- **MyGo 原生 UI**：界面用 `ui` 包编写；只有原生 UI 确实做不到的部分才考虑 Web 页面窗口，并且先说明原因。

## 项目特有验证

- `gofmt -l .`、`go vet ./...`、`go tool mygo vet .`、`go tool mygo vet ./internal/view` 和 `CGO_ENABLED=0 go test ./...`；视图用 MyGo 的 `ui.NewTester` 测试。
- 涉及安装或卸载的集成测试会真实修改本机，执行前必须获得用户授权，并使用用户认可的测试软件（`APPSHELF_INSTALL_TEST=1` 等变量见 `docs/operations.md`）。只读检测（list、search、检查更新）可以直接运行。
- 界面效果不由 AI 截图自验；改完说明改了什么、怎么验证，由用户在真实窗口里检查。
- 自更新的签名私钥（`%AppData%\mygo\update-keys\mygo-update.key`）绝不进入仓库、日志或输出。`go tool mygo build -upload`、创建或发布 GitHub Release 属于发布操作，需要用户明确授权。
- 升级 MyGo 时参考 `.agents/skills/mygo-maintenance/`，并核对 [Releases](https://github.com/egoist/mygo/releases) 的变更说明。

## 启动流程

1. 阅读 `README.md`，确认项目目标、使用方法和文档结构。
2. 阅读 `docs/README.md` 和 `docs/architecture/source-of-truth.md`，确认信息归属。
3. 阅读 `plans/README.md`，确认是否存在进行中的计划。
4. 如任务涉及外部资料，阅读 `references/README.md` 和对应来源索引。
5. 改代码前先理解现有结构、风格、测试入口和错误处理方式。

## 目录职责

| 目录/文件 | 用途 | 默认权限 |
|---|---|---|
| `.agents/rules/` | 项目级 AI 规则 | 只读，除非任务要求维护规则 |
| `.agents/README.md` | `.agents/` 使用说明 | 只读 |
| `.agents/skills/mygo-maintenance/` | MyGo 官方生成的版本维护 Skill | 只读，升级 MyGo 时使用 |
| `main.go`、`internal/` | 应用代码，分层见 `docs/architecture/project-structure.md` | 读写 |
| `mygo.json` | MyGo 应用配置（含自更新公钥和仓库） | 读写 |
| `CHANGELOG.md` | 每个版本的更新说明 | 读写 |
| `docs/` | 当前代码和程序的长期文档 | 读写 |
| `references/` | 外部文档和外部仓库参考 | 读写 |
| `plans/` | 计划、设计和改进追踪 | 读写 |
| `README.md` | 项目说明 | 读写 |

## 规范约定

- 默认使用简体中文沟通和编写文档；代码标识符、命令和外部专名保持原文。
- 保持 diff 聚焦，每处改动都对应任务目标、缺陷修复或必要验证。
- 非平凡改动先写计划到 `plans/`，并同步维护 `plans/README.md`。
- 改代码后按风险运行定向测试、类型检查、lint 或构建。
- 不提交真实凭据、机器路径、运行态数据、缓存或个人笔记。
- 默认不新增依赖；确需新增时说明原因、影响面和验证方式。

## 文档维护

- `README.md` 面向人类使用者，描述目标、使用方式和主要结构。
- `AGENTS.md` 面向 AI 助手，描述协作入口、约束和项目不变量。
- `docs/` 面向当前项目事实，承载结构、运行、接口和长期专题文档。
- `docs/architecture/source-of-truth.md` 规定同类信息的主维护位置，其他文档只做入口或摘要。
- `references/` 面向外部资料，记录来源结构、版本、commit、许可和借鉴点；采纳后的当前项目事实应回写到 `docs/` 或源码。
- `.agents/rules/` 存放稳定、长期、可自动加载的项目规则。
- `plans/README.md` 是计划索引，新增、移动、归档计划时同步更新；完成后的稳定事实同步到 `docs/`。
