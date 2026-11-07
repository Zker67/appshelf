# internal

AppShelf 的全部业务代码。分层和依赖方向见 [项目结构总览](../docs/architecture/project-structure.md)，数据格式见 [API 与接口说明](../docs/api.md)。

| 包 | 主要文件 | 职责 | 测试 |
|---|---|---|---|
| `view` | `view.go`（页签和通用对话框）、`shelf.go`（软件页：筛选、表格、安装预览）、`editor.go`（添加和编辑软件）、`tasks.go`（任务页）、`settings.go`（来源、分类、标签、清单、关于） | MyGo 原生 UI。只调用 `core`，打开文件对话框、打开日志、检查自更新等系统能力通过 `Hooks` 由 `main.go` 传入 | `ui.NewTester` 加假来源：筛选取并集、预览跳过已装、编辑校验、选择搜索结果、分类管理 |
| `core` | `core.go` | 打开清单和本机状态（损坏的文件改名留存），并行读取各来源状态、套用检测规则，保存编辑，导入导出，安装预览，以及队列的执行者 | 假来源和临时目录 |
| `queue` | `queue.go` | 安装队列：一个 goroutine 逐项执行；执行前复查（已装跳过、状态不确定不执行），执行后复查；重试、停止、继续、清除已完成 | 假执行者 |
| `source` | 见 [source/README.md](./source/README.md) | 来源接口、状态类型和三个来源 | 见来源说明 |
| `runner` | `runner.go`、`powershell.go`、`shell_windows.go` | 运行外部命令并保留输出末尾；用临时脚本和 UTF-8 JSON 文件运行 PowerShell，避免参数转义和编码问题；用 `ShellExecuteExW` 启动安装程序，让需要管理员权限的安装程序弹出 UAC | 真实的 Windows PowerShell，包括中文和以 `-` 开头的参数 |
| `detect` | `detect.go`、`scan_windows.go` | 扫描注册表 Uninstall 项（64 位、32 位、当前用户），按检测规则认出软件 | 真实扫描本机注册表 |
| `catalog` | `catalog.go`（格式和校验）、`edit.go`（编辑）、`merge.go`（导入合并） | 软件清单 | 单元测试 |
| `state` | `state.go` | 本机状态：GitHub 软件的安装记录 | 单元测试 |
| `version` | `version.go` | 版本号规范化（去掉 `v`、`refs/tags/`、`+build`）和比较 | 单元测试 |
| `jsonfile` | `jsonfile.go` | JSON 读取和原子写入（同目录临时文件、落盘、改名） | 由 `catalog`、`state` 的测试覆盖 |

## 约定

- 外部命令一律按参数数组传递；PowerShell 脚本是常量，用户数据只通过输入 JSON 文件传入。
- 来源的错误信息直接显示给用户，用中文写清原因；不包含 token 等凭据。
- 后台 goroutine 的结果通过 `core.Config.Post` 回到 UI 线程（`Window.Update`），不要在后台直接改界面状态。
