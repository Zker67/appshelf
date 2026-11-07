# internal/procs

判断某个目录下的程序是否正在运行，用于自动更新前避免关掉用户正在使用的软件，以及在更新预览里提示“正在运行”。

| 文件 | 说明 |
|---|---|
| `procs.go` | `Under(paths, dir)`：进程的可执行文件是否位于目录内（不区分大小写，按完整目录名匹配，`C:\Git` 不会匹配 `C:\GitHub`） |
| `procs_windows.go` | `Paths()`：用 ToolHelp 快照枚举进程，`QueryFullProcessImageName` 读取可执行文件路径；读不到的进程（其他用户、系统进程）跳过 |
| `procs_other.go` | 非 Windows 平台返回错误 |

只读，不结束任何进程。读不到软件的安装目录时，调用方应视为“无法确认”，而不是“没有运行”。
