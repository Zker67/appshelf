# 阶段 01：基础层

状态：`[done]`

## 目标

搭好代码结构、数据格式和命令执行层，让后面的来源适配器和界面都基于同一套接口。本阶段没有可见的界面变化。

## 代码结构

```text
appshelf/
├── main.go                  # 启动：单实例锁、加载数据、打开窗口
├── internal/
│   ├── catalog/             # 清单模型、校验、读写、导入合并
│   ├── state/               # 本机状态（GitHub 安装记录）
│   ├── source/              # 来源接口和公共类型
│   │   ├── winget/          # 阶段 02
│   │   ├── scoop/           # 阶段 04
│   │   └── github/          # 阶段 06
│   ├── detect/              # 注册表 Uninstall 扫描，以及检测规则匹配
│   ├── runner/              # 外部命令执行：超时、输出采集、日志
│   └── queue/               # 安装队列（阶段 02）
└── ui/                      # 视图（阶段 03）
```

目录内有两个以上稳定模块时，补一份中文 `README.md`（`internal/`、`internal/source/`）。

## 清单格式（`catalog.json`）

```json
{
  "schemaVersion": 1,
  "categories": [{ "id": "c_dev", "name": "开发工具" }],
  "tags": [{ "id": "t_ai", "name": "AI" }],
  "apps": [
    {
      "id": "a_7f3k",
      "name": "Visual Studio Code",
      "categoryId": "c_dev",
      "tagIds": ["t_ai"],
      "source": { "type": "winget", "id": "Microsoft.VisualStudioCode" },
      "detect": { "displayName": "^Microsoft Visual Studio Code" }
    }
  ]
}
```

`source` 的三种形式：

| type | 字段 | 唯一键 |
|---|---|---|
| `winget` | `id` | `winget:<id>` |
| `scoop` | `bucket`、`name`，可选 `bucketUrl`（不在已知 bucket 列表里时必填） | `scoop:<bucket>/<name>` |
| `github` | `repo`（`owner/name`），可选 `assetPattern`（正则）、`installerArgs` | `github:<owner>/<name>` |

`detect`：可选的检测规则，包括 `displayName`（匹配注册表 DisplayName 的正则）和 `exePath`（可含环境变量的路径），两者至少填一个。**GitHub 来源必须填写**；winget 和 scoop 可选，填写后作为额外的已装判断。

校验规则（读文件和导入时都执行，任何一条不通过就整份拒绝，不部分导入）：

- `schemaVersion` 必须是已知版本。
- ID 唯一；`categoryId` 和 `tagIds` 必须指向存在的分类和标签；`categoryId` 可以为空（未分类）。
- 同一个来源唯一键只能出现一次。
- winget ID 和 scoop 的 bucket、name 符合严格的字符集（字母、数字、`.`、`-`、`_`、`+`）；`repo` 符合 `owner/name` 格式；正则必须能编译。

## 本机状态（`state.json`）

只存放无法从本机实时查到的信息：

```json
{
  "schemaVersion": 1,
  "github": {
    "a_9x2m": { "tag": "v1.4.0", "assetName": "app-1.4.0-x64.msi", "digest": "sha256:…", "installedAt": "2026-11-07T10:00:00Z" }
  }
}
```

winget 和 scoop 的已装状态和版本每次都实时查询，不缓存。

## 存放位置与写入

- 两个文件都放在 `App.Path(mygo.PathUserData)`（`%AppData%\AppShelf\`）。
- 写入时先写临时文件再重命名，避免写到一半损坏。
- 启动时用 `App.RequestSingleInstanceLock()` 保证只有一个实例，避免两个进程同时写同一份文件；第二个实例启动时把已有窗口带到前面。
- 首次启动没有文件时，创建一份空清单。

## 来源接口

```go
type Source interface {
    Name() string
    Check(ctx context.Context) Availability                     // 来源是否可用，不可用的原因
    Installed(ctx context.Context, apps []catalog.App) (map[string]Status, error)  // 批量查已装状态和更新
    Recheck(ctx context.Context, app catalog.App) (Status, error)                  // 单项复查，安装前后各调用一次
    Search(ctx context.Context, query string) ([]SearchResult, error)              // 添加软件时查精确 ID（GitHub 不提供）
    Install(ctx context.Context, app catalog.App, log io.Writer) error
    Upgrade(ctx context.Context, app catalog.App, log io.Writer) error
}
```

`Status` 的取值：`未安装`、`已安装`（含版本）、`可更新`（含当前版本和新版本）、`未知`（含原因）。只要查询出错，就返回 `未知`，不返回 `未安装`。

`detect` 规则由 `internal/detect` 统一处理：扫描 `HKLM\…\Uninstall`、`HKLM\SOFTWARE\WOW6432Node\…\Uninstall`、`HKCU\…\Uninstall`，跳过 `SystemComponent=1` 和带 `ParentKeyName` 的条目。任一来源报告“未安装”、但 `detect` 规则命中时，最终状态为“已安装（来源外）”，不允许安装。

## 命令执行（`internal/runner`）

- 只用 `exec.CommandContext` 加参数数组。
- PowerShell 脚本用 `go:embed` 嵌入，运行时写到临时目录，用 `powershell -NoProfile -NonInteractive -ExecutionPolicy Bypass -File <脚本> -参数 值` 调用，参数不拼进脚本文本。
- 每次调用都有超时（查询 120 秒；安装不设硬超时，但可以取消）。
- 输出写入 `App.Path(mygo.PathLogs)` 下的日志文件；界面只显示失败原因和最后几行输出。

## 步骤

1. 建目录和包，定义 `catalog`、`state`、`source` 的类型。
2. 实现清单的读、写、校验，以及按 ID 增删改分类、标签、软件。删除分类时，相关软件变为未分类；删除标签时，从所有软件上移除该标签。
3. 实现 `state` 的读写。
4. 实现 `runner` 和 `detect`。
5. `main.go` 接入单实例锁和数据加载；窗口暂时保持示例内容。

## 成功标准

- 单元测试覆盖：校验规则的每一条、删除分类和标签的联动、写入后再读取内容一致。
- `detect` 的集成测试在本机真实扫描注册表，能找到一个确定已装的软件（例如 Git）。
- `go vet ./...`、`go test ./...` 通过。

## 失败与回滚

本阶段只新增文件；失败时删除 `internal/` 并还原 `main.go`。
