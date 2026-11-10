# internal/uninstall

从本机卸载程序。包管理器装的交给包管理器卸载（winget、scoop、应用商店应用用 `Remove-AppxPackage`），其余的运行注册表 Uninstall 项里的卸载命令；认得出安装程序类型时静默卸载，认不出时打开卸载程序的窗口，由用户在窗口里完成。

安装程序类型和静默参数的判断规则移植自 Bulk Crap Uninstaller（Apache-2.0），见 [references/external-repos/BCUninstaller__Bulk-Crap-Uninstaller](../../references/external-repos/BCUninstaller__Bulk-Crap-Uninstaller/README.md)。

## 接口

| 名称 | 作用 |
|---|---|
| `Target` | 要卸载的程序：`Key`（本机已装列表里的键）、名称、版本、发布者、安装目录、范围，卸载方式 `Method` 和它需要的字段（`WingetID`/`WingetSource`、`ScoopName`/`Global`、`PackageFullName`、注册表项 `Reg`），以及对应的清单软件 `CatalogIDs` |
| `PlanFor(t, interactive, opt)` | 只根据已读到的信息算出卸载方式：命令、是否静默、能否改用窗口重试、不能卸载的原因 `Blocked`、提示 `Notes`。不访问本机 |
| `New(opt, winget, scoop)` | 卸载执行者；没有 winget 或 scoop 时对应参数传 nil |
| `Runner.Inspect(ctx, targets)` | 在 `PlanFor` 之上读取本机现状：程序是否正在运行、应用包是不是框架包或 Windows 自带、卸载程序（或主程序）的数字签名。一次 PowerShell 读完所有目标 |
| `Runner.Run(ctx, t, interactive, log)` | 执行卸载，输出写进 `log`，阶段和等待状态用 `source.ReportProgress` 报告；等这次卸载的进程都结束后才返回，见下文“等待卸载完成”。用户取消（卸载程序里取消、拒绝管理员权限）时返回的错误包装 `ErrCanceled` |
| `WithCommands(ctx, ch)`、`Command` | 把用户在等待中的操作传给 `Run`：`StopWaiting`（停止等待，不结束卸载程序）、`ShowWindow`（把卸载程序的窗口提到前面）、`EndUninstaller`（结束卸载程序的进程）、`SwitchToWindow`（同上，由队列改用窗口重新执行） |
| `WaitWorking`、`WaitUser`、`WaitStalled`、`WaitInstaller` | `source.Progress.Wait` 的取值：正在等卸载程序结束、等用户操作、可能卡住、等另一个 Windows Installer 安装结束；`Prompt` 是卸载程序窗口的标题和文字，`Window` 表示有窗口可以提到前面 |
| `Runner.Present(ctx, t)` | 程序是否还在：winget / scoop 用各自的复查，应用包查 `Get-AppxPackage`，注册表程序查它的 Uninstall 项（`detect.Lookup`） |
| `Runner.Remains(t)` | 卸载后程序还在时补一句原因：“部分文件要重启电脑后才会删除”（`PendingFileRenameOperations` 里有它的目录），或“程序文件已经删除，但注册表里还留着它的记录” |
| `KindOf(e)`、`ProductCode(e)`、`SplitCommand(cmd)`、`KindText(k)` | 识别安装程序类型、取 MSI 产品代码、拆分卸载命令、类型的中文名 |

卸载是否成功不由 `Run` 的返回值决定，而由 `internal/queue` 执行后的复查（`Present`）决定。

## 卸载方式

| 方式 | 命令 | 静默 | 可改用窗口重试 |
|---|---|---|---|
| `ByWinget` | `winget uninstall --id <ID> --exact --source <来源> --silent` | 是 | 是（`--interactive`） |
| `ByScoop` | `scoop uninstall <名称> [--global]` | 是 | 否 |
| `ByStore` | `Remove-AppxPackage -Package <PackageFullName>` | 是 | 否 |
| `ByRegistry` | 见下表 | 视类型 | 视类型 |

注册表程序按类型选命令（Inno Setup 之外，注册表里有 `QuietUninstallString` 时优先用它；Inno Setup 的这一项常是 `/SILENT`，仍会显示进度窗口）：

