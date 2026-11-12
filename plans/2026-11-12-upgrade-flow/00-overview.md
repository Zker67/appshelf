# 升级流程优化（对照 UniGetUI）

状态：`[in-progress]`：P1 四项、P2 三项、P3 四项已实现并通过自动检查，等待用户真实更新验收。

## 背景

对照 UniGetUI 的升级流程：发现更新与执行分离、每个软件独立任务、按包管理器适配、结构化异常处理。AppShelf 的骨架已经一致，而且执行前后都重新读真实版本，比 UniGetUI 在重新查询失败时按预期版本构造记录更严。差距在边角：装完后的等待和判断、自动更新的执行时机、失败后的可操作处理。外部资料见 [UniGetUI 参考](../../references/external-repos/Devolutions__UniGetUI/README.md)。

## 不变量

- 更新成败仍只由执行后的复查决定；等待、`StillOffered` 只影响等待多久和提示文字。
- 自动更新只对用户逐个开启、用户范围、版本已知、程序没在运行的 winget / scoop 软件；绝不在后台弹出 UAC 或安装程序窗口。
- 不退化为安装，不按名称模糊匹配，不改 scope / 架构参数重试。

## P1（本次实施）

| # | 问题 | 做法 | 位置 |
|---|---|---|---|
| 1 | 自动更新只在入队时查程序是否在运行，排在长任务后面时用户可能已经打开它 | 队列项带 `Auto`（`Queue.AddAuto` / `Core.EnqueueAuto`）；复查确认仍有更新后、开始前调用 `Queue.SetHold` 设置的 `App.autoHold`，重新读 `autoBlock` 条件和进程；在运行或无法确认就“跳过”，下次检查再排 | `internal/queue`、`internal/core`、`internal/api/autoupdate.go` |
| 2 | winget 更新固定 `-Mode Default`（SilentWithProgress，部分安装器会弹进度窗口），与“后台不弹窗”冲突 | 新增 `source.SilentUpgrader`；winget 的 `UpgradeSilent` 用 `-Mode Silent` / `--silent`；只有 `Auto` 的更新走它，手动更新不变 | `internal/source`、`internal/source/winget`、`internal/core` |
| 3 | 安装器启动副本后先退出，复查读到旧版本，误判“版本没有变化” | 执行前 `procs.WatchInstallers` 记下已有进程；成功返回后等这次新启动、在临时目录 / `.tmp` / `.cr` `.be` / `SquirrelTemp` 运行的安装进程结束（不按父子关系，以免等到安装程序最后打开的软件）；2 分钟无活动或 30 分钟上限停止等待并写日志。安装与更新共用执行路径，安装也一起受益 | `internal/procs/installers.go`、`internal/core`、`main.go` |
| 4 | 来源版本号对不上时“假更新”反复出现，自动更新每次检查都重跑 | 队列项记 `Latest`（目标版本）和 `StillOffered`（更新后来源仍提示这个版本；版本变了算成功并提示，没变照旧失败）；更新记录多存 `latest`、`stillOffered`；`autoStuck`：同一目标版本有一次 `StillOffered`，或连续失败 3 次，就不再自动更新到它，`autoNote` 说明原因，出新版本自动恢复 | `internal/queue`、`internal/state`、`internal/api` |

### 验收

- 自动检查：`go vet ./...`、`go tool mygo vet`、`CGO_ENABLED=0 go test ./...`、`go tool mygo build`。
- 新测试：`queue`（`TestAutoHold`、`TestUpgradeStillOffered`）、`core`（`TestUpgradesSilentlyAndWaitsForInstallers`）、`procs`（规则、等待各结局、真实进程副本）、`api`（`TestAutoHoldRightBeforeItRuns`、`TestUpdatesAutoStopsWhenStillOffered`、`TestAutoStuck`，`TestUpdatesAuto` 改为期望静默更新）。
- 用户真实验收（会真实修改本机，需用户选测试软件）：
  1. 一个用户范围 winget 软件设为自动更新，确认后台更新时没有任何安装程序窗口；日志第一行为 `Update-WinGetPackage … -Mode Silent`。
  2. 自动更新排队期间打开该软件，任务显示“跳过：它正在运行……”，更新记录有一条“跳过”。
  3. 用一个会启动副本的安装器（Inno Setup / Squirrel 类）手动更新，任务阶段出现“正在等待安装程序结束（…）”，结束后判为成功。
  4. 遇到更新后仍提示同一版本的软件时，更新页详情显示“不再自动更新到这个版本”，下次检查不再自动跑。

### 回滚

各项独立：去掉 `SetHold` 调用即恢复第 1 项；`Run` 不再判断 `SilentUpgrader` 恢复第 2 项；`main.go` 不传 `Installers` 恢复第 3 项；`autoNote` 只返回 `autoBlock` 恢复第 4 项。`state.json` 新增字段可选，旧版本读取时忽略。

## P2（本次实施）

