# internal/watch

在 AppShelf 之外安装、更新、卸载软件时及时发现：监听记录已装程序的注册表键和 scoop 的 `apps` 目录，有变化就通知调用方。只负责“可能变了”，读什么、什么时候读由调用方决定。

## 接口

| 名称 | 作用 |
|---|---|
| `Watch(ctx, keys, dirs, changed)` | 在自己的 goroutine 上等待，任一键或目录变化时调用 `changed`，直到 `ctx` 结束；打不开的键、不存在的目录跳过；一个都监听不了时返回错误 |
| `InstalledKeys()` | 三个 Uninstall 键（64 位、32 位、当前用户；子键增删和值变化），以及当前用户的应用商店包仓库键 `…\AppModel\Repository\Packages`（只看子键增删） |
| `ScoopDirs()` | scoop 的 `apps` 目录：`%SCOOP%` 或 `~\scoop`，`%SCOOP_GLOBAL%` 或 `%ProgramData%\scoop`，只返回存在的 |

## 实现

- 注册表：`RegNotifyChangeKeyValue`（异步、`REG_NOTIFY_THREAD_AGNOSTIC`），每次通知后重新登记。
- 目录：`FindFirstChangeNotification`（只看子目录增删，不含子树），每次通知后 `FindNextChangeNotification`。
- 所有句柄在一个锁定的系统线程上用 `WaitForMultipleObjects` 等待，另一个事件用于结束。
- AppShelf 启动后才装上的 scoop 不在监听范围内，重启 AppShelf 后生效。

## 谁用

`main.go` 把它交给 `internal/api`（`Config.Watch`）。`api` 把 1.5 秒内的变化合并为一次，再重读本机已装（winget 不复用缓存）；AppShelf 自己的任务运行时不重读，任务结束时本来就会重读。

## 测试

`watch_test.go`（真实注册表和目录）：在 `HKCU\Software\AppShelfTest-<随机>` 下建子键、改值，在临时目录建子目录，都能收到通知；结束后不再通知；不存在的键和目录被跳过；`InstalledKeys` 在本机至少能监听三个。测试结束删除临时键。
