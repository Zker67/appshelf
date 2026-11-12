# internal/procs

判断某个目录下的程序是否正在运行，用于自动更新前避免关掉用户正在使用的软件，以及在更新预览里提示“正在运行”；并为卸载提供进程追踪：进程的父子关系、活动、窗口，以及在用户要求时结束进程；还认出安装或更新留下仍在运行的安装进程，供复查前等待；以及更新前关闭软件。

| 文件 | 说明 |
|---|---|
| `procs.go` | `Under(paths, dir)`：进程的可执行文件是否位于目录内（不区分大小写，按完整目录名匹配，`C:\Git` 不会匹配 `C:\GitHub`）；类型 `Proc`、`Usage`、`Window` 和 `ErrDenied` |
| `procs_windows.go` | `Paths()`：用 ToolHelp 快照枚举进程，`QueryFullProcessImageName` 读取可执行文件路径；读不到的进程（其他用户、系统进程）跳过 |
| | `Snapshot()`：所有进程的 PID、父 PID 和文件名（包括其他用户和系统的）；`Path(pid)`：一个进程的可执行文件 |
| | `UsageOf(pid)`：进程至今用掉的处理器时间（`GetProcessTimes`）和读写字节数（`GetProcessIoCounters`） |
| | `Windows(pids)`：这些进程可见的顶层窗口（不含工具窗口和零大小的窗口）：标题，以及窗口里静态文字控件的内容（`WM_GETTEXT`，最多等 200 毫秒；以管理员身份运行的程序通常读不到） |
| | `Front(handle)`：把窗口提到前面，最小化的先还原；`End(pid)`：结束进程，没有权限时返回 `ErrDenied`；`CloseWindow(handle)`：向窗口发 `WM_CLOSE`（同点关闭按钮），不等待；`BootTime()`：Windows 这次开机的时间（`GetTickCount64`），用于判断待重启的任务是否已完成 |
| `installers.go` | `WatchInstallers(sys, TempDirs())` 在安装或更新开始前记下已有进程；`Alive()` 只认之后新启动、在临时目录（用户的 `%TEMP%`、`%SystemRoot%\Temp`、`SystemTemp`）里运行，或是 `.tmp` 程序、位于 `\.cr\`、`\.be\`（WiX）、`\SquirrelTemp\` 的进程；浏览器、`explorer.exe`、`conhost.exe` 不算。不按父子关系认，避免等到安装程序最后打开的软件本身 |
| | `Wait(ctx, report)`：没有这类进程就立刻返回；有就每秒看一次，变化时 `report`，全部结束后再等 3 秒（链式安装的下一步）；2 分钟没有处理器和磁盘活动（读不到活动视为在工作）返回 `WaitIdle`，30 分钟返回 `WaitTimeout`；`Times` 可在测试中改短；`System.Windows` 设置时，有可见窗口的安装进程（可能在等用户操作）始终算在工作。`Names` 把进程名去重、最多列 3 个 |
| `close.go` | `NewCloser(LiveClose()).Close(ctx, dir, report)`：关闭可执行文件在 `dir` 下的进程（AppShelf 自己除外，读不到路径的不管）：先 `CloseWindow` 它们的可见窗口并等 10 秒（没有窗口就不等），仍在运行且已没有窗口的 `End`；窗口还开着的不结束，`Front` 提到前面、报告 `CloseWaiting`，等 2 分钟；返回找到的、没有权限结束的（`Denied`）和一直没关的窗口（`Open`）。全部关掉后再等 1 秒让文件释放 |
| `procs_other.go` | 非 Windows 平台返回错误或空结果 |

`End` 只在用户点了“结束卸载程序”或“改用窗口卸载”后由 `internal/uninstall` 调用，其余都只读。读不到软件的安装目录时，调用方应视为“无法确认”，而不是“没有运行”。

测试：`procs_test.go` 读真实进程，找到测试程序自己和它的父进程、读出自己的处理器时间、枚举窗口，并结束一个自己启动的 `ping.exe`。`installers_test.go` 用假快照覆盖认定规则、PID 复用、链式安装、无动静、超时和取消，并从临时目录运行一份 `ping.exe` 副本，确认真实进程能被认出并等到结束。`close_test.go` 用假桌面覆盖关窗即退、关窗后留在托盘被结束、窗口一直开着、用户中途关掉、没有权限、取消、不关自己，并结束一个从临时目录运行、没有窗口的真实 `ping.exe` 副本。
