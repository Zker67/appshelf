# AGENTS.md

本文件是 AI 助手的项目入口文档。每次新会话应先阅读本文件，再阅读 `README.md` 和相关计划文档。

## 项目定位

AppShelf：Windows 个人软件图书馆，桌面应用；自更新使用公开仓库 `Zker67/appshelf` 的 GitHub Release。软件清单支持分类和标签，可以导入导出；能检测本机已装软件，按分类或标签多选逐个批量安装，并检查更新（包括 AppShelf 自身）。“发现”页统一搜索 winget、scoop、GitHub 并一键加入清单，“本机已装”页列出本机已有的程序，“卸载”页按占用空间和安装时间挑选程序卸载并清理残留。任务面板显示每项的阶段、下载进度和实时日志。安装来源为 winget、scoop、GitHub Releases。技术栈：Go + MyGo（`github.com/egoist/mygo`）的 Web 页面窗口，界面是 `frontend/` 的 React 应用；版本以 `go.mod` 和 `frontend/package.json` 为准。

## 项目不变量

- **绝不误装**：安装前必须检测本机是否已有该软件（包括不是经 AppShelf 安装的），已安装的跳过；包 ID 精确匹配并指定来源（如 winget `--exact --source`），不按名称模糊安装。检测结果不确定时停下来让用户确认，不猜测。
- **清单与本机状态分离**：软件清单（分类、标签、来源，可导入导出、跨设备共享）和本机状态（已安装、版本、忽略的更新）分开存放；卸载软件不会让它从清单里消失。
- **批量安装逐个执行**：安装前先预览，确认后按队列逐项执行，每项单独记录结果，失败的可以单独重试。
- **复用来源自身的能力**：安装、检测、更新都调用 winget、scoop 和 GitHub API 的官方能力，不自己实现安装器。卸载同样优先交给 winget、scoop、`Remove-AppxPackage`，其余运行程序自己注册的卸载命令；不自己删除程序文件来代替卸载。
- **绝不误删**：卸载对象只来自 Go 读到的本机已装列表（前端只传 key，不能传命令或路径），先预览、由用户确认，执行前后复查，以执行后程序是否还在判断成败。winget、scoop、AppShelf 自身、标明不可卸载的程序和系统应用包不卸载。残留只在卸载成功后由用户发起查找，从不自动清理，只默认勾选“很可能是残留”的项；系统和共享目录、AppShelf 的目录、其他已装程序的目录不列出也不清理，注册表只动 `HKCU\Software` 下的键和当前用户的开机启动项；文件进回收站，注册表先导出备份，备份失败就不删除。
- **不做离线安装包**：不缓存或管理本地 EXE/MSI 安装包。
- **MyGo Web 前端，前端只做展示**：界面是 `frontend/` 的 React 应用，只通过 `internal/api` 绑定的服务读写；安装判断、检测、队列、外部命令和网络请求都在 Go 端，token 不进前端。本机已装、搜索结果、软件详情和图标只用于展示，不参与是否已装的判断。
- **安装与直接更新的边界**：安装只对清单里的软件执行；“本机已装”页允许对精确识别的 winget / scoop 软件直接更新，包括未加入清单的软件。直接更新先预览并确认，复用逐项队列和执行前后复查，不自动加入清单，不退化为安装；未知来源或来源外安装仍不更新。
- **自动更新只对用户逐个开启的软件**：开启即是用户对该软件后续更新的确认；只用于 scoop（非全局）和用户范围安装的 winget 软件，且版本已知、程序没有在运行，绝不在后台弹出 UAC 或安装程序窗口（GitHub 软件、机器范围安装只提醒）。自动更新同样只走 `Upgrade` 和队列的执行前后复查。更新方式、跳过的版本、暂缓期限和更新记录是本机状态（`state.json`），不随清单导出。

## 项目特有验证

- `gofmt -l .`、`go vet ./...`、`go tool mygo vet` 和 `CGO_ENABLED=0 go test ./...`；绑定服务在 `internal/api` 用假来源测试。
- 前端：`bun run --cwd frontend check`（格式、lint、类型）和 `bun run --cwd frontend test`；改了 `internal/api` 的服务或类型后先 `go tool mygo generate`。最后 `go tool mygo build` 确认能打包。
- 涉及安装或卸载的集成测试会真实修改本机，执行前必须获得用户授权，并使用用户认可的测试软件（`APPSHELF_INSTALL_TEST=1`、`APPSHELF_UNINSTALL_TEST=1` 等变量见 `docs/operations.md`）。只读检测（list、search、检查更新、生成卸载方式、读签名）可以直接运行。残留测试只在 `AppShelfTest-*` 临时目录和临时注册表键里造数据，被清理的测试文件会进回收站。
- 界面效果不由 AI 截图自验；改完说明改了什么、怎么验证，由用户在真实窗口里检查。前端界面遵循用户的 `zker67-ui-way` 约定。
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
| `main.go`、`internal/` | Go 代码，分层见 `docs/architecture/project-structure.md` | 读写 |
| `frontend/` | Web 前端，见 `frontend/README.md`；`src/mygo.ts` 和 `src/components/ui/` 是生成的 | 读写 |
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
