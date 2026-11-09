# 卸载页按上次打开时间挑选

状态：`[in-progress]`：实现与自动检查完成后，等待用户在真实窗口验收。

## 用户诉求

- 卸载页挑选程序主要看两件事：占用空间和上次打开时间。安装时间不是挑选依据。

## 数据来源

Windows 没有一个“程序上次打开时间”的统一记录。可用的来源和取舍：

| 来源 | 结论 |
|---|---|
| UserAssist（`HKCU\…\Explorer\UserAssist\{GUID}\Count`） | **采用**。当前用户可读，无需管理员；记录从开始菜单、任务栏、桌面、资源管理器启动的程序及时间。值名 ROT13 编码，数据第 60 字节起是 FILETIME。`{CEBFF5CD-…}` 是程序路径或 AppUserModelID，`{F4E57C4B-…}` 是快捷方式 |
| 正在运行的进程 | **采用**。程序目录下有进程在运行即视为“现在” |
| BAM（`HKLM\SYSTEM\…\bam\State\UserSettings`） | 不用：普通用户无权读取 |
| Prefetch | 不用：需要管理员权限 |
| 文件最后访问时间 | 不用：可被系统关闭，杀毒和索引也会更新它，不可靠 |

UserAssist 里的条目有三种：

1. 程序路径，可能以 Known Folder GUID 开头（如 `{6D809377-…}\App\app.exe`），用 `SHGetKnownFolderPath` 展开。
2. AppUserModelID：应用包是 `<PackageFamilyName>!<App>`；桌面程序自带的（如 `Chrome`、`com.xxx.desktop`）靠开始菜单快捷方式的 `System.AppUserModel.ID` 对应到程序文件。
3. 快捷方式 `.lnk`：解析出目标程序。

快捷方式解析用一次 PowerShell（`WScript.Shell` 取目标、`Shell.Application` 取 AppUserModelID），本机约 300 个快捷方式约 2 秒，与 winget 读取并行。

## 归属到程序

- 程序自己的目录：注册表 `InstallLocation`、`DisplayIcon` 和卸载命令所在目录，只取 `detect.OwnDirectory` 认可的目录；程序文件在目录下即算这个程序。卸载程序本身（`unins*`、`uninst*`）的启动不算。
- 应用包：AppUserModelID 以 `<PackageFamilyName>!` 开头，或路径在 `WindowsApps\<包目录>\` 下且包目录属于该包族。
- scoop：路径含 `\scoop\apps\<名称>\`。
- 取所有匹配记录里最晚的时间；目录下有进程在运行时取读取时刻。

## 局限（界面上说明）

- 只记录从开始菜单、任务栏、桌面、资源管理器启动的程序；开机自启、命令行、被其他程序拉起的不记录。“没有打开记录”不等于从没用过。
- 用户关闭了“允许 Windows 跟踪应用启动”时没有任何记录，卸载页给出提示。

## 界面

- 左侧栏“安装时间”组换成“上次打开”：一个月内、1 到 6 个月前、半年以上没打开、没有打开记录。
- 排序：占用空间、久未打开（最早打开的在前，没有记录的最后）、最近打开、名称。
- 每行右侧显示“x 天前打开”和日期（正在运行的算“今天打开”）；没有记录显示“没有打开记录”。
- Windows 没有任何启动记录时，左侧“上次打开”组下提示去设置里打开“允许 Windows 跟踪应用启动”。

## 实现清单

- `internal/usage`（新模块，含中文 README）：读取 UserAssist、展开 Known Folder、解析快捷方式、正在运行的进程；按目录 / 包族 / scoop 名称算上次打开时间。单元测试 + 本机只读测试。
- `internal/inventory`：`Sources.Usage`，与其他来源并行读取；`Entry.LastUsed`；`List.UsageTracked`。
- `internal/api`：`InstalledEntry.lastUsed`、`InstalledList.usageTracked`；`go tool mygo generate`。
- `frontend/`：`lib/uninstall.ts`（`useBand`、排序）、`pages/uninstall/index.tsx`；测试。
- 文档：`internal/inventory/README.md`、`internal/api/README.md` / `docs/api.md`（若列出字段）、`plans/README.md`、`AGENTS.md` 项目定位里的“按占用空间和安装时间”。

## 不变量

- 上次打开时间只用于展示和筛选，不影响是否已装、能否卸载的判断。
- 读不到使用记录时列表照常显示，只是没有上次打开时间。

## 验收

- `gofmt -l`、`go vet ./...`、`CGO_ENABLED=0 go test ./...` 通过；本机只读测试能读到 UserAssist 记录并对上常用程序。
- `npm run check`、`npm test` 通过；`go tool mygo build` 能打包。
- 用户在真实窗口确认：常用软件显示最近打开，久未用的软件排在“久未打开”前面。

## 回滚

删除 `internal/usage`、`Sources.Usage` 和 `lastUsed` 字段，前端恢复“安装时间”分组。
