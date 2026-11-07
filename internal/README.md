# internal

AppShelf 的全部业务代码。分层和依赖方向见 [项目结构总览](../docs/architecture/project-structure.md)，数据格式见 [API 与接口说明](../docs/api.md)。

| 包 | 主要文件 | 职责 | 测试 |
|---|---|---|---|
| [api](./api) | `app.go`（循环 goroutine、通知器）、`dto.go`（前端看到的类型）、`shelf.go`、`tasks.go`、`sources.go`、`discover.go`、`installed.go`、`about.go`、`settings.go` | 前端调用的绑定服务和 `changed` 事件，见 [api/README.md](./api/README.md) | 假来源，直接调用服务方法，检查返回值和事件，不需要窗口 |
| [inventory](./inventory) | `inventory.go` | 本机已装程序：合并 winget、scoop、注册表和应用商店应用，与清单对照，见 [inventory/README.md](./inventory/README.md) | 单元测试，加一次真实的只读读取 |
| [discover](./discover) | `rank.go`、`details.go`、`suggest.go` | 搜索结果排序、软件详情（带缓存）、GitHub 软件的检测规则建议，见 [discover/README.md](./discover/README.md) | 单元测试，加真实读取 VS Code 的详情 |
| [icons](./icons) | `icons.go`、`database.go`、`fetch.go` | 图标协议：图标库、本机图标、GitHub 头像，下载和缓存，见 [icons/README.md](./icons/README.md) | 单元测试，加真实下载图标库 |
| [core](./core) | `core.go` | 打开清单和本机状态（损坏的文件改名留存），并行读取各来源状态、套用检测规则，保存编辑，导入导出，安装预览、清单外软件的批量更新预览（[updates.go](./core/updates.go)），以及队列的执行者 | 假来源和临时目录 |
| [queue](./queue) | `queue.go` | 安装队列：一个 goroutine 逐项执行；执行前复查（已装跳过、状态不确定不执行），执行后复查；重试、停止、继续、清除已完成 | 假执行者 |
| [source](./source) | 见 [source/README.md](./source/README.md) | 来源接口、状态类型和三个来源 | 见来源说明 |
| [runner](./runner) | `runner.go`、`powershell.go`、`worker.go`、`shell_windows.go` | 运行外部命令并保留输出末尾；用临时脚本和 UTF-8 JSON 文件运行 PowerShell，避免参数转义和编码问题；`Worker` 是常驻的 PowerShell，逐个执行脚本，省去每次启动和导入模块（请求以 base64 JSON 按行收发并带编号，调用方放弃时短请求跑完、长请求结束进程，空闲 3 分钟退出）；用 `ShellExecuteExW` 启动安装程序，让需要管理员权限的安装程序弹出 UAC | 真实的 Windows PowerShell，包括中文和以 `-` 开头的参数；`Worker` 的复用、错误、取消、超时、退出、空闲回收和提速 |
| [detect](./detect) | `detect.go`、`scan_windows.go`、`suggest.go` | 扫描注册表 Uninstall 项（64 位、32 位、当前用户），按检测规则认出软件；由程序名或仓库名生成检测规则 | 真实扫描本机注册表 |
| [catalog](./catalog) | `catalog.go`（格式和校验）、`edit.go`（编辑）、`merge.go`（导入合并） | 软件清单 | 单元测试 |
| [state](./state) | `state.go` | 本机状态：GitHub 软件的安装记录 | 单元测试 |
| [version](./version) | `version.go` | 版本号规范化（去掉 `v`、`refs/tags/`、`+build`）和比较 | 单元测试 |
| [jsonfile](./jsonfile) | `jsonfile.go` | JSON 读取和原子写入（同目录临时文件、落盘、改名） | 由 `catalog`、`state` 的测试覆盖 |

## 约定

- 外部命令一律按参数数组传递；PowerShell 脚本是常量，用户数据只通过输入 JSON 文件传入。
- 来源的错误信息直接显示给用户，用中文写清原因；不包含 token 等凭据。
- `core` 只在 `api` 的循环 goroutine 上使用：后台 goroutine 的结果通过 `core.Config.Post` 交回这个 goroutine，不要在别的 goroutine 上直接改 `core`。
- 本机已装、搜索、详情和图标只用于展示；是否已装只由来源查询和检测规则决定。
