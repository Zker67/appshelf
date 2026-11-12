# Devolutions/UniGetUI

- 仓库：https://github.com/Devolutions/UniGetUI
- 访问日期：2026-11-07；commit 未锁定（读的是 main 分支）
- 许可证：MIT
- 技术栈：C# / .NET，界面为 Avalonia（`src/UniGetUI.Avalonia`，跨 Windows / macOS / Linux），另有终端界面 `src/UniGetUI.Tui`；早期版本为 WinUI 3
- 参考范围：已装检测、检查更新、忽略更新、操作队列、自更新。只借鉴思路，不复制代码。

## 借鉴点

| 主题 | UniGetUI 的做法 | 关键源码 | AppShelf 结论 |
|---|---|---|---|
| winget 集成 | 默认走 COM API（`WindowsPackageManager.Interop`），失败时回退到 CLI 文本解析 | `src/UniGetUI.PackageEngine.Managers.WinGet/ClientHelpers/NativeWinGetHelper.cs`、`WinGetCliHelper.cs` | 待评估：Go 调 COM 成本高，优先评估 `Microsoft.WinGet.Client` PowerShell 模块输出 JSON |
| 列已装 | COM 建 `LocalCatalogs` 组合目录全量查询，直接读 `IsUpdateAvailable` | 同上 | 参考 |
| CLI 回退 | `winget update --include-unknown`、`winget list` 按列解析，处理 `…` 截断和多语言表头 | `WinGetTableLayout.cs`、`WinGetLocalizedTableMatrixTests.cs` | 只作兜底 |
| 并发 | winget 本地索引不支持并发，CLI 调用加全局锁，超时 120 秒 | `WinGet.cs` | 采纳：所有 winget 调用串行 |
| 来源归类 | `MSIX\` 归 Store，`{GUID}` 归本机 ARP，`_is1` 等有特殊规则 | `WinGet.GetLocalSource` | 参考 |
| scoop | `scoop list` / `scoop status -l` 加 `Format-Table -Width 4096` 后解析表格 | `Managers.Scoop/Scoop.cs`、`Helpers/ScoopTable.cs` | 不采纳表格解析，改为 `ConvertTo-Json` 拿对象（待验证） |
| 安装参数 | `--id X --exact --source`、`--accept-source-agreements`、`--disable-interactivity` | `Helpers/WinGetPkgOperationHelper.cs` | 采纳；不采纳“Id 截断时退回按名称安装” |
| 忽略更新 | key 为 `{manager}\{Id}`，版本可为具体值或 `*`，支持时限 | `IgnoredUpdatesDatabase.cs` | 采纳 |
| 操作队列 | 并发数默认 1；按 winget 退出码自动重试，另可手动以管理员 / 交互式重试 | `AbstractOperation.cs` | 采纳队列和手动重试；自动重试规则逐条评估 |
| 后台检查 | 检查间隔、关闭自动检查、自动更新三个设置，Toast 通知 | `Win32ToastNotifier.cs` | 参考 |
| 自更新 | 拉产品信息 JSON，校验 SHA256 和签名者指纹，空闲时静默安装 | `Avalonia/Infrastructure/AvaloniaAutoUpdater.cs` | 不采纳，改用 MyGo 自带更新机制 |
| 软件详情（2026-11-07 补充） | winget 走 COM 的 `CatalogPackageMetadata`，另用自家的 Pinget（[Devolutions/pinget](https://github.com/Devolutions/pinget)，MIT，0.x）`show --output json` 补全安装程序信息 | `Managers.WinGet/Helpers/WinGetPkgDetailsHelper.cs`、`ClientHelpers/PingetPackageDetailsProvider.cs` | 不采纳：`Microsoft.WinGet.Client` 的对象没有简介和主页；改读 winget-pkgs 仓库的 YAML 清单 |
| 图标 | 自建图标库 `WebBasedData/screenshot-database-v2.json`（约 1.5 MB、1.2 万条，图标托管在第三方网站）；winget 清单的 Icons；注册表 DisplayIcon（不是 `.exe` 时） | `Core.IconStore/IconDatabase.cs`、`PackageManagerClasses/Packages/Package.cs` 的 `GenerateIconId`、`ClientHelpers/WinGetIconsHelper.cs` | 采纳图标库（运行时下载缓存，不分发）和键名规则；注册表图标另外支持从 `.exe` 提取 |
| 已装匹配 | 按卸载键、UpgradeCode、ProductCode、名称加发布者依次匹配注册表项 | `Managers.WinInstaller/Internal/InstalledMatcher.cs` | 参考：用 ProductCodes 把 winget 项对上注册表项 |
| 已装、发现页 | 已装页汇总所有管理器的已装软件（含“本机”和应用商店项）；发现页并行搜索各管理器后合并 | `PackageLoader/InstalledPackagesLoader.cs`、`DiscoverablePackagesLoader.cs` | 采纳，见 [Web 前端计划](../../../plans/2026-11-07-web-ui-and-discovery/01-unigetui-comparison.md) |

## 图标来源复核（2026-11-07）

本次锁定 commit `e857b3f5d61e377b6137f91c0fa3c25b4bfcd4d0`；保留前文早期调研记录，只参考思路，AppShelf 实现自行编写。

| 源码 | 做法 | AppShelf 处理 |
|---|---|---|
| [WinGetIconsHelper.cs](https://github.com/Devolutions/UniGetUI/blob/e857b3f5d61e377b6137f91c0fa3c25b4bfcd4d0/src/UniGetUI.PackageEngine.Managers.WinGet/ClientHelpers/WinGetIconsHelper.cs) | 读取 Appx 的 Square44x44Logo，尝试多个 scale 文件；用 AppModel Repository 注册表取得 MSIX 路径；winget 原生包元数据支持 Icons URL 与 SHA-256 | 采纳 XML 应用 Logo、多缩放资源和注册表路径；补稀疏包的 WindowsApps 回退。暂未接入 WinGet 原生图标元数据，仍保留现有公共图标库回退 |
| [WinGetPkgDetailsHelper.cs](https://github.com/Devolutions/UniGetUI/blob/e857b3f5d61e377b6137f91c0fa3c25b4bfcd4d0/src/UniGetUI.PackageEngine.Managers.WinGet/Helpers/WinGetPkgDetailsHelper.cs) | 区分 Local PC、MSIX、msstore 与 winget 的图标来源；安装目录按明确标识查找 | 保留来源分流；本地额外补充 MSI ProductIcon、软件同名主 EXE 和无歧义的开始菜单名称回退 |
| [IconCacheEngine.cs](https://github.com/Devolutions/UniGetUI/blob/e857b3f5d61e377b6137f91c0fa3c25b4bfcd4d0/src/UniGetUI.Core.IconStore/IconCacheEngine.cs) | 同一任务共享、磁盘缓存，按 SHA-256、大小、版本或 URL 校验；部分大图缩小至最多 192 像素 | 已有合并任务、文件变化失效和持久缓存；本轮不引入图片缩放依赖 |

当前项目的完整策略见 [图标服务](../../../internal/icons/README.md)，执行与验收见 [图标优化计划](../../../plans/2026-11-07-installed-icons/00-overview.md)。

## 升级流程对照（2026-11-12）

本次依据用户整理的 UniGetUI 升级流程说明（`AbstractOperation.cs`、`AbstractProcessOperation.cs`、`WinGetPkgOperationHelper.cs`、`PackageOperations.cs`、CLI / IPC 文档）对照 AppShelf 源码，未重新锁定 commit。

| UniGetUI 机制 | AppShelf 结论 |
|---|---|
| 每个软件独立任务、防重复、默认串行 | 已有 |
| 升级后重新查询失败时按预期版本构造记录 | 不采纳：AppShelf 以执行后的真实复查为准 |
| 虚假更新的记录与抑制 | 采纳为“更新后来源仍提示同一版本就停止自动更新这个版本” |
| 升级前关闭进程 | 采纳为用户发起的更新前自动关闭；有窗口不关的不强行结束（已实施） |
| 手动任务优先于自动更新、取消、管理员 / 交互式重试 | 交互式重试、手动优先、单项取消（等待项与可安全停下的运行项）已实施；自动提权不采纳 |
| 按错误改 scope / 架构 / ID 参数重试 | 不采纳：与“精确 ID、不模糊”冲突 |
| 更新年龄（新版本满 N 天才自动更新） | 采纳，但按 AppShelf 第一次发现的时间算：winget、scoop 拿不到可靠的发布时间（已实施） |
| 需要重启的结果单独记录 | 采纳：持久记录到这次开机之后，任务面板和托盘提示（已实施） |

取舍和实施状态见 [升级流程优化计划](../../../plans/2026-11-12-upgrade-flow/00-overview.md)。
