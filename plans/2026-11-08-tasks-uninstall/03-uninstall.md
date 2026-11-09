# 阶段 3：按来源卸载

## 目标

在“本机已装”（单项、多选）和“我的清单”（已安装的软件）里卸载软件：先预览，确认后进入队列逐个执行，执行后复查确认。

## 卸载方式

| 本机已装的种类 | 方式 |
|---|---|
| winget 源的包 | `winget uninstall --id <id> --exact --source winget --accept-source-agreements --disable-interactivity`；交互式重试加 `--interactive` |
| msstore 源的非 MSIX 包 | 同上，`--source msstore` |
| 应用商店应用（MSIX） | `Remove-AppxPackage -Package <完整包名>`（当前用户）；框架包、系统应用（`IsFramework`、`NonRemovable`、`SignatureKind = System`）不卸载 |
| scoop | `scoop uninstall <name>`，全局安装加 `--global` |
| 只在注册表里的程序 | 按卸载器种类（规则借鉴 BCU 的 `UninstallerTypeAdder`）：<br>MSI：`WindowsInstaller = 1`、命令含 `msiexec` 或 `Package Cache\{GUID}` → `msiexec /x {产品码} /passive /norestart`<br>Inno Setup：`unins\d{3}.exe` 且同目录有同名 `.dat` → `/VERYSILENT /SUPPRESSMSGBOXES /NORESTART`<br>NSIS：卸载程序文件里有 `Nullsoft` → `/S`<br>有 `QuietUninstallString` → 用它<br>其他：`UninstallString`，打开卸载程序窗口由用户操作 |

- 命令解析：带引号的取引号内为程序；不带引号的取最长一个存在的 `.exe` 前缀；`MsiExec.exe /I{GUID}` 改成 `/X{GUID}`。
- 用 `ShellExecuteExW` 启动，需要管理员的卸载程序弹出 UAC。卸载程序退出后，注册表项还在时继续等待：仍有进程在安装目录下运行，或有常见的卸载程序临时副本（`Au_.exe`、`Un_A.exe`、`_iu*.tmp`），最长 30 分钟，结束后再给 5 秒。
- 退出码：`0` 成功；`3010`、`1641` 需要重启；`1602`、`1223` 已取消；`1618` 另一个安装正在进行；`1605` 产品未知（以复查为准）；其他非零码记录下来，仍以复查为准。

## 不能卸载 / 提醒

- 不能卸载：`NoRemove = 1`；注册表里没有卸载命令；AppShelf 自己；winget（应用安装程序）和 scoop 本体；框架包和系统应用。
- 提醒（仍可执行）：程序正在运行；会打开卸载程序窗口；机器范围安装可能请求管理员权限；运行库等可能被其他软件依赖的组件。

## 设计

| 部分 | 实现 |
|---|---|
| `internal/uninstall` | `Target`（Go 端从本机列表解析出的目标）、`Plan(target)`（纯函数：方式、种类、命令、是否静默、不能卸载的原因、提醒）、`Inspect`（读进程、签名、应用商店包属性）、`Runner.Run` 和 `Runner.Present`（复查） |
| 来源 | winget、scoop 各增加 `Uninstall(ctx, app, interactive, log)` |
| 队列 | 新动作 `Uninstall`；`Item.Target`、`Item.Interactive`；执行前：读不到 → 失败，已不在本机 → 跳过；执行后：仍在本机 → 失败，已不在 → 成功（卸载程序报错但已确认卸载时注明）；`RetryInteractive` |
| core | `PreviewUninstall`、`EnqueueUninstall`；卸载后重读关联的清单软件状态 |
| 接口 | `Installed.PreviewUninstall(keys)`、`Installed.StartUninstall(keys)`、`Tasks.RetryInteractive(id)`；预览项含方式、种类、命令、静默、安装位置、大小、签名、提醒 |
| 前端 | 本机已装行菜单和多选栏“卸载”；清单已安装软件的右键菜单“卸载”；卸载预览弹窗；任务卡片“卸载”动作、失败时“打开卸载程序重试” |

## 验收

- `uninstall`：卸载器识别、命令解析、静默参数、不能卸载规则的单元测试（用真实的注册表字符串样本）；真实只读：对本机注册表所有项生成计划不出错。
- `queue`：卸载的执行前后判断、交互式重试。
- `api`：预览只接受本机列表里的键、开始后进入队列、卸载后清单软件仍在。
- 真实卸载集成测试：`APPSHELF_UNINSTALL_TEST=1` 且 `APPSHELF_UNINSTALL_KEY=<本机已装键>` 时运行；需用户授权和指定软件，实现时不运行。

## 回滚

回退本阶段改动；卸载不写清单和 `state.json`。
