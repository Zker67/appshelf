# 阶段 07：交付与用户测试

状态：`[in-progress]`（AI 自测完成，等待用户测试）

## 交付前 AI 自测

1. `go vet ./...` 和 `go test ./...` 全部通过，其中包括 winget、scoop、GitHub 的只读集成测试。
2. `go tool mygo build` 生成 exe 和安装包。
3. 同步文档：`docs/api.md`（清单和本机状态的格式）、`docs/architecture/project-structure.md`（实际目录）、`docs/operations.md`（运行、构建、发布）、`README.md`（功能特性、勾选已完成的需求）。
4. 写 `09-implementation-report.md`：实际改动、验证结果、与计划不同的地方、遗留问题。

界面效果不由 AI 截图自验，由用户在真实窗口里检查。

### 自测结果（2026-11-07）

| 项 | 结果 |
|---|---|
| `gofmt -l .`、`go vet ./...`、`go tool mygo vet .`、`go tool mygo vet ./internal/view` | 通过，无输出 |
| `CGO_ENABLED=0 go test ./...` | 全部通过。winget 只读集成测试调用真实 winget；GitHub 真实 API 测试在本机额度用完时跳过，临时用 `gh auth token` 设置 token 后通过；scoop 真实测试因本机没有 scoop 而跳过；三个来源的真实安装测试默认不运行 |
| `go tool mygo build`（带私钥） | 生成 `AppShelf.exe`、`appshelf-setup-0.1.0-amd64.exe`、签名的 `update-windows-amd64.json`、更新包、`install.ps1`、`SHA256SUMS`；没有 `-upload`，没有上传 |
| 启动 `build/windows-amd64/AppShelf.exe` | 正常启动并保持运行，窗口关闭后以 0 退出；没有截图 |
| 文档同步 | 已完成，见 [09-implementation-report.md](./09-implementation-report.md) |

## 用户测试清单

用 `go tool mygo dev`（开发版，不能自更新），或运行 `build/windows-amd64/appshelf-setup-0.1.0-amd64.exe` 安装后测试（安装到 `%LOCALAPPDATA%\Programs\AppShelf`，不需要管理员权限）。两者共用 `%AppData%\AppShelf\` 里的清单。GitHub 软件较多时，先设置环境变量 `GH_TOKEN`。

| # | 场景 | 预期 |
|---|---|---|
| 1 | 首次启动 | 清单为空；设置页正确显示 winget、模块、scoop、GitHub 的状态 |
| 2 | 管理分类和标签 | 可以添加、重命名、删除；删除时提示影响几个软件 |
| 3 | 添加 winget 软件 | 搜索后选择，填入精确 ID；保存后出现在列表中 |
| 4 | 添加 scoop 软件 | 同上，显示 `bucket/name` |
| 5 | 添加 GitHub 软件 | 填写仓库后点“检查”，能看到将选中的安装包；不填检测规则无法保存 |
| 6 | 本机已有的软件 | 不是用 AppShelf 装的软件，也显示为“已安装” |
| 7 | 筛选 | 多选分类和标签，列表显示并集；“全选”只选当前筛选结果 |
| 8 | 批量安装 | 选中几个软件，其中包括已装的 → 预览里已装的显示跳过 → 确认后逐个安装 → 每项都有结果 |
| 9 | 失败与重试 | 失败项显示原因，可以打开日志；“重试”可以重新执行 |
| 10 | 停止 | 当前项执行完后不再继续，剩下的项保持等待 |
| 11 | 检查更新 | 可更新的软件显示当前版本和新版本；选中后点“更新”能升级 |
| 12 | 导出与导入 | 导出 → 删除几个软件 → 导入，软件恢复，原有标签不丢失 |
| 13 | 防误装：断网 | 本机检测不依赖网络，读得到的照常显示“已安装 / 未安装”；读不到的显示“未知”，不能安装；对未装的软件执行安装会失败并说明原因。不会装错，也不会重复装 |
| 14 | 防误装：来源外安装 | 清单里的软件来源是 scoop 或 winget，本机已经用别的方式装了同一软件，并且填写了检测规则 → 显示“已安装（来源外）”，不安装，也不从该来源更新。本机没有 scoop 时同样有效 |
| 15 | 自更新 | 需要先授权创建公开仓库 `Zker67/appshelf` 并发布：安装 0.1.0 → 发布 0.1.1 → 0.1.0 提示更新并成功升级 |

测试中发现的问题，记录到 `09-implementation-report.md` 的遗留项，或者开一份新的计划。
