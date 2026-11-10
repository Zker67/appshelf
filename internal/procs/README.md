# internal/procs

判断某个目录下的程序是否正在运行，用于自动更新前避免关掉用户正在使用的软件，以及在更新预览里提示“正在运行”；并为卸载提供进程追踪：进程的父子关系、活动、窗口，以及在用户要求时结束进程。

| 文件 | 说明 |
|---|---|
| `procs.go` | `Under(paths, dir)`：进程的可执行文件是否位于目录内（不区分大小写，按完整目录名匹配，`C:\Git` 不会匹配 `C:\GitHub`）；类型 `Proc`、`Usage`、`Window` 和 `ErrDenied` |
| `procs_windows.go` | `Paths()`：用 ToolHelp 快照枚举进程，`QueryFullProcessImageName` 读取可执行文件路径；读不到的进程（其他用户、系统进程）跳过 |
| | `Snapshot()`：所有进程的 PID、父 PID 和文件名（包括其他用户和系统的）；`Path(pid)`：一个进程的可执行文件 |
| | `UsageOf(pid)`：进程至今用掉的处理器时间（`GetProcessTimes`）和读写字节数（`GetProcessIoCounters`） |
| | `Windows(pids)`：这些进程可见的顶层窗口（不含工具窗口和零大小的窗口）：标题，以及窗口里静态文字控件的内容（`WM_GETTEXT`，最多等 200 毫秒；以管理员身份运行的程序通常读不到） |
| | `Front(handle)`：把窗口提到前面，最小化的先还原；`End(pid)`：结束进程，没有权限时返回 `ErrDenied` |
| `procs_other.go` | 非 Windows 平台返回错误或空结果 |

`End` 只在用户点了“结束卸载程序”或“改用窗口卸载”后由 `internal/uninstall` 调用，其余都只读。读不到软件的安装目录时，调用方应视为“无法确认”，而不是“没有运行”。

测试：`procs_test.go` 读真实进程，找到测试程序自己和它的父进程、读出自己的处理器时间、枚举窗口，并结束一个自己启动的 `ping.exe`。
