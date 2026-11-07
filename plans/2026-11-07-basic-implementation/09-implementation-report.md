# AppShelf 基本实现：实施报告

日期：2026-11-07。状态：7 个阶段已实现，等待用户测试；scoop 未实测，自更新等待发布。

## 实际改动

### 代码

- `main.go`：单实例锁、数据目录、组装三个来源和 `core`、主窗口、更新器插件（`plugins/updater/native`）。
- `internal/`：`view`（界面）、`core`（汇总）、`queue`（安装队列）、`source`（接口）及 `source/winget`、`source/scoop`、`source/github`、`runner`、`detect`、`catalog`、`state`、`version`、`jsonfile`。职责见 [internal/README.md](../../internal/README.md) 和 [internal/source/README.md](../../internal/source/README.md)。
- 删除了 MyGo 示例的 `main_test.go`。
- 新增依赖 `golang.org/x/sys`（读注册表、调用 `ShellExecuteExW`）。

### 配置与文档

- `mygo.json`：`name` 改为 `AppShelf`，加入 `updates.publicKey` 和 `updates.github: Zker67/appshelf`。
- 新增 `CHANGELOG.md`（`## 0.1.0`）。
- 同步了 `README.md`、`docs/api.md`（两份 JSON 的格式和导入合并规则）、`docs/operations.md`（运行环境、数据位置、测试变量、发布步骤、故障排查）、`docs/architecture/project-structure.md`（实际分层），新增 `internal/README.md`、`internal/source/README.md`。

### 仓库之外的改动（本机）