| # | 用户决定 | 做法 | 位置 |
|---|---|---|---|
| 5 | 软件正在运行时**自动关闭** | 只用于用户发起的更新（含重试）；自动更新仍是“在运行就跳过”（AGENTS.md 不变量）。执行前 `Core.SetBeforeUpgrade` 注册的 `App.closeForUpdate` 用 `procs.Closer` 关闭从 `installDir` 运行的进程：先 `WM_CLOSE` 可见窗口、等 10 秒；仍在运行且没有窗口的结束（一开始就没有窗口的不等）；窗口还开着（多半在问是否保存）绝不结束，提到前面、发任务通知、等 2 分钟，仍开着就让这次更新失败并提示关闭后重试；没有权限结束的交给安装程序；读不到安装目录的不关闭。GitHub 软件没有可靠的安装目录，暂不关闭 | `internal/procs/close.go`、`internal/api/closing.go`、`internal/core` |
| 6 | Windows Installer 正忙时排队 | 安装和更新开始前 `_MSIExecute` 被占用就等（最多 30 分钟，scoop 不等）；来源返回 `source.ErrInstallerBusy`（winget `0x8A150102` / 安装程序 1618，GitHub 1618）时等完重新执行一次 | `internal/core`、`internal/source/*` |
| 7 | 用安装程序窗口重试 | winget 实现 `source.InteractiveUpgrader`（`-Mode Interactive` / `--interactive`）；运行过的失败更新可“打开安装程序重试”；用户重试的任务清掉 `Auto`；窗口安装期间，有可见窗口的安装进程不算“没有动静” | `internal/queue`、`internal/api/tasks.go`、`frontend/src/components/dialogs/tasks-sheet.tsx` |

### P2 验收

- 新测试：`procs`（`close_test.go`、窗口不算空闲）、`core`（`TestUpgradeWaitsForTheInstallerAndRetriesWithAWindow`）、`queue`（`TestRetryUpdateWithAWindow`）、`api`（`TestUpdateClosesTheProgramFirst`）。
- 用户真实验收：
  1. 打开一个用户范围的 winget 软件，在本机已装页手动更新：它被关闭，任务日志有“先关闭它再更新”“已关闭”，更新成功。
  2. 在一个有未保存内容的软件（如记事本类）里改点东西再更新：它弹出是否保存时，任务显示“等待你关闭…”，窗口被提到前面；不处理的话 2 分钟后任务失败“还开着……关闭它后重试”，内容没有丢。
  3. 让某个 winget 更新静默失败后，任务里出现“打开安装程序重试”，点它会打开安装程序窗口。
  4. Windows Installer 忙（如另一个 MSI 正在装）时开始更新，任务显示“等待另一个安装或卸载结束”，结束后自动开始。

## P3（本次实施）

| # | 做法 | 位置 |
|---|---|---|
| 9 | 用户发起的任务（包括重试）插到第一个等待中的自动更新前面；正在执行的不打断 | `internal/queue`（`enqueueLocked`） |
| 10 | `Tasks.Cancel`：等待项直接记为“已取消，没有开始”；运行项只在来源报告 `Progress.Cancelable`（下载中、等 Windows Installer、等你关闭软件）时结束这一项的 ctx，复查发现其实已装好 / 更新好就算成功。winget COM 不能直接结束进程（winget 服务会继续），改为脚本看到取消文件后 `Stop-Job`，30 秒不停才结束；`runner` 结束命令时用 `taskkill /T` 结束整个进程树（scoop 的 aria2、winget 命令行的安装器）；GitHub 下载完后、启动安装程序前再看一次。已取消的更新在更新记录里记为“跳过”，不计入连续失败。任务面板对 `cancelable` 的任务显示“取消” | `internal/queue`、`internal/runner`、`internal/source/*`、`internal/api/tasks.go`、`tasks-sheet.tsx` |
| 11 | 冷却期改为按 **AppShelf 第一次发现这个版本** 计算：能自动更新的 winget、scoop 都拿不到可靠的发布时间（GitHub 有，但 GitHub 软件不自动更新）。每次检查后 `state.json` 的 `offered` 记下版本和第一次发现的时间；设置“新版本等几天再自动更新”（不等待 / 1 / 3 / 7 / 14 天）；未到期的列表项带 `autoAt`，不排队、不单独通知，详情显示“将自动更新：某时后”；到期由每分钟的 `tick` 排队，不必等下次检查；调短设置时立即重算 | `internal/state`、`internal/api/autoupdate.go`、`updates.go`、设置页、更新详情 |
| 12 | 成功但需要重启的任务（安装、更新、卸载）记进 `state.json` 的 `reboots`，读取时丢掉早于这次开机（`GetTickCount64`）的；任务面板顶部“重启电脑后才能完成：…”，托盘悬停提示“N 个软件需要重启电脑后生效”，队列都结束后发一次任务通知 | `internal/state`、`internal/procs`、`internal/api`、`tray.go`、`tasks-sheet.tsx` |

### P3 验收

- 新测试：`queue`（`TestUserItemsGoFirst`、`TestCancel`）、`core`（`TestCancelWhileWaitingForTheInstaller`）、`state`（`TestOffered`、`TestReboots`）、`api`（`TestCancelAnUpdate`、`TestRebootsAreTold`、`TestAutoDelay`）、`tray_test.go`（重启提示）；`TestAutoHold` 改为期望手动任务先执行，`com_test.go` 期望下载阶段可取消。
- 用户真实验收：
  1. 开着自动更新、排了几个时，手动点一个更新，它排在自动更新前面。
  2. 一个 winget 软件更新下载中点“取消”：任务变成“已取消”，软件版本没变，winget 没有在后台继续装（重新检查仍提示可更新）。scoop、GitHub 软件同样试一次。
  3. 设置“新版本等 3 天再自动更新”后，新发现的自动更新项详情显示“将自动更新：…后”，期间不自动装。
  4. 装一个需要重启的软件后，任务面板顶部和托盘提示需要重启；重启电脑后提示消失。
- 未验证的风险：winget COM 的 `Stop-Job` 能否真正取消 winget 的下载，需要真实更新验证；不能取消时 winget 会装完，复查后如实记为成功（“取消时已经更新好了”）。

另外，0x8A150114“不支持直接更新”只提供“卸载后重新安装”入口、卸载照常确认（原分析第 8 项，用户未表态，未做）。

## 不照搬

自动改 scope / 架构 / ID 参数重试、自动提权或批量提权、并发执行、CLI 与 IPC 接口。
