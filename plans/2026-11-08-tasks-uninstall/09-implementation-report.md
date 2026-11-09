# 实施报告：任务进度、日志、卸载与残留清理

状态：`[in-progress]`：四期都已实现，并通过了自动检查和本机只读测试。还没有做的有三项：真实卸载，需要用户指定一个测试程序；真实窗口验收；scoop 实测，本机没有 scoop。

## 实际改动

| 阶段 | 改动 |
|---|---|
| 1 进度与日志 | 新增 `internal/tasklog`：日志原样写进文件，同时整理成行留在内存，处理 `\r` 重绘、CRLF、ANSI 和 spinner，最多保留 2000 行。<br>`source.Progress` / `ReportProgress`。<br>队列有了阶段（检查、开始、复查）、进度通知（250 毫秒节流）和 `Queue.Log`。<br>winget 看到下载地址时进入“正在下载”，并统计 `%TEMP%\WinGet` 下的大小；scoop 按英文行首识别阶段；GitHub 统计下载字节。<br>`Tasks.Log`；`TaskEntry` 增加 `stage`、`done`、`total`。<br>前端：任务卡片显示阶段、进度条和用时；内嵌日志可跟随、复制，错误行高亮，失败的任务默认展开；标题栏有进度环 |
| 2 读取现代化 | `detect.Entry` 读出卸载命令、静默卸载命令、`WindowsInstaller`、`NoRemove`、大小、安装日期和安装来源；缺安装位置时从卸载程序或图标推断（`location.go`）；`detect.Lookup` 重读单个键。<br>只在注册表里的程序改用 winget 的 `arp:` 键。<br>`InstalledList.partial`：先显示注册表结果，再补全。<br>新增 `internal/watch`：监听 Uninstall 键、应用包仓库键和 scoop `apps` 目录，变化合并 1.5 秒后重读。<br>前端：按名称、占用空间、安装日期排序，宽窗口显示大小和日期，补全期间显示“正在补充来源” |
| 3 卸载 | 新增 `internal/uninstall`：`PlanFor` 识别 MSI、WiX（Burn）、Inno Setup、NSIS、InstallShield 和静默参数，并拦下不能卸载的程序；`Inspect` 读运行中的进程、应用包属性和签名；`Run` 等卸载程序的副本结束；`Present` 复查。<br>winget、scoop 各增加 `Uninstall`。<br>队列新增 `Uninstall` 动作、`judgeRemoval` 和 `RetryInteractive`。<br>`core` 增加 `PreviewUninstall`、`EnqueueUninstall`。<br>`Installed.PreviewUninstall` / `StartUninstall`。<br>前端：卸载预览弹窗；本机已装悬停和多选栏有“卸载”；清单右键“从本机卸载…”；任务卡片有“打开卸载程序重试” |
| 4 残留 | 新增 `internal/leftover`：查找目录、`HKCU\Software` 下的键、快捷方式、开机启动项和 scoop `persist`，并按理由打分；有保护规则；清理时文件进回收站（`SHFileOperationW` + `FOF_ALLOWUNDO`），注册表先 `reg export` 再删除。<br>`Tasks.FindLeftovers` / `CleanLeftovers` / `OpenBackup`：结果留在服务端，前端只能按 ID 选。<br>前端：残留弹窗，按等级分组，每项显示理由和分数，清理后逐项显示结果，可打开备份；任务卡片有“查找残留” |
| 文档 | 新增 `internal/tasklog`、`internal/watch`、`internal/uninstall`、`internal/leftover` 的 README，以及 BCU 外部参考。<br>更新 `internal/README.md`、`internal/api/README.md`、`internal/source/README.md`、`internal/inventory/README.md`、`frontend/README.md`、`docs/api.md`（接口、防误删流程、备份位置）、`docs/operations.md`（数据位置、卸载测试变量）、`docs/architecture/project-structure.md`（分层）、`AGENTS.md`（“绝不误删”不变量）、`README.md`、`CHANGELOG.md`（未发布） |

## 追加：独立的卸载页（阶段 5）

用户试用后，卸载改为有自己的页签，见 [05-uninstall-page.md](./05-uninstall-page.md)：

- 按占用空间、安装时间、来源组合筛选；
- 默认按占用空间排序；
- 每行有大小条、安装日期，以及常驻的“卸载”按钮；
- 多选时显示合计大小；
- “最近卸载”里可以查找残留和重试；
- 「本机已装」去掉了悬停才出现的卸载图标。

