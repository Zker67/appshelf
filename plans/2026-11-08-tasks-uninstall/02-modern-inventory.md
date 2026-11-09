# 阶段 2：本机已装读取现代化

## 目标

1. 注册表一次读全卸载、大小、日期相关字段，供显示和阶段 3 卸载使用。
2. 第一次打开时先显示注册表结果（毫秒级），winget 读完后再补上来源、更新和应用商店应用。
3. 在 AppShelf 外安装、卸载软件后，本机已装自动刷新。

## 设计

| 部分 | 实现 |
|---|---|
| 注册表字段 | `detect.Entry` 增加 `UninstallString`、`QuietUninstallString`、`WindowsInstaller`、`EstimatedSize`（KB）、`InstallDate`、`NoRemove`、`InstallSource`；`detect.Lookup(scope, arch, name)` 重读单个键 |
| 补全安装位置 | `InstallLocation` 为空时，从 `DisplayIcon` 或卸载程序所在目录推断（跳过 msiexec、`Package Cache`、`Windows\Installer`、临时目录和盘符、Program Files 等根目录），标记为推断 |
| 键名稳定 | 只在注册表里的程序，键名改为与 winget 一致的 `arp:<范围>\<架构>\<键名>`，winget 读完后不变，界面行不跳 |
| 分步显示 | 还没有完整列表时，先发出注册表结果（`InstalledList.partial = true`），winget、scoop 读完后替换；以后的刷新保留旧列表直到新的读完 |
| 监听 | 新包 `internal/watch`：`RegNotifyChangeKeyValue` 监听三个 Uninstall 键（64 位、32 位、当前用户）和当前用户的应用商店包仓库键；`FindFirstChangeNotification` 监听 scoop 的 `apps` 目录（存在时）。变化合并 1.5 秒后重读本机已装（winget 不复用缓存）；安装队列运行时不触发，队列结束时本来就会重读 |
| 接口 | `InstalledEntry` 增加 `size`（字节）、`installed`（安装日期）、`location`；`InstalledList.partial` |
| 前端 | 行右侧显示大小和安装日期；分步显示时顶部显示“正在补充来源和更新”；列表可按名称、大小、安装日期排序 |

## 不变量

- 本机已装仍只用于展示；检测规则用的 `detect.Scan` 结果不变（只是多了字段）。
- 监听失败不影响其他功能，只是不自动刷新。

## 验收

- `detect`：真实扫描本机注册表，至少有一项带 `UninstallString`；`Lookup` 能读回同一项；位置推断的单元测试。
- `inventory`：注册表项键名与 winget ARP 键名一致的单元测试。
- `watch`：在 `HKCU\Software\AppShelfTest-<随机>` 下建临时键并改值，确认收到通知后删除；临时目录建子目录确认收到通知。
- `api`：分步显示（先 partial 再完整）的测试。
- 用户验收：第一次打开本机已装立即出现列表；在外部安装或卸载一个软件后列表自动更新。

## 回滚

回退本阶段改动；`arp:` 键名替换 `reg:` 只影响显示用的键，不进入清单或本机状态。
