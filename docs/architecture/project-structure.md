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
| `README.md` | 项目总览、快速启动和文档分流 |
| `AGENTS.md` | AI 助手协作入口、约束和项目不变量 |
| `.agents/rules/` | 项目级可自动加载规则 |

## 项目文档层

| 路径 | 职责 |
|---|---|
| `docs/README.md` | 当前项目文档总入口 |
| `docs/architecture/` | 架构、结构、边界和长期技术事实 |
| `docs/operations.md` | 运行、配置、部署和运维说明 |
| `docs/api.md` | API、接口、schema 和事件合同说明 |

## 外部参考层

| 路径 | 职责 |
|---|---|
| `references/README.md` | 外部资料总索引 |
| `references/external-docs/` | 外部文档、标准、教程和资料结构 |
| `references/external-repos/` | 外部仓库、参考实现和对比分析 |

外部参考只记录来源和可借鉴点。采纳后的当前项目事实应回写到 `docs/` 或源码，不应停留在 `references/`。

## 计划层

| 路径 | 职责 |
|---|---|
| `plans/` | 计划、设计草案、执行过程和验收状态 |

## 应用源码层

Go 模块 `appshelf`，由 `mygo init -template native` 生成。依赖只有 MyGo 和 `golang.org/x/sys`（读注册表、调用 `ShellExecuteExW`），版本见 `go.mod`。

| 路径 | 职责 |
|---|---|
| `main.go` | 应用入口：单实例锁、数据目录、组装来源和 `core`、创建窗口、接入更新器 |
| `internal/` | 全部业务代码，按包分层，见 [internal/README.md](../../internal/README.md) |
| `go.mod` / `go.sum` | 依赖和 MyGo CLI 工具版本 |
| `mygo.json` | 应用名、标识符、版本号、输出目录、自更新配置 |
| `CHANGELOG.md` | 每个版本的更新说明，构建时写进自更新清单 |
| `resources/icon.png` | 应用图标（1024×1024） |
| `.agents/skills/mygo-maintenance/` | MyGo 版本维护 Skill |

`internal/` 的分层：每个包只依赖层号比它大的包，`main.go` 在最上面组装。

| 层 | 包 | 职责 |
|---|---|---|
| 1 | `internal/view` | 界面：软件、任务、设置三个页签 |
| 2 | `internal/core` | 清单、本机状态、来源和队列的汇总；界面只通过它读写 |
| 3 | `internal/queue` | 逐个执行的安装队列，执行前后都复查 |
| 4 | `internal/source/winget`、`internal/source/scoop`、`internal/source/github` | 三个安装来源 |
| 5 | `internal/source`、`internal/runner`、`internal/detect` | 来源接口和状态类型；外部命令、PowerShell 脚本、提权启动安装程序；注册表扫描和检测规则 |
| 6 | `internal/catalog`、`internal/state`、`internal/version` | 清单的格式、校验、编辑、导入合并；本机状态；版本号规范化和比较 |
| 7 | `internal/jsonfile` | JSON 原子写入 |

各包的文件和测试见 [internal/README.md](../../internal/README.md)，来源的实现细节见 [internal/source/README.md](../../internal/source/README.md)。新增包或来源时同步这两处和本表。

## 运行与产物层

| 路径 | 职责 | 是否入库 |
|---|---|---|
| `build/` | `mygo build` 的打包产物 | 否 |
| `.mygo/` | 开发版本的应用 | 否 |
| `mygo_windows_*.syso` | 构建时生成的 Windows 资源 | 否 |

用户的软件清单和本机状态是运行时数据，存放在 `%AppData%\AppShelf\`，不入库；自更新的签名私钥也不入库，见 [运行与部署说明](../operations.md#签名密钥)。
