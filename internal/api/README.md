# internal/api

前端调用的 Go 服务。`main.go` 用 `mygo.Bind(shelf.Services()...)` 绑定这里的六个服务，`go tool mygo generate` 按它们生成前端客户端 `frontend/src/mygo.ts`。服务、方法、事件和图标协议的合同见 [API 与接口说明](../../docs/api.md#前端接口)。

## 线程模型

`internal/core` 的清单、状态和检查中标记只能在一个 goroutine 上读写。这里由 `App` 的循环 goroutine 担任：

- `Post(fn)`：core 的后台结果交回循环；队列无上限，不会阻塞后台 goroutine。
- `read(fn)` / `write(kinds, fn)`：绑定方法各自在自己的 goroutine 上运行，经循环读写 core 后返回；`write` 同时标记变化种类。`fn` 里的 panic 会带回调用方。
- 通知器把 80 毫秒内的变化合并成一个 `changed` 事件 `{kinds, revision}`。
- core 刷新结束时顺带刷新本机已装（1 分钟内的结果复用），安装队列跑完时强制刷新。

## 文件

| 文件 | 内容 |
|---|---|
| `app.go` | `App`、`Hooks`（文件对话框、打开路径和链接、自更新、外观）、`Config`、循环和通知器 |
| `dto.go` | 前端看到的全部类型；改动后要重新生成 `mygo.ts` |
| `shelf.go` | `Shelf`：清单快照、检查更新、添加、批量加入、删除与撤销、分类和标签、导入导出 |
| `tasks.go` | `Tasks`：安装预览、开始、队列操作、日志 |
| `sources.go` | `Sources`：来源状态与额度、安装 winget 模块、编辑时搜索（等版本读完一起返回）、检查 GitHub 仓库 |
| `discover.go` | `Discover`：统一搜索（`Channel` 流式返回，新的搜索取消旧的；winget 先发结果，读到版本再发一次）、预热（`Warm`）、GitHub 搜索、详情、规则建议；以及 `Changed` 事件 |
| `search.go` | 搜索缓存：按来源和查询（忽略大小写和首尾空格）保留来源的原始结果和 winget 版本 10 分钟，最多 64 次搜索；出错不缓存；“已在清单”“本机已装”在发送时重新标记 |
| [installed.go](./installed.go)、[installed_updates.go](./installed_updates.go) | `Installed`：本机已装列表、与清单对照，以及按已识别 key 预览和开始直接更新；不会自动加入清单 |
| `about.go` | `About`：版本、自更新、打开目录和链接（只允许 http/https）、外观 |
| `settings.go` | `settings.json` 的读写（外观） |

## 约定

- 防误装的判断只在 core 和来源里：这里返回的本机已装、搜索、详情只用于展示。
- 错误直接显示给用户：用中文写清原因，不带 token 等凭据。
- 新增或修改方法后：`go tool mygo generate`，再跑 `CGO_ENABLED=0 go test ./internal/api` 和前端的 `bun run --cwd frontend check`。

## 测试

`api_test.go` 用假来源和临时目录直接调用服务方法，检查返回值和事件：清单快照与变化通知、批量加入与撤销删除、预览与开始、搜索（含取消）、本机已装对照、导入导出、关于、通知合并、循环里的 panic。

`search_test.go`：winget 结果先于版本发出、版本补齐后再发一次、缓存命中与过期、读版本失败不影响结果、编辑时搜索带版本、预热；`TestSearchRealWinget` 经服务真实搜索本机 winget 并计时（只读，没有 winget 时跳过）。
