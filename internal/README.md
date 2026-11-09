# internal

AppShelf 的全部业务代码。分层和依赖方向见 [项目结构总览](../docs/architecture/project-structure.md)，数据格式见 [API 与接口说明](../docs/api.md)。

| 包 | 主要文件 | 职责 | 测试 |
|---|---|---|---|
| [api](./api) | `app.go`（循环 goroutine、通知器）、`dto.go`（前端看到的类型）、`shelf.go`、`tasks.go`、`sources.go`、`discover.go`、`installed.go`、`uninstall.go`、`leftovers.go`、`updates.go`、`autoupdate.go`、`about.go`、`settings.go` | 前端调用的绑定服务、`changed` 和 `navigate` 事件，统一更新列表、定期检查和自动更新，卸载和残留清理，见 [api/README.md](./api/README.md) | 假来源，直接调用服务方法，检查返回值和事件，不需要窗口 |
| [inventory](./inventory) | `inventory.go` | 本机已装程序：合并 winget、scoop、注册表和应用商店应用（带大小、安装日期和注册表项），与清单对照，见 [inventory/README.md](./inventory/README.md) | 单元测试，加一次真实的只读读取 |
| [discover](./discover) | `rank.go`、`details.go`、`suggest.go` | 搜索结果排序、软件详情（带缓存）、GitHub 软件的检测规则建议，见 [discover/README.md](./discover/README.md) | 单元测试，加真实读取 VS Code 的详情 |
| [icons](./icons) | `icons.go`、`database.go`、`fetch.go` | 图标协议：图标库、本机图标、GitHub 头像，下载和缓存，见 [icons/README.md](./icons/README.md) | 单元测试，加真实下载图标库 |
| [core](./core) | `core.go`、`uninstall.go` | 打开清单和本机状态（损坏的文件改名留存），并行读取各来源状态、套用检测规则，保存编辑，导入导出，安装预览、清单外软件的批量更新预览（[updates.go](./core/updates.go)）、卸载预览和排队（[uninstall.go](./core/uninstall.go)），以及队列的执行者 | 假来源和临时目录 |
| [queue](./queue) | `queue.go` | 任务队列（安装、更新、卸载）：一个 goroutine 逐项执行；执行前复查（已装跳过、状态不确定不执行；卸载时已不在则跳过），执行后复查（卸载以程序是否还在判断成败）；阶段、进度和日志；重试（卸载可改用卸载程序窗口重试）、停止、继续、清除已完成 | 假执行者 |
| [tasklog](./tasklog) | `tasklog.go` | 一个任务的日志：原样写进文件，同时整理成行（处理 `\r` 重绘、ANSI、进度条字符）留在内存，并记下来源报告的进度，见 [tasklog/README.md](./tasklog/README.md) | 单元测试 |
| [uninstall](./uninstall) | `plan.go`、`run.go`、`inspect.go` | 卸载：winget、scoop、应用商店应用交给各自卸载，其余按注册表的卸载命令；识别 MSI、WiX、Inno Setup、NSIS 并静默卸载，等待卸载程序的副本结束；读取签名，拦下不能卸载的程序，见 [uninstall/README.md](./uninstall/README.md) | 替换的进程和注册表；真实本机只读生成卸载方式和读签名；真实卸载需显式开启 |
| [leftover](./leftover) | `leftover.go`、`scan_windows.go`、`clean_windows.go` | 卸载后的残留：目录、注册表设置、快捷方式、开机启动项，按理由打分；清理时文件进回收站、注册表先备份，见 [leftover/README.md](./leftover/README.md) | 真实临时目录和 `HKCU` 临时键，真实回收站和 `reg export` |
| [watch](./watch) | `watch.go`、`watch_windows.go` | 监听 Uninstall 注册表键、应用包仓库键和 scoop 的 `apps` 目录，发现 AppShelf 之外的安装和卸载，见 [watch/README.md](./watch/README.md) | 真实注册表和目录 |
| [source](./source) | 见 [source/README.md](./source/README.md) | 来源接口、状态类型和三个来源 | 见来源说明 |
| [runner](./runner) | `runner.go`、`powershell.go`、`worker.go`、`shell_windows.go` | 运行外部命令并保留输出末尾；用临时脚本和 UTF-8 JSON 文件运行 PowerShell，避免参数转义和编码问题；`Worker` 是常驻的 PowerShell，逐个执行脚本，省去每次启动和导入模块（请求以 base64 JSON 按行收发并带编号，调用方放弃时短请求跑完、长请求结束进程，空闲 3 分钟退出）；用 `ShellExecuteExW` 启动安装程序，让需要管理员权限的安装程序弹出 UAC | 真实的 Windows PowerShell，包括中文和以 `-` 开头的参数；`Worker` 的复用、错误、取消、超时、退出、空闲回收和提速 |
| [detect](./detect) | `detect.go`、`scan_windows.go`、`location.go`、`suggest.go` | 扫描注册表 Uninstall 项（64 位、32 位、当前用户），读出卸载命令、大小、安装日期等，缺安装位置时从卸载程序和图标推断；按检测规则认出软件；按键查一个程序（`Lookup`）；由程序名或仓库名生成检测规则 | 真实扫描本机注册表 |
| [catalog](./catalog) | `catalog.go`（格式和校验）、`edit.go`（编辑）、`merge.go`（导入合并） | 软件清单 | 单元测试 |
| [state](./state) | `state.go` | 本机状态：GitHub 软件的安装记录、每个软件的更新方式（自动 / 提醒 / 忽略、跳过的版本、暂缓期限）、最近 200 条更新记录 | 真实临时文件，含旧格式文件 |
| [procs](./procs) | `procs.go`、`procs_windows.go` | 某个目录下的程序是否正在运行，见 [procs/README.md](./procs/README.md) | 真实进程（测试程序自己） |
| [version](./version) | `version.go` | 版本号规范化（去掉 `v`、`refs/tags/`、`+build`）、比较和大版本升级判断 | 单元测试 |
| [jsonfile](./jsonfile) | `jsonfile.go` | JSON 读取和原子写入（同目录临时文件、落盘、改名） | 由 `catalog`、`state` 的测试覆盖 |
| [secret](./secret) | `secret.go`、`secret_windows.go` | 用 Windows DPAPI 以当前用户加密保存机密（设置里的 GitHub token），见 [secret/README.md](./secret/README.md) | 真实 DPAPI 加解密、明文不落盘 |

## 约定

- 外部命令一律按参数数组传递；PowerShell 脚本是常量，用户数据只通过输入 JSON 文件传入。
- 来源的错误信息直接显示给用户，用中文写清原因；不包含 token 等凭据。
- `core` 只在 `api` 的循环 goroutine 上使用：后台 goroutine 的结果通过 `core.Config.Post` 交回这个 goroutine，不要在别的 goroutine 上直接改 `core`。
- 本机已装、搜索、详情和图标只用于展示；是否已装只由来源查询和检测规则决定。