验证：`bun run --cwd frontend check` 通过，`bun run --cwd frontend test` 61 项通过（新增 7 项卸载页逻辑测试）。

## 验证结果（2026-11-09）

| 检查 | 结果 |
|---|---|
| `gofmt -l .` | 无输出 |
| `go vet ./...`、`go tool mygo vet` | 通过 |
| `CGO_ENABLED=0 go test ./...` | 全部通过 |
| 真实只读：注册表 | 本机 209 个程序都有卸载命令，其中 47 个的安装位置是推断出来的 |
| 真实只读：卸载方式 | 为本机全部 209 个程序生成卸载方式：Burn 24、Inno 29、MSI 32、NSIS 70、其他 53；163 个可以静默卸载，1 个不能卸载。`explorer.exe` 读出有效签名，签名者为 Microsoft Windows |
| 真实环境：监听 | 在 `HKCU\Software\AppShelfTest-*` 下建键、改值，以及建临时子目录，都收到了通知 |
| 真实环境：残留 | 在 `%LOCALAPPDATA%\AppShelfTest-*` 和 `HKCU\Software\AppShelfTest-*` 下造数据。查找结果的分数符合预期，受保护的项没有出现。清理后文件进了回收站，注册表有备份且键已删除。测试文件留在回收站里 |
| `bun run --cwd frontend check` | 71 个文件格式正确；62 个文件没有 lint 和类型错误 |
| `bun run --cwd frontend test` | 54 项通过 |
| `go tool mygo build` | 生成 `build/windows-amd64/AppShelf.exe` 和 `appshelf-setup-0.3.0-amd64.exe` |

没有运行真实卸载：需要设置 `APPSHELF_UNINSTALL_TEST=1` 和 `APPSHELF_UNINSTALL_REG`，并由用户指定测试程序。界面效果没有截图自验，由用户在真实窗口里检查。

## 偏离原计划的地方

| 计划 | 实际 | 原因 |
|---|---|---|
| 命令含 `Package Cache\{GUID}` 的程序按 MSI 处理（03） | 单独识别为 WiX 安装包（Burn），追加 `/quiet /norestart` | Package Cache 里的是 Burn 安装包的 exe，不是 MSI 产品代码 |
| 不提 InstallShield | 识别出来，但不静默，打开卸载程序窗口 | 它的静默卸载需要应答文件，猜测参数不可靠 |
| 来源的 `Uninstall(ctx, app, interactive, log)`（03） | winget 为 `Uninstall(ctx, id, source, interactive, log)`，scoop 为 `Uninstall(ctx, name, global, log)` | 卸载对象来自本机列表，不是清单里的软件 |
| 真实卸载测试用 `APPSHELF_UNINSTALL_KEY=<本机已装键>`（03） | `APPSHELF_UNINSTALL_REG=<Machine\|User>\<X64\|X86>\<键名>` | 测试直接在 `internal/uninstall` 里按注册表项运行，不需要整个应用读完本机列表 |
| 清单右键“卸载” | 叫“从本机卸载…” | 避免误解为把软件从清单移除 |
| 本机已装行菜单“卸载”（03） | 卸载有自己的页签（阶段 5）；本机已装只在多选栏保留“卸载” | 用户试用后要求：卸载看大小和安装时间，不看更新 |
| 打分理由（04） | 另加“scoop 为它保留的数据 +4”“里面只剩它的快捷方式 +4”“只剩这个软件的设置 +2” | 让 scoop 的 `persist` 目录、开始菜单文件夹和发布者键能达到“很可能” |
| 卸载程序的临时副本 `Au_.exe`、`Un_A.exe`、`_iu*.tmp` | 另加 `Un_B.exe` 和 `unins*.tmp` | 覆盖 NSIS 和 Inno Setup 的其他副本名 |

## 待用户验收

1. 安装一个软件：任务面板有阶段、进度和用时，日志实时滚动；往上翻时暂停跟随。
2. 第一次打开“本机已装”时列表立即出现，随后补上来源；在软件图书馆之外装卸一个软件后，列表自动刷新。
3. 选一个测试软件卸载：预览里的方式、命令、签名和提示正确，卸载后任务成功。然后“查找残留”，勾选后清理，检查回收站和备份。
