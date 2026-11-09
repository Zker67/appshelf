# internal/usage

读取用户上次打开每个已装程序的时间，供“卸载”页按“上次打开”挑选程序。结果只用于展示和筛选，**不参与是否已装、能否卸载的判断**。

## 数据来源

| 来源 | 内容 |
|---|---|
| UserAssist（`HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\UserAssist\{GUID}\Count`） | Windows 记录的、从开始菜单、任务栏、桌面和资源管理器启动过的程序及最后一次启动时间。值名用 ROT13 编码；Windows 7 起数据为 72 字节，第 60 字节起是最后一次运行的 FILETIME |
| 开始菜单和任务栏的快捷方式 | 解析 UserAssist 里记录的快捷方式，以及只记了 AppUserModelID 的启动对应哪个程序（快捷方式的 `System.AppUserModel.ID`） |
| 正在运行的进程（`internal/procs`） | 程序目录下有进程在运行，就当作读取时刻刚打开过 |

UserAssist 值名有三种：程序路径（可能以 Known Folder GUID 开头，用 `SHGetKnownFolderPath` 展开）、AppUserModelID（应用包是 `<包族名>!<App>`）、快捷方式路径。Windows 自己的计数器（`UEME_…`）忽略。

快捷方式用一次 Windows PowerShell 解析：`WScript.Shell` 取目标，`Shell.Application` 取 AppUserModelID，每个目录只打开一次。本机约 300 个快捷方式约 2 秒，`internal/inventory` 让它和 winget 并行读取。

不用的来源：BAM 需要管理员权限才能读；Prefetch 需要管理员；文件最后访问时间可能被系统关闭，也会被杀毒、索引更新，不可靠。

## 接口

| 名称 | 说明 |
|---|---|
| `Read(ctx) (Record, error)` | 读取启动记录和正在运行的进程。读不到的部分返回错误，已读到的照常返回。非 Windows 返回 `errors.ErrUnsupported` |
| `Record.Tracked()` | 有没有任何启动记录；没有通常是用户关闭了“允许 Windows 跟踪应用启动” |
| `Record.LastUsed(Program) time.Time` | 一个程序的上次打开时间，没有记录时为零值 |
| `Program` | 用来认出程序的信息：自己的目录 `Dirs`、应用包族名 `Family`、scoop 名称 `ScoopName` |

## 归属规则

- **目录**：启动的程序文件在 `Dirs` 之一下面。只认 `detect.OwnDirectory` 接受的目录，Program Files、用户目录等共享目录不算任何程序的。
- **应用包**：AppUserModelID 以 `<包族名>!` 开头，或程序文件在 `WindowsApps\<包名>_<版本>_<架构>_<资源>_<发布者>\` 下且包族名相同。
- **scoop**：路径含 `\scoop\apps\<名称>\`。
- 卸载程序本身（文件名以 `unins`、`uninst` 开头）的启动不算。
- 取所有匹配记录里最晚的时间；目录下有进程在运行时取读取时刻。

## 局限

- 只记录从开始菜单、任务栏、桌面、资源管理器启动的程序；开机自启、命令行、被其他程序拉起的不记录，所以“没有打开记录”不等于从没用过。
- 用户关闭“设置 › 隐私和安全性 › 常规 › 允许 Windows 跟踪应用启动”后没有任何记录；“卸载”页会提示。
- 只读当前用户的记录。

## 测试

`usage_test.go`：ROT13、FILETIME、值名解析和归属规则的单元测试；`TestReadThisMachine` 只读地读取本机记录并对上注册表里的程序（本机实测 205 条启动记录，209 个注册表程序中 75 个有上次打开时间）。
