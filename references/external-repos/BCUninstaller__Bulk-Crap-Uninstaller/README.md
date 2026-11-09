# BCUninstaller/Bulk-Crap-Uninstaller

| 项 | 内容 |
|---|---|
| 地址 | https://github.com/BCUninstaller/Bulk-Crap-Uninstaller |
| 版本 | commit `30da609384c98ba6e35c6530129541ea4ff3970a`（2026-09-08），2026-11-08 访问 |
| 许可证 | Apache-2.0（`Licence.txt`、`NOTICE`） |
| 语言 | C# / .NET，Windows Forms |
| 参考范围 | 卸载器识别、静默卸载参数、批量卸载的执行与完成判断、残留扫描与打分 |

## 关键模块（`source/UninstallTools/`）

| 文件 | 作用 |
|---|---|
| `Factory/ApplicationUninstallerFactory.cs` | 汇总注册表、MSI、目录、Steam、Store、scoop 等来源，去重并补全信息 |
| `Factory/RegistryFactory.cs` | 读取 Uninstall 键：`UninstallString`、`QuietUninstallString`、`WindowsInstaller`、`EstimatedSize`、`InstallDate`、`NoRemove`、`ReleaseType` 等 |
| `Factory/InfoAdders/UninstallerTypeAdder.cs` | 卸载器种类：msiexec 或 `Package Cache\{GUID}` 为 MSI；`unins\d\d\d` 且有同名 `.dat` 为 Inno Setup；文件含 `Nullsoft` 为 NSIS；`InstallShield Installation Information\{` 为 InstallShield |
| `Factory/InfoAdders/InnoSetupQuietUninstallStringGenerator.cs` | Inno 静默参数 `/VERYSILENT /SUPPRESSMSGBOXES /NORESTART` |
| `Factory/InfoAdders/NsisQuietUninstallStringGenerator.cs` | NSIS 不直接加 `/S`，而是交给 `UninstallerAutomatizer` 自动点按钮 |
| `Uninstaller/BulkUninstallEntry.cs` | 执行后跟踪子进程和 msiexec，所有相关进程结束才算完成；静默失败可退回交互式 |
| `Uninstaller/BulkUninstallTask.cs` | 并发冲突：MSI 类不并发、同发布商不并发、安装目录互相包含的不并发 |
| `Junk/JunkManager.cs`、`Junk/Finders/*` | 残留扫描器：安装目录、AppData、注册表、COM、防火墙、快捷方式、启动项等 |
| `Junk/Confidence/ConfidenceRecords.cs` | 残留打分：明确关联 +4、目录为空 +4、目录仍被使用 −7、仍有可执行文件 −4 等 |

## 与 AppShelf 的关系

| BCU 的做法 | AppShelf 的取舍 |
|---|---|
| 自己生成并执行卸载命令 | 有来源的交给 winget / scoop / `Remove-AppxPackage`；只在注册表里的程序才按 BCU 的识别规则执行 |
| NSIS 靠自动点按钮 | 不做自动点按钮；NSIS 用标准的 `/S`，没有静默方式的打开卸载程序窗口由用户操作 |
| 跟踪子进程判断完成 | 采纳思路：以复查（注册表项消失、来源查不到）为准，复查前等待安装目录下的进程和常见卸载程序临时副本结束 |
| 并发冲突规则 | 不需要：AppShelf 的队列一次只执行一项 |
| 静默失败自动退回交互式 | 改成失败项由用户点击“打开卸载程序重试” |
| 残留扫描器与打分 | 采纳打分和理由的模型；首期只扫安装目录、常见数据目录、`HKCU\Software`、快捷方式、启动项和 scoop 的 persist；不扫 COM、防火墙、HKLM |
| `DirectoryFactory` 扫描未注册的便携软件 | 不采纳：误报多 |

未复制 BCU 的代码，只按上述规则在 Go 中重新实现。实现见 [plans/2026-11-08-tasks-uninstall](../../../plans/2026-11-08-tasks-uninstall/00-overview.md)。
