# internal/api

前端调用的 Go 服务。`main.go` 用 `mygo.Bind(shelf.Services()...)` 绑定这里的七个服务，`go tool mygo generate` 按它们生成前端客户端 `frontend/src/mygo.ts`。服务、方法、事件和图标协议的合同见 [API 与接口说明](../../docs/api.md#前端接口)。

## 线程模型

`internal/core` 的清单、状态和检查中标记只能在一个 goroutine 上读写。这里由 `App` 的循环 goroutine 担任：

- `Post(fn)`：core 的后台结果交回循环；队列无上限，不会阻塞后台 goroutine。
- `read(fn)` / `write(kinds, fn)`：绑定方法各自在自己的 goroutine 上运行，经循环读写 core 后返回；`write` 同时标记变化种类。`fn` 里的 panic 会带回调用方。
- 通知器把 80 毫秒内的变化合并成一个 `changed` 事件 `{kinds, revision}`。
- core 刷新结束时顺带刷新本机已装（1 分钟内的结果复用），安装队列跑完时强制刷新。
- 一次“检查更新”：重读清单状态和本机已装，两者都结束才算完成（启动时的读取也算一次）。循环每轮结束时由 `checker.afterLoop` 判断检查是否完成、把结束的更新任务写进历史；完成后在后台给“自动更新”的软件排队，并对新发现的更新发通知。下次检查按设置的间隔排好，每分钟对照一次墙上时间，电脑睡眠醒来后也会补上。

## 文件

| 文件 | 内容 |
|---|---|
| `app.go` | `App`、`Hooks`（文件对话框、打开路径和链接、自更新、外观、系统通知、托盘、开机启动）、`Config`、循环和通知器 |
| `dto.go` | 前端看到的全部类型；改动后要重新生成 `mygo.ts` |
| `shelf.go` | `Shelf`：清单快照、检查更新、添加、批量加入、删除与撤销、分类和标签、备注、导入导出 |
| `tasks.go` | `Tasks`：开始安装和更新（先等正在读取的状态，不预览）、队列操作（含用卸载程序窗口重试 `RetryInteractive`），任务的阶段和进度，应用内日志（`Log` 按游标增量读取）和日志文件 |
| [leftovers.go](./leftovers.go) | `Tasks.FindLeftovers` / `CleanLeftovers` / `OpenBackup`：为卸载成功的任务查找残留，结果和令牌留在服务端，清理只按 ID 选择；注册表备份到 `<数据目录>\backups\leftovers-<时间>-<软件名>` |
| `sources.go` | `Sources`：来源状态与额度、安装 winget 模块、编辑时搜索（等版本读完一起返回）、检查 GitHub 仓库 |
| `discover.go` | `Discover`：统一搜索（`Channel` 流式返回，新的搜索取消旧的；winget 先发结果，读到版本再发一次）、预热（`Warm`）、GitHub 搜索、详情、规则建议；以及 `Changed` 事件 |
| `search.go` | 搜索缓存：按来源和查询（忽略大小写和首尾空格）保留来源的原始结果和 winget 版本 10 分钟，最多 64 次搜索；出错不缓存；“已在清单”“本机已装”在发送时重新标记 |
| [installed.go](./installed.go)、[installed_updates.go](./installed_updates.go) | `Installed`：本机已装列表、与清单对照，以及按已识别 key 直接开始更新；不会自动加入清单。启动时先显示上次运行缓存的列表（`Config.InstalledCache`），同时重新读取；没有缓存时第一次读取先给出只含注册表的列表（`partial`），卸载要等这次读完才用列表，完整列表随后替换；`Config.Watch` 报告 AppShelf 之外的变化时，1.5 秒内的变化合并为一次重读（自己的任务运行时不重读） |
| [uninstall.go](./uninstall.go) | `Installed.PreviewUninstall` / `StartUninstall`：只接受本机已装列表里的 key，由 Go 自己读到的条目决定卸载方式（winget、scoop、应用商店或注册表卸载命令），页面传不了命令和路径；开始前再预览一次，只把能卸载的排进队列，一个都不能时返回原因 |
| [updates.go](./updates.go)、[dto_updates.go](./dto_updates.go) | `Updates`：清单软件和本机 winget / scoop 软件按来源 key 合并的更新列表、AppShelf 自身新版本、更新方式（`SetMode`、`Skip`、`Snooze`、`Restore`）、预览和开始、历史、更新设置；能否自动更新（`autoBlock`）和安装目录（`installDir`）的判断；`navigate` 事件 |
| [autoupdate.go](./autoupdate.go) | `checker`：检查的开始与完成、定时、自动更新排队、通知、更新历史 |
| `about.go` | `About`：版本、自更新、打开目录和链接（只允许 http/https）、外观 |
| `settings.go` | `settings.json` 的读写（外观、更新设置） |

## 约定

- 防误装的判断只在 core 和来源里：这里返回的本机已装、搜索、详情只用于展示。
- 错误直接显示给用户：用中文写清原因，不带 token 等凭据。
- 新增或修改方法后：`go tool mygo generate`，再跑 `CGO_ENABLED=0 go test ./internal/api` 和前端的 `bun run --cwd frontend check`。

## 测试

`api_test.go` 用假来源和临时目录直接调用服务方法，检查返回值和事件：清单快照与变化通知、批量加入与撤销删除、预览与开始、搜索（含取消）、本机已装对照、导入导出、关于、通知合并、循环里的 panic。

`note_test.go`：备注的保存与规范化、变化通知、编辑软件时保留备注、清空备注。

`updates_test.go`：清单与本机合并去重、清单状态优先、跳过 / 暂缓 / 忽略 / 恢复、自动更新的条件（安装范围、GitHub、正在运行）与立即执行、通知只发一次且窗口在前或关闭通知时不发、定时检查与设置、预览开始与历史、安装目录判断、来源 key 校验。

`uninstall_test.go`：假卸载执行者下的预览、排队、执行和任务日志，卸载不会把软件移出清单，不能卸载的程序开始时被拒绝；`Tasks.Start` 不接受卸载；失败后用卸载程序窗口重试，成功的任务不能这样重试。

`leftovers_test.go`（Windows）：只为卸载成功的任务查找残留，在 `%LOCALAPPDATA%` 下找到测试目录并清理到回收站；未选择、未知令牌和未知 ID 的处理；没有注册表备份时不能打开备份。

`api_test.go` 另有：本机已装先给出注册表列表再补全、外部变化后重读、任务日志按游标读取。

`search_test.go`：winget 结果先于版本发出、版本补齐后再发一次、缓存命中与过期、读版本失败不影响结果、编辑时搜索带版本、预热；`TestSearchRealWinget` 经服务真实搜索本机 winget 并计时（只读，没有 winget 时跳过）。
