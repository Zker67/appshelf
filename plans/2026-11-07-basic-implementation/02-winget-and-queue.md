# 阶段 02：winget 来源与安装队列

状态：`[done]`（真实安装测试默认不运行，等待用户指定测试软件）

## 目标

实现 winget 来源的检测、搜索、检查更新、安装和更新，以及所有来源共用的安装队列和防误装流程。

## 步骤 1：技术核验（内部，不交付）

在本机只读地确认下面几点，结果写进本文件的“核验结果”一节。和预期不符时，先改计划再写代码。

| 核验项 | 预期 |
|---|---|
| `Get-WinGetPackage \| ConvertTo-Json` | 输出 Id、Name、InstalledVersion、Source、IsUpdateAvailable、AvailableVersions |
| `Microsoft.WinGet.Client` 能否在 Windows PowerShell 5.1 中运行 | 能；不能时改用 `pwsh`，并把 `pwsh` 加入来源可用性检查 |
| `Get-WinGetPackage -Id X -MatchOption Equals` | 精确匹配单个包 |
| `Find-WinGetPackage -Query X -Source winget` | 可以用于添加软件时的搜索 |
| 已装但没能关联到 winget 源的软件 | Id 以 `ARP\`、`MSIX\` 开头，不会被误认成清单里的 winget ID |
| 全量查询耗时 | 记录实测值，决定界面上加载状态的展示方式 |

## 步骤 2：winget 适配器（`internal/source/winget`）

| 方法 | 实现 |
|---|---|
| `Check` | `winget --version` 能运行，并且 `Microsoft.WinGet.Client` 模块已安装。缺少模块时返回“可修复”，由设置页提示安装 |
| `Installed` | 一次 `Get-WinGetPackage` 全量查询，按 Id 精确匹配清单里的 winget 软件（区分大小写，与 `--exact` 一致） |
| `Recheck` | `Get-WinGetPackage -Id <id> -MatchOption Equals` |
| `Search` | `Find-WinGetPackage -Query <q> -Source winget`，返回 Id、名称、版本 |
| `Install` | `winget install --id <id> --exact --source winget --no-upgrade --accept-package-agreements --accept-source-agreements --disable-interactivity` |
| `Upgrade` | `winget upgrade --id <id> --exact --source winget --accept-package-agreements --accept-source-agreements --disable-interactivity` |

- 所有 winget 调用共用一把进程内全局锁，因为 winget 的本地索引不支持并发访问（UniGetUI 的做法）。
- 退出码：`0` 成功；`0x8A150061`（已安装）记为“跳过：已安装”；`0x8A150014`（找不到）和 `0x8A150016`（多个匹配）记为失败，并显示原因；其他非零值记为失败，并附上退出码和输出末尾几行。
- 不采用 UniGetUI 的“ID 被截断时退回按名称安装”。

## 步骤 3：安装队列（`internal/queue`）

- 一个后台 goroutine 按顺序逐项执行，同一时间只执行一项。
- 每一项包含：软件、动作（安装或更新）、状态（等待、进行中、成功、跳过、失败）、失败原因、日志文件位置。
- 执行一项的流程：
  1. 调用 `Recheck`，再匹配 `detect` 规则。
     - 动作是“安装”且已装：记为跳过，不执行。
     - 动作是“更新”但没有可用更新：记为跳过。
     - 结果为“未知”：记为失败，原因是“无法确认安装状态”，不执行。
  2. 执行 `Install` 或 `Upgrade`。
  3. 再次 `Recheck`：确认已装（更新时确认版本已经变化）才记为成功，否则记为失败。
- 支持“重试”：失败的项重新排到队尾。
- 支持“停止”：当前这一项执行完后，不再执行剩下的项，剩下的项保持“等待”。
- 状态变化通过 `Window.Update` 通知界面（按 MyGo 文档里“从其他 goroutine 修改状态”的方式）。

## 步骤 4：批量查询状态

`internal/source` 提供一个汇总函数：对清单里的所有软件，按来源分组调用 `Installed`，合并 `detect` 结果，得出每个软件的最终状态。启动时自动执行一次；界面上的“检查更新”按钮再次执行。

## 成功标准

- 集成测试（只读，真实 winget）：`Check` 能识别本机环境；`Installed` 能找出本机一个确定已装的 winget 包；`Search` 能搜到 `Microsoft.VisualStudioCode`；`Recheck` 对一个不存在的 ID 返回“未安装”，而不是报错。
- 单元测试：队列的执行流程用假的来源测试，包括已装跳过、未知不装、装完复查失败、重试、停止。
- 安装集成测试：设置 `APPSHELF_INSTALL_TEST=1` 后，安装、复查、再次安装（应跳过）一个用户指定的测试软件。默认不运行。

## 失败与回滚

模块方案核验不通过时，`Installed` 和 `Recheck` 改为逐项执行 `winget list --id <id> --exact` 并看退出码（`0x8A150014` 表示未安装），检查更新的方式另行评估并回写计划。

## 核验结果

2026-11-07 在本机（Windows 11，winget 已装）只读核验：

| 核验项 | 结果 |
|---|---|
| `Get-WinGetPackage` 的输出 | 符合预期：有 Id、Name、InstalledVersion、Source、IsUpdateAvailable、AvailableVersions。读取 AvailableVersions 较慢，所以只对清单里的软件读取 |
| Windows PowerShell 5.1 | 符合：本机原来没有这个模块，按用户同意为当前用户安装了 `Microsoft.WinGet.Client` 1.29.380，在 5.1 中可用，不需要 `pwsh` |
| `-MatchOption Equals` | **与预期不同**：`Equals` 区分大小写。改用 `EqualsCaseInsensitive`，批量查询也按不区分大小写的 ID 匹配：ID 大小写不同时宁可认成已装、跳过安装，不会因为大小写而重复安装。安装仍用 `--exact` |
| `Find-WinGetPackage -Query X -Source winget` | 符合，用于添加软件时的搜索，取前 30 项 |
| `ARP\`、`MSIX\` 开头的 ID | 符合：本机 318 个已装项中有 170 个，不会与清单里的 winget ID 混淆 |
| 全量查询耗时 | `Get-WinGetPackage` 本身约 1.4 秒，加上 PowerShell 启动和模块加载约 2.3 秒。启动时后台查询，状态列先显示“检查中…” |

实施中的调整：

- 只有 winget 的安装和更新命令共用一把锁；状态查询走 `Microsoft.WinGet.Client` 模块，不加锁。
- 新增退出码：`0x8A15002B`（没有适用的更新）、`0x8A150101`（软件正在运行）、`0x8A150102`（另一个安装正在进行）、`0x8A150109`（需要重启，记为成功并提示）、`0x8A15010C`（已取消）。
- 队列增加“继续”（停止后继续执行剩下的项）和“清除已完成”。
