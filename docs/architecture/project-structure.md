# 项目结构总览

> 当前项目的目录结构、职责边界和层级索引以本文件为准。

## 总分层

```text
项目根目录/
├── 入口与协作层
├── 项目文档层
├── 外部参考层
├── 计划层
├── 应用源码层
└── 运行与产物层
```

## 入口与协作层

| 路径 | 职责 |
|---|---|
| [README.md](../../README.md) | 项目总览、快速启动和文档分流 |
| [AGENTS.md](../../AGENTS.md) | AI 助手协作入口、约束和项目不变量 |
| [.agents/rules/](../../.agents/rules/) | 项目级可自动加载规则 |

## 项目文档层

| 路径 | 职责 |
|---|---|
| [docs/README.md](../../docs/README.md) | 当前项目文档总入口 |
| [docs/architecture/](../../docs/architecture/) | 架构、结构、边界和长期技术事实 |
| [docs/operations.md](../../docs/operations.md) | 运行、配置、部署和运维说明 |
| [docs/api.md](../../docs/api.md) | API、接口、schema 和事件合同说明 |

## 外部参考层

| 路径 | 职责 |
|---|---|
| [references/README.md](../../references/README.md) | 外部资料总索引 |
| [references/external-docs/](../../references/external-docs/) | 外部文档、标准、教程和资料结构 |
| [references/external-repos/](../../references/external-repos/) | 外部仓库、参考实现和对比分析 |

外部参考只记录来源和可借鉴点。采纳后的当前项目事实应回写到 `docs/` 或源码，不应停留在 `references/`。

## 计划层

| 路径 | 职责 |
|---|---|
| [plans/](../../plans/) | 计划、设计草案、执行过程和验收状态 |

## 应用源码层

Go 模块 `appshelf`，最初由 `mygo init -template native` 生成，0.2.0 起界面换成 `frontend/` 的 Web 前端。Go 依赖：MyGo、`golang.org/x/sys`（读注册表、调用 `ShellExecuteExW`）、`go.yaml.in/yaml/v3`（读 winget-pkgs 的清单文件），版本见 `go.mod`。

| 路径 | 职责 |
|---|---|
| [main.go](../../main.go) | 应用入口：单实例锁、数据目录、组装来源、`core`、图标和详情服务，绑定 `internal/api` 的服务，注册图标协议，创建窗口，接入更新器 |
| [frontend/](../../frontend/) | Web 前端：React + TypeScript + Tailwind CSS + shadcn/ui，Vite+ 构建，见 [frontend/README.md](../../frontend/README.md) |
| [internal/](../../internal/) | 全部业务代码，按包分层，见 [internal/README.md](../../internal/README.md) |
| [go.mod](../../go.mod) / `go.sum` | 依赖和 MyGo CLI 工具版本 |
| [mygo.json](../../mygo.json) | 应用名、标识符、版本号、输出目录、自更新配置 |
| [CHANGELOG.md](../../CHANGELOG.md) | 每个版本的更新说明，构建时写进自更新清单 |
| [resources/icon.png](../../resources/icon.png) | 应用图标（1024×1024，冰川蓝玻璃圆角方形，外部透明；`frontend/src/assets/logo.png` 和 `frontend/public/favicon.png` 是同一图标的 128×128） |
| [.agents/skills/mygo-maintenance/](../../.agents/skills/mygo-maintenance/) | MyGo 版本维护 Skill |

`internal/` 的分层：每个包只依赖层号比它大的包，`main.go` 在最上面组装。

| 层 | 包 | 职责 |
|---|---|---|
| 1 | `internal/api` | 前端调用的绑定服务和 `changed` 事件；一个循环 goroutine 串行读写 `core` |
| 2 | `internal/core`、`internal/icons`、`internal/discover` | 清单、本机状态、来源和队列的汇总；图标的查找、下载和缓存；搜索排序、软件详情和检测规则建议 |
| 3 | `internal/queue`、`internal/inventory` | 逐个执行的安装队列，执行前后都复查；本机已装程序的汇总与对照 |
| 4 | `internal/source/winget`、`internal/source/scoop`、`internal/source/github` | 三个安装来源 |
| 5 | `internal/source`、`internal/runner`、`internal/detect` | 来源接口和状态类型；外部命令、PowerShell 脚本、提权启动安装程序；注册表扫描、检测规则和规则建议 |
| 6 | `internal/catalog`、`internal/state`、`internal/version` | 清单的格式、校验、编辑、导入合并；本机状态；版本号规范化和比较 |
| 7 | `internal/jsonfile`、`internal/secret` | JSON 原子写入；以当前用户 DPAPI 加密保存机密 |

前端只调用 `internal/api` 绑定的服务（客户端由 `go tool mygo generate` 生成到 `frontend/src/mygo.ts`），图标走 Go 注册的 `icon` 协议；安装判断、检测和队列都在 Go 端。

各包的文件和测试见 [internal/README.md](../../internal/README.md)，来源的实现细节见 [internal/source/README.md](../../internal/source/README.md)。新增包或来源时同步这两处和本表。

## 运行与产物层

| 路径 | 职责 | 是否入库 |
|---|---|---|
| [build/](../../build/) | `mygo build` 的打包产物 | 否 |
| [frontend/node_modules/](../../frontend/node_modules/)、`frontend/dist/` | 前端依赖和构建结果 | 否（`frontend/.gitignore`） |
| `.mygo/` | 开发版本的应用 | 否 |
| `mygo_windows_*.syso` | 构建时生成的 Windows 资源 | 否 |

用户的软件清单、本机状态和设置是运行时数据，存放在 `%AppData%\AppShelf\`，图标和详情缓存在 `%LocalAppData%\AppShelf\`，都不入库；自更新的签名私钥也不入库，见 [运行与部署说明](../operations.md#签名密钥)。