| 改动 | 位置 | 说明 |
|---|---|---|
| 安装 PowerShell 模块 `Microsoft.WinGet.Client` 1.29.380 | `Documents\WindowsPowerShell\Modules\`（当前用户） | 用户同意依赖该模块后安装，用于读取 winget 状态 |
| 生成自更新签名密钥 | `%AppData%\mygo\update-keys\mygo-update.key`（私钥）、`mygo-update.pub` | 私钥不在仓库里；需要用户另外备份 |
| NSIS 3.13 | `%LOCALAPPDATA%\mygo\nsis-3.13\` | `mygo build` 制作安装包时自动下载 |
| AppShelf 数据目录 | `%AppData%\AppShelf\`（`logs\`、`updater.json`、`window-state.json`） | 启动自测时创建；还没有 `catalog.json` |
| 构建产物 | `build/`（已被 `.gitignore` 忽略） | exe、安装包、签名的更新文件 |
| 指挥室索引 | `REPOS.md`、`repos.json` | 用 `python scripts/repos.py sync` 登记 appshelf |

没有执行任何 git 提交、创建远端仓库、上传或发布。

## 验证结果

| 验证 | 结果 |
|---|---|
| `gofmt -l .`、`go vet ./...`、`go tool mygo vet .`、`go tool mygo vet ./internal/view` | 通过 |
| `CGO_ENABLED=0 go test ./...` | 全部通过（共 11 个有测试的包） |
| winget 只读集成测试（真实 winget） | 通过：可用性、批量状态、复查不存在的 ID 返回未安装、搜索 |
| GitHub 真实 API 测试 | 本机未认证额度已用完时跳过；临时用 `gh auth token` 设置 token 后通过 |
| scoop 真实测试 | 本机没有 scoop，跳过 |
| 三个来源的真实安装测试 | 默认不运行，没有运行 |
| `go tool mygo build`（带私钥） | 生成 exe、安装包、签名的 `update-windows-amd64.json`、更新包、`install.ps1`、`SHA256SUMS` |
| 启动构建出的 `AppShelf.exe` | 正常运行，关闭后以 0 退出 |

**未验证**（需要用户测试或授权）：

- 任何真实安装、更新：winget、scoop、GitHub 都没有在本机真实安装过软件。
- scoop 的全部命令（按源码实现）。
- GitHub `.msi` 用 `/passive` 时能否正常弹出 UAC，`ShellExecuteExW` 启动真实安装程序的表现（阶段 06 计划要求实施时确认，因为需要真实安装，留给用户测试）。
- 自更新的完整流程：需要创建公开仓库并发布两个版本。
- 界面效果：由用户在真实窗口里检查。

## 与计划不同的地方

| 计划 | 实际 | 原因 |
|---|---|---|
| 视图放在 `ui/` | 放在 `internal/view`；另加 `internal/core`（汇总）、`internal/version`、`internal/jsonfile` | 视图不直接接触来源和文件；版本比较和原子写入被多个包共用 |
| 页面路由切换三个页面 | 用 `Tabs` 切换 | 三个固定页面，`Tabs` 更简单，任务页签上直接显示任务数 |
| 左侧用 `Sidebar` 加勾选框 | 用 `Checkbox` 列表 | MyGo 的 `Sidebar` 只能单选；另加了“未分类”筛选项 |
| 来源接口 `Name`、`Installed`，`Install` 只返回 `error` | `Type`、`Statuses`；`Install`/`Upgrade` 返回 `Result`（已安装、需要重启、GitHub 安装记录）；`Search` 拆成可选的 `Searcher` | 队列需要区分“已安装”和“需要重启”；GitHub 不支持搜索 |
| 状态只有四种 | 增加 `Unavailable`（来源不可用）和 `External`（来源外安装）标记 | 界面要分别说明原因；来源外安装时不从该来源更新 |
| PowerShell 脚本用 `go:embed`，参数用 `-参数 值` 传入 | 脚本是 Go 字符串常量，带 BOM 写到临时目录；输入、输出都是 UTF-8 JSON 文件 | 避免中文编码问题，以及以 `-` 开头的值被当成参数 |
| 所有外部命令用 `exec.CommandContext` | 命令行工具如此；GitHub 安装程序用 `ShellExecuteExW` 启动 | 需要管理员权限的安装程序用 `exec` 启动会直接失败（错误 740），`ShellExecuteExW` 会弹出 UAC |
| 首次启动创建空清单文件 | 没有文件时在内存里用空清单，第一次保存时才写文件；无法解析的文件改名为 `.broken-<时间>` 留存 | 不写没有内容的文件；损坏的文件不被覆盖 |
| winget 按 ID 区分大小写匹配，所有 winget 调用共用一把锁 | 状态查询不区分大小写、不加锁；只有安装和更新命令加锁 | `Equals` 区分大小写，大小写不同会误判为未装；锁只需要保护 winget 命令行 |
| scoop 按 `bucket/name` 精确匹配 | 按软件名匹配所有 bucket；批量查询先 `scoop update` 再 `scoop status -l` | scoop 同名软件只能装一个；`-l` 不联网 |
| 队列：重试、停止 | 另加“继续”“清除已完成” | 停止后需要能接着执行；完成的项太多时可以清掉 |
| `Executor` 只负责执行 | 增加 `Finished`：执行后复查、保存 GitHub 安装记录，返回最终状态 | 复查和记录需要 `core` 的数据 |
| “检查 AppShelf 更新”总是显示 | 开发版不显示按钮，改为说明“自更新只在用安装包安装的版本里可用” | 开发版不能自更新，点了只会报错 |
| 测试场景 13 预期断网时显示“未知” | 本机检测不依赖网络，读得到的照常显示；只有读不到才是“未知”；安装会在下载时失败并说明原因 | 按实际行为修正预期，同样不会误装 |
| 检查不了更新时（例如 GitHub 额度用完）预览写“已是最新版本” | 有原因时写“没有发现更新（原因）” | 避免误导；交付前发现并修正 |

## 遗留项

1. **用户测试**：按 [07-acceptance.md](./07-acceptance.md) 的 15 个场景测试；发现的问题记在本节或另开计划。
2. **scoop 实测**：在装有 scoop 的机器上运行只读测试，并测场景 4、14。
3. **真实安装测试**：用户指定测试软件后，用 `APPSHELF_INSTALL_TEST=1` 运行三个来源的安装测试（变量见 [运行与部署说明](../../docs/operations.md#测试与构建)）。
4. **自更新**：需要用户授权创建公开仓库 `Zker67/appshelf`、提交代码、发布 `v0.1.0`，之后再发布一个新版本来验证升级。
5. **签名私钥备份**：`%AppData%\mygo\update-keys\mygo-update.key` 需要用户备份到安全位置，丢失后已安装的版本收不到更新。
6. **GitHub token**：本机未认证的 GitHub 额度已经用完过，建议设置环境变量 `GH_TOKEN`。
