# winget 安装、更新、卸载改走 COM 接口，显示真实进度

状态：`[in-progress]`

## 用户诉求

- 安装、更新时任务面板只有一直流动的进度条，看不到真实进度。用户选择改走 winget 的 COM 接口。
- 卸载能有进度的也显示。

## 现状与原因（2026-11-09 实测）

- winget 命令行的输出被重定向时不画进度条，只有本地化的阶段文字；`Total` 从来没有。
- “已下载”靠量 `%TEMP%\WinGet\<id>.<版本>` 的大小。本机 winget 用 Delivery Optimization 下载（临时文件 `DO*.tmp`），文件先是 0，几秒后一次预分配到约 160 MiB（安装包 170 MB），量出来的不是真实进度。
- UniGetUI（commit `e857b3f5`）的 winget 安装、更新、卸载同样走命令行（`WinGetPkgOperationHelper` 不实现 `IManagedPackageOperationHelper`），winget 只有阶段、没有百分比。借鉴它的原则：没有真实数值就显示不确定进度；下载和安装不拼成一个虚构的整体百分比；成败看结果码，不看进度。

## COM 接口的取法（实测）

- `Microsoft.WinGet.Client` 模块（AppShelf 已依赖，1.29.380）的 `Install-` / `Update-` / `Uninstall-WinGetPackage` 内部调用 COM，进度以 PowerShell 进度记录给出：`StatusDescription` 为 `Downloading` / `Uninstalling` / `Completed` 等，`PercentComplete` 为 COM 回报的真实百分比。没有字节数。
- 进度记录在控制台宿主里读不到；嵌套 `[powershell]::Create()` 运行会卡住（STA 和 MTA 都试过）；`Start-Job` 在独立进程运行，`$job.ChildJobs[0].Progress` 可读。用 `Export-WinGetPackage` 只下载 Alma（170 MB）实测：0、4、6、9、12 … 57、95、Completed，结果 `Status=Ok`。
- 结果对象：`PSInstallResult` 有 `Status`、`RebootRequired`、`InstallerErrorCode`、`ExtendedErrorCode`（Exception，HResult 与命令行退出码同一套）；`PSUninstallResult` 另有 `UninstallerErrorCode`。

## 方案

1. **winget 来源**（`internal/source/winget`）：`Install`、`Upgrade`、`Uninstall` 改为运行一段 PowerShell，在 `Start-Job` 里调用模块的 cmdlet（`-MatchOption Equals`、指定来源、`-Confirm:$false`；安装和更新 `-Mode Default`，卸载 `Silent` / `Interactive`），外层每 300 毫秒读进度记录，按行写出 `@@progress` 标记，Go 端解析成 `source.Progress`。
   - 安装前在作业里先查是否已装，已装返回“已安装”，保持命令行 `--no-upgrade` 的保护；卸载前查不到返回“已不在”，与命令行的“找不到”处理一致。
   - 结果：`Ok` 成功（带是否需要重启）；`NoApplicableUpgrade` 为“没有适用的更新”；其他按 `ExtendedErrorCode` 的 HResult 复用现有退出码说明，没有时用安装程序退出码或状态名。
   - 模块导入失败时退回现在的命令行方式，并在日志里说明。
   - 日志写明执行的 cmdlet、阶段变化和结果；进度只作为进度报告，不写成日志行。
2. **进度模型**：`source.Progress` 增加 `Percent`（来源回报的百分比，0 表示不知道），经 `tasklog`、`queue`、`api.TaskEntry.percent` 到前端。
3. **前端**：`taskProgress` 有字节总数时照旧；只有百分比时显示确定进度条和“45%”；都没有时仍是不确定动画。
4. **卸载**：winget 管理的软件随第 1 条走 COM，有回报的阶段和百分比就显示；注册表卸载命令的程序没有进度来源，保持阶段 + 已用时间 + 不确定动画。

## 不变量

- 包 ID 精确匹配并指定来源；安装前已装就不装；成败仍由队列执行前后的复查决定。
- 不新增依赖：只用已依赖的模块和 Windows PowerShell 5.1。
- 不自动接受除命令行原本已接受之外的协议。

## 风险

- 结束 AppShelf 时外层 PowerShell 被终止；作业进程随管道断开退出，但 COM 服务端已开始的安装可能继续完成。与命令行方式被终止时安装程序继续运行的情况相同。
- `Start-Job` 多启动一个 PowerShell 进程，开始前多约 1–2 秒。

## 验收

- 单元测试：进度标记解析与阶段映射、结果解释（成功、需要重启、已安装、已不在、没有适用更新、HResult 说明、安装程序退出码、模块不可用时退回命令行）；前端 `taskProgress` 的百分比。
- 本机只读测试：对不存在的包 ID 执行更新，确认走 COM 并返回错误，不改动本机。
- 真实安装 / 更新 / 卸载测试需要用户授权和用户认可的测试软件（`APPSHELF_INSTALL_TEST=1` 等）。
- `gofmt`、`go vet ./...`、`go tool mygo vet`、`go tool mygo generate`、`CGO_ENABLED=0 go test ./...`、前端 `check` 和 `test`、`go tool mygo build`。
- 用户在真实窗口更新一个 winget 软件：下载阶段显示真实百分比，安装阶段显示阶段文字和不确定动画。

## 回滚

还原 `internal/source/winget` 的 `Install` / `Upgrade` / `Uninstall` 到命令行实现即可；`Percent` 字段为空时前端行为与原来一致。
