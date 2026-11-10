# 卸载等待与判断更人性化

状态：`[in-progress]`：已实现并通过自动检查，等待用户真实卸载验收。

## 背景

卸载成败 AppShelf 已经比 BCU 判断得稳：执行前复查，以执行后程序是否还在为准。落后的是等待过程：

- 窗口模式下 `runner.ShellRun` 用 `WaitForSingleObject(INFINITE)` 等卸载程序退出，不接 `ctx`；用户不操作窗口时队列一直卡住，“停止”打断不了正在运行的项。
- `settle()` 按文件名猜卸载程序副本是否在运行：本机任何一个 `Au_.exe`、`unins*.tmp`，或者安装目录下用户自己开着的程序，都会让它等满 30 分钟；WiX Burn 的 `%TEMP%\{GUID}\.cr\` 副本、InstallShield 的 `_isdel.exe` 等认不出来。
- 等待时只写一句“卸载程序还在运行”，看不出在等谁、是否卡住、是否要用户操作。
- NSIS 卸载程序把自己复制到临时目录后立即以 0 退出，真实退出码（用户取消等）拿不到；注册表命令已带 `_?=` 时 `/S` 被追加在它后面，而 `_?=` 必须是最后一个参数，`/S` 会被当成目录的一部分。
- 用户取消、拒绝管理员权限都显示成红色的“失败”。
- 另一个安装正在进行（1618）时直接报错。

## BCU 源码核对（commit `30da609`）

| 项 | 源码 | 结论 |
|---|---|---|
| 进程追踪 | `BulkUninstallEntry.UninstallThread()` | 启动前记下已有进程 ID；不断加入被追踪进程的子进程；MSI 另看 `msiexec` 的子进程；去掉已结束、启动前就有的、浏览器（`NamesOfIgnoredProcesses`）；都结束后，再按安装目录找启动后新出现的进程 |
| NSIS 真实退出码 | `UninstallManager.UpdateNsisStartInfo()` | 把卸载程序复制到 `%TEMP%`，以 `_?=<原目录>` 运行副本。条件：原命令没有参数、文件名含 `uninst`、所在目录名不是 `uninstall`。只用于普通（非静默）卸载 |
| 退出码 | `UninstallThread()` | MSI 1602、NSIS 1 和 2 判为用户取消；NSIS 1627 视为正常；`0xC000013A` 判为被中止；其余报告退出码 |
| 卡住检测 | `TestUninstallerForStalls()` | 每个进程 CPU ≤ 1% 且 IO ≤ 10240 B/s，所有可测进程都这样才算一次空闲；每次采样约 1.1 秒，连续超过 30 次就结束进程并报超时。只在静默卸载时检测 |
| 重试 | `UninstallThread()` finally | `RetryFailedQuiet` 只是重新排队、不再自动点按钮，不会从静默改成普通卸载（`references` 里“静默失败可退回交互式”的说法有误，一并改正） |
| 调度 | `BulkUninstallTask.CheckFor*Collisions()` | MSI、InstallShield 等不并发；同发布商、目录互相包含、已有多个 `msiexec` 时不并发。AppShelf 一次只跑一项，不需要 |

## 不变量

- 卸载成败仍只由执行后的复查（`Present`）决定；新增的等待状态、退出码解释只影响等待方式和提示文字。
- 不自动点击卸载程序的按钮，不自动结束任何进程：结束卸载程序只在用户点击后执行。
- 卸载对象和命令仍只来自 Go 读到的本机已装列表；前端只传任务 ID 和动作。
- 安装、更新任务不受影响。

## 方案

### 一、追踪这次卸载的进程（`internal/procs`、`internal/uninstall/track.go`）

- `procs` 新增 `Snapshot()`（PID、父 PID、文件名）、`Path(pid)`、`UsageOf(pid)`（CPU 时间、IO 字节数）、`Windows(pids)`（进程的可见顶层窗口：标题和窗口里的文字）、`Front(窗口)`、`End(pid)`。
- 启动卸载前取一次快照作为基线。属于这次卸载的进程：启动的卸载程序；被追踪进程的子进程（父进程已退出也认）；启动后新出现的、名字或位置像卸载程序副本的进程（`Au_.exe`、`Un_A.exe`、`Un_B.exe`、`_isdel.exe`、扩展名为 `.tmp` 的程序、`\.cr\` / `\.be\` 下的程序）；启动后新出现的、位于程序自己安装目录下的进程。
- 基线里已有的进程一律不算（不再被别的软件的 `Au_.exe` 或用户开着的程序拖住）；浏览器、`explorer.exe`、`conhost.exe` 不算（卸载程序常在结束时打开反馈网页）。
- winget 经 COM 运行卸载程序，不是 AppShelf 的子进程：在调用 winget 前取基线，之后按“新出现 + 像副本或在安装目录下 + 它们的子进程”追踪。

### 二、等待：看得见、停得下（`internal/runner`、`internal/uninstall/run.go`）

- `runner.ShellStart` 启动后返回进程句柄，不再阻塞；`ShellRun` 改为基于它实现，行为不变。
- 卸载程序启动后每秒检查一次：被追踪的进程、它们的窗口、CPU 和 IO 的变化。等待状态：

| 状态 | 判断 | 任务里显示 |
|---|---|---|
| 正在等待卸载程序结束 | 还有被追踪的进程在运行 | “正在等待卸载程序结束（Au_.exe）” |
| 等待你操作 | 窗口模式下卸载程序有可见窗口；静默模式下出现可见窗口且 10 秒没有活动 | “等待你在卸载窗口里完成操作”，或“卸载程序弹出了窗口，可能在等你操作”，并显示窗口标题和文字 |
| 可能卡住 | 静默模式、没有窗口、所有可测进程连续 35 秒 CPU ≤ 1% 且 IO ≤ 10 KB/s（参考 BCU）；Windows Installer 和 WiX 安装包不判断（工作在系统服务里，测不到） | “卸载程序长时间没有动静，可能卡住了” |
| 等待另一个安装结束 | MSI / WiX 卸载前 `Global\_MSIExecute` 存在 | “等待另一个安装或卸载结束” |

- 等待中可以：
  - **停止等待**：不再等，立即复查；不结束卸载程序。
  - **显示窗口**：把卸载程序的窗口提到前台。
  - **改用窗口卸载**（静默卸载、可以改用窗口时）：结束这次的卸载程序，程序还在就自动以窗口方式重新排队。
  - **结束卸载程序**（可能卡住、不能改用窗口时）。以管理员身份运行的进程 AppShelf 结束不了，会在日志和阶段里说明，请用户在它的窗口里关闭。
- 不再有固定 30 分钟上限：连续 30 分钟测不到任何活动、且不是“等待你操作”时才自动停止等待（只停止等待，不结束进程）。
- 状态变化写进日志，例如 `[AppShelf] 卸载程序弹出了窗口「Upscayl 卸载」：请先关闭 Upscayl……`。
- winget 没有注册表项时，等待期间的 winget 复查最多每 10 秒一次。

### 三、NSIS 与退出码（`internal/uninstall/plan.go`、`run.go`）

- NSIS（按 BCU 的条件：原命令没有参数、文件名含 `uninst`、目录名不是 `uninstall`）：复制到 `%TEMP%\AppShelf-uninstall-*\`，以 `/S _?=<原目录>`（窗口模式为 `_?=<原目录>`）运行副本，拿到真实退出码；结束后删除副本目录。这与 NSIS 自己复制到临时目录运行的方式相同。静默和窗口模式都用（BCU 只用于窗口模式，因为它的静默模式靠自动点按钮）。
- 原命令已带 `_?=` 时，`/S` 插在 `_?=` 之前。
- 退出码：
  - 0 成功。
  - 1605 视为已不存在。
  - 3010 和 1641 成功但需要重启。
  - 1602、1223，以及 NSIS 的 1 和 2，判为用户取消。
  - NSIS 的 1627 视为正常。
  - 1618 先等另一个安装结束，再自动重新执行一次。
  - 1603 报告“Windows Installer 卸载出错”。
  - `0xC000013A` 报告“卸载程序被中止”。
  - 拒绝管理员权限判为取消：“你拒绝了管理员权限，没有卸载”。
- 取消用 `uninstall.ErrCanceled` 表示。程序还在时，任务仍是失败，但标记为“已取消”：界面显示灰色的“已取消”，不再显示红色，不计入失败数。

### 四、复查与调度（`internal/uninstall`、`internal/core`）

- 卸载后程序还在时，补一句原因（`Runner.Remains`）：“程序文件已经删除，但注册表里还留着它的记录”，或者“部分文件要重启电脑后才会删除”（`PendingFileRenameOperations` 里有它的目录）。
- 一次卸载多个时，能静默卸载的排在前面，需要打开窗口的排到最后。

### 五、提醒（`internal/api`、`main.go`、前端）

- 进入“等待你操作”时：AppShelf 窗口不在前台，发系统通知“「X」的卸载程序在等你操作”，点击打开任务面板；窗口在前台，弹出页面提示，带“显示窗口”按钮。每个任务只提醒一次。
- 任务面板在进行中的卸载下显示窗口文字，以及上述按钮。

## 接口变化

- `procs`：`Proc`、`Snapshot`、`Path`、`Usage`/`UsageOf`、`Window`/`Windows`、`Front`、`End`。
- `runner`：`ShellStart`、`Process`。
- `source.Progress`：`Wait`、`Prompt`、`Window`。
- `uninstall`：`Command`（`StopWaiting`、`ShowWindow`、`EndUninstaller`、`SwitchToWindow`）、`WithCommands`、`ErrCanceled`、`Runner.Remains`、`Plan.Copy`；等待状态常量 `WaitWorking`、`WaitUser`、`WaitStalled`、`WaitInstaller`。
- `queue`：`Item.Control`、`Item.Canceled`、`Queue.Control(id, cmd)`。
- `core.Uninstaller` 增加 `Remains`。
- `api`：`TaskEntry` 增加 `wait`、`prompt`、`window`、`canSwitch`、`canceled`；`Tasks.Control(id, TaskControl)`；`Hooks.NotifyTasks`。

## 验收

- `internal/procs`：真实进程快照里能找到测试进程自己和父进程；`UsageOf` 读得出自己的 CPU 时间；`End` 能结束测试启动的子进程。
- `internal/uninstall`，用替换的系统函数验证：
  - 别的软件的 `Au_.exe`、启动前就开着的程序不会让卸载等待。
  - 卸载程序的子进程、父进程已退出的副本，会一直等到它们结束。
  - 窗口模式下有窗口时显示“等待你操作”；静默模式出现窗口且空闲时也显示；静默模式没有窗口、长时间空闲时显示“可能卡住”，MSI 不判断。
  - 停止等待立即返回；结束卸载程序会结束被追踪的进程；`ctx` 结束时返回。
  - NSIS 用副本加 `_?=` 运行，`/S` 在 `_?=` 前面；不满足条件的不复制。
  - 各退出码的解释；取消返回 `ErrCanceled`；1618 等待后重新执行一次。
  - Windows Installer 忙时先等待，忙完才开始；停止等待则不卸载。
- `internal/queue`：取消的卸载标记为已取消；`Control` 只对正在运行的卸载有效；“改用窗口卸载”结束后程序还在时自动以窗口方式重新排队，已不在时判为成功。
- `internal/api`：任务列表带出等待状态和按钮条件；进入“等待你操作”只通知一次。
- 静默优先排序：同一批里需要窗口的排在最后。
- 自动检查：`gofmt -l .`、`go vet ./...`、`go tool mygo vet`、`CGO_ENABLED=0 go test ./...`、`go tool mygo generate`、`bun run --cwd frontend check`、`bun run --cwd frontend test`、`go tool mygo build`。
- 用户验收（真实卸载，需要用户选定测试软件）：
  - NSIS 软件静默卸载后立即消失。
  - 窗口模式下点取消显示“已取消”。
  - 开着软件做静默卸载，能看到“可能在等你操作”和窗口文字，“改用窗口卸载”可用。
  - “停止等待”立即结束等待。

## 回滚

各部分相互独立。`procs`、`runner` 只是新增函数（`ShellRun` 保持原行为）；还原 `internal/uninstall/run.go`、`track.go` 即回到按文件名等待；前端按钮依赖 `TaskEntry` 的新字段，没有这些字段时不显示。

## 实施结果（2026-11-10）

- 已实现方案一到五，接口与上文一致。新增文件：`internal/uninstall/track.go`、`remains_windows.go`、`remains_other.go`、`run_test.go`，以及 `frontend/src/lib/use-wait-toasts.ts`。
- 自动检查全部通过：`gofmt`（改动的文件）、`go vet ./...`、`go tool mygo vet`、`CGO_ENABLED=0 go test ./...`、`go tool mygo generate`、`vp check src`、`bun run --cwd frontend test`、`go tool mygo build`。
  - `bun run --cwd frontend check` 只报 `frontend/package.json` 的格式问题。这个文件没有改动，原因是工作区副本是 CRLF 换行。
- `procs` 的真实进程测试已在本机通过：快照、处理器时间、窗口枚举（读到 28 个窗口），以及结束自己启动的 `ping.exe`。
- 未做：
  - 真实卸载验收（需要用户选定测试软件）。
  - 界面效果由用户在真实窗口检查。
  - `CHANGELOG.md` 留到发布时写。