| 类型 | 识别依据 | 静默命令 |
|---|---|---|
| Windows Installer | 卸载命令含 `msiexec`，或 `WindowsInstaller=1` 且键名是 GUID；产品代码取键名或命令里的 GUID，取不到时不卸载 | `msiexec /x {GUID} /qn /norestart`，为所有用户安装的先以 `runas` 请求管理员权限（`/qn` 下 msiexec 自己不会请求）；窗口重试为 `msiexec /x {GUID}` |
| WiX 安装包（Burn） | 卸载程序在 `Package Cache\{…}\` 下 | 追加 `/quiet /norestart` |
| Inno Setup | `unins000.exe` 这类文件名，且旁边有同名 `.dat` | 追加 `/VERYSILENT /SUPPRESSMSGBOXES /NORESTART` |
| NSIS | 卸载程序前 8 MiB 里有 `Nullsoft` 标记 | 追加 `/S`（命令已有 `_?=` 时插在它前面，`_?=` 必须在最后）。命令本身没有参数、文件名含 `uninst`、所在目录名不是 `uninstall` 时（BCU 的条件），复制到 `%TEMP%\AppShelf-uninstall-*\` 以 `/S _?=<原目录>` 运行副本（窗口模式为 `_?=<原目录>`），由它自己完成卸载并返回真实退出码，结束后删除副本；复制失败时直接运行原卸载程序 |
| InstallShield | 卸载命令在 `InstallShield Installation Information\{…}` 下 | 不静默，打开卸载程序窗口 |
| 其他 | — | 不静默，打开卸载程序窗口 |

退出码：

- 0 和 1605（产品不存在）视为完成；3010、1641 视为完成但需要重启。
- 1602、1223，以及 NSIS 的 1（取消按钮）和 2（脚本中止），判为用户取消（`ErrCanceled`）；NSIS 的 1627 视为正常（同 BCU）。
- 1618 是另一个安装正在进行：Windows Installer 和 WiX 安装包等它结束后自动再执行一次。
- Windows Installer 的 1603 报告“卸载时出错”；`0xC000013A` 报告“卸载程序被中止”；其余报告退出码。
- 拒绝管理员权限判为取消：“你拒绝了管理员权限，没有卸载”。

## 不会卸载的程序

- winget（`Microsoft.AppInstaller` / `Microsoft.DesktopAppInstaller`）和 scoop 本身：软件图书馆依赖它们。
- 软件图书馆自己（安装目录等于 `Options.SelfDir`）。
- 注册表标明 `NoRemove=1` 的程序。
- 框架包、Windows 自带或不可移除的应用包。
- 找不到注册表项、没有卸载命令、或 MSI 取不到产品代码的程序。

运行库和驱动（名称含 Redistributable、Runtime、.NET、WebView2、驱动等）可以卸载，但会提示其他软件可能依赖它；为所有用户安装的程序会提示可能请求管理员权限。

## 等待卸载完成

做法参考 BCU 的 `BulkUninstallEntry.UninstallThread()` 和 `TestUninstallerForStalls()`（`track.go`、`run.go`）：

- **追踪这次卸载的进程**：启动前取一次进程快照作为基线。属于这次卸载的进程包括：启动的卸载程序，被追踪进程的子进程（父进程已退出的也算，如 NSIS 的 `Au_.exe`），启动后新出现、名字或位置像卸载程序副本的进程（`Au_.exe`、`Un_A.exe`、`Un_B.exe`、`_isdel.exe`、扩展名为 `.tmp` 的程序、`\.cr\` 和 `\.be\` 下的程序），以及启动后新出现、位于程序自己安装目录下的进程。基线里已有的进程不算，浏览器、`explorer.exe`、`conhost.exe` 也不算。winget 经 COM 服务运行卸载程序，在调用 winget 前取基线，按名字、位置和它们的子进程追踪。
- **每秒检查一次**：被追踪的进程、它们的可见窗口（标题和窗口里的文字）、CPU 和读写的变化（CPU ≤ 1% 且读写 ≤ 10 KB/s 算空闲，同 BCU）。等待状态如下：

  | 状态 | 判断 |
  |---|---|
  | `WaitWorking` | 还有被追踪的进程在运行，阶段写出进程名 |
  | `WaitUser` | 窗口模式下有可见窗口；静默模式下出现可见窗口，且 10 秒没有活动（读不到活动时出现 10 秒即算） |
  | `WaitStalled` | 静默模式、没有窗口、所有可测进程连续 35 秒空闲；Windows Installer 和 WiX 安装包不判断（工作在系统服务里，测不到） |
  | `WaitInstaller` | Windows Installer、WiX 卸载开始前 `Global\_MSIExecute` 存在 |

- **结束等待**：进程都结束后，再给注册表项 5 秒的消失时间（winget 没有注册表项时用 winget 复查，等待期间最多每 10 秒问一次）；卸载程序已退出、程序也已消失时立即返回。用户停止等待、结束卸载程序时返回对应的错误；连续 30 分钟测不到任何活动、且不是在等用户时，自动停止等待（不结束进程）。不自动结束任何进程，也不点卸载程序的按钮。
- 状态变化写进日志，如 `[AppShelf] 卸载程序弹出了窗口，可能在等你操作：「App」请先关闭 App`。

## 谁用

`main.go` 用 `New(Options{SelfDir}, winget, scoop)` 创建，交给 `internal/core`（`Config.Uninstaller`）。`core` 用它预览卸载、作为队列执行卸载项、执行前后复查；`internal/api` 的 `Installed.PreviewUninstall` / `StartUninstall` 把本机已装列表的条目转成 `Target`。

## 测试

`uninstall_test.go`：

- 各类安装程序的命令和静默参数、`QuietUninstallString` 优先、窗口重试；不会卸载的程序；来源卸载的命令和提示；`SplitCommand`、`ProductCode`。
- `run_test.go`：用替换的进程表、窗口、活动、注册表和 Windows Installer 状态验证：
  - NSIS 从副本加 `_?=` 运行，副本随后删除。
  - 等待副本和安装目录下的新进程结束；别的软件的 `Au_.exe`、启动前就开着的程序、浏览器不会拖住卸载。
  - winget 之后等副本结束。
  - “等待你操作”“可能卡住”的判断（MSI 除外），以及停止等待、显示窗口、结束卸载程序、无权结束时的说明、长时间无动静和 `ctx` 结束。
  - 各退出码和取消，Windows Installer 忙时排队、1618 后重新执行一次，`Remains`、进程追踪规则和 NSIS 副本的条件。
- `TestPlansOfThisMachine`：为本机注册表里每个程序生成卸载方式，并对其中几个读取签名；`explorer.exe` 必须读出有效的 Windows 签名。只读，不卸载任何程序。
- `TestUninstallRealOptIn`：真实卸载一个程序，会改变本机。只有设置 `APPSHELF_UNINSTALL_TEST=1` 和 `APPSHELF_UNINSTALL_REG=<Machine|User>\<X64|X86>\<键名>`（用户选定的测试程序）时才运行，否则跳过。
