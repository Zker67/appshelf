# AppShelf 基本实现：总览

状态：`[in-progress]`（7 个阶段已实现，等待用户测试）

## 背景与目标

做一个 Windows 个人软件商城：用户在清单里维护软件、分类和标签，按分类或标签多选软件，逐个批量安装；AppShelf 检测本机已装软件，绝不误装，并检查更新。

本计划的交付物是**一个能日常使用的基本版本**，实现后由用户亲自测试。不做单独的原型阶段；技术核验放在对应阶段的开头，作为内部步骤。

## 范围

| 功能 | 说明 |
|---|---|
| 清单管理 | 添加、编辑、删除软件；管理分类和标签 |
| 导入导出 | 清单就是一份 JSON 文件；导入时合并 |
| 本机检测 | 识别本机已装的软件，包括不是经 AppShelf 安装的 |
| 防误装 | 已装的跳过；包 ID 精确匹配并指定来源；检测失败就不装 |
| 批量安装 | 按分类、标签多选 → 勾选软件 → 预览 → 逐个安装；失败的可单独重试 |
| 检查更新 | 启动时和手动触发时检查，参考 UniGetUI 的方式；选中软件后执行更新 |
| 自更新 | AppShelf 自身检查并安装新版本，使用 MyGo 的更新机制 |
| 安装来源 | winget、scoop、GitHub Releases |

## 非目标

后续 AI 不要自行加入以下功能：离线安装包、卸载、忽略或跳过某个更新、后台定时检查和托盘通知、自动安装更新、多设备同步、预发布版本、zip 便携包、winget 和 scoop 以外的包管理器、把本机已装软件反向导入清单。

## 当前事实（规划时）

实施后的实际情况见 [09-implementation-report.md](./09-implementation-report.md)。

- 项目骨架已有：MyGo `v0.8.1` 原生 UI 示例（`main.go`），`go vet`、`go test`、`go build` 均已通过。
- MyGo 原生 UI 提供 `Table`（多选 `Selection`、按列排序、只绘制可见行）、`Sidebar`、`Checkbox`、`Modal`、`Select`、`Tabs`、`Progress`、`Spinner`、`Badge`，以及页面路由；`App.Path(PathUserData)` 提供数据目录，`RequestSingleInstanceLock` 提供单实例锁，`plugins/updater/native` 提供原生 UI 的更新窗口。
- 外部调研结论见 [UniGetUI](../../references/external-repos/Devolutions__UniGetUI/README.md)、[GitHub Store](../../references/external-repos/komi-store__komi-store/README.md)、[winget / scoop 集成要点](../../references/external-docs/README.md)。

## 不变量

见 [AGENTS.md](../../AGENTS.md) 的“项目不变量”，其中最重要的是：

1. **检测不确定就不装（fail-closed）。** 来源不可用、检测命令出错、结果无法解析时，该软件的状态记为“未知”，不允许安装。
2. **每项安装前都要重新检测。** 预览时的检测结果只用于展示；队列真正执行到某一项时，再查一次。
3. **装完之后再检测确认。** 安装程序报告成功、但重新检测仍找不到软件时，这一项记为失败。
4. 所有外部命令按参数数组传递，不拼接 shell 字符串；ID 先按严格格式校验。

## 假设（2026-11-07 用户已确认）

- 一个软件只属于**一个分类**，可以有**多个标签**。
- 多选分类、标签时取**并集**：列表里显示属于任一所选分类或带任一所选标签的软件；不选时显示全部。
- winget 只使用 `winget` 源，不使用 `msstore`。
- GitHub 来源只处理 `.msi` / `.exe` 安装包，只取正式版（`releases/latest`）。
- GitHub token 是可选的，从环境变量 `GITHUB_TOKEN` 或 `GH_TOKEN` 读取，不存盘、不写日志。

## 用户决定（2026-11-07）

1. **自更新的发布位置：公开仓库** `Zker67/appshelf`（`mygo.json` 的 `updates.github`）。创建远端仓库、上传和发布 Release 仍需用户另外授权；在那之前阶段 05 标为 `[partial]`。
2. **可以依赖 `Microsoft.WinGet.Client` PowerShell 模块**：本机没有时，AppShelf 提示并在用户确认后执行 `Install-Module -Scope CurrentUser`。

## 风险

| 风险 | 应对 |
|---|---|
| `Microsoft.WinGet.Client` 的实际输出或兼容性和调研不符 | 阶段 02 先核验；不符时回退到“单项 `winget list --id X --exact` 看退出码”的方式检测，检查更新再另行评估，并回写计划 |
| scoop 命令转 JSON 的方式和预期不同 | 阶段 04 先核验；`scoop export` 是查证过的 JSON，至少能保证已装检测 |
| MyGo 0.x 版本的 API 变化 | 锁定 `go.mod` 中的版本，实施期间不升级 |
| GitHub 安装包的静默参数因软件而异 | 默认交互式安装，用户在清单里填写参数后才静默安装 |
| 安装时弹出 UAC 或安装程序窗口 | AppShelf 本身不以管理员身份运行，由安装程序自己申请提权；队列等待进程退出 |

## 阶段索引

| 阶段 | 文件 | 内容 | 状态 |
|---|---|---|---|
| 01 | [01-foundation.md](./01-foundation.md) | 代码结构、清单与本机状态、命令执行、来源接口 | `[done]` |
| 02 | [02-winget-and-queue.md](./02-winget-and-queue.md) | winget 来源、防误装检测、安装队列 | `[done]` |
| 03 | [03-ui.md](./03-ui.md) | 主界面、清单编辑、设置页 | `[done]` |
| 04 | [04-scoop.md](./04-scoop.md) | scoop 来源 | `[partial]`：本机没有 scoop，未实测 |
| 05 | [05-import-export-and-self-update.md](./05-import-export-and-self-update.md) | 导入导出、AppShelf 自更新 | `[partial]`：自更新等待发布 |
| 06 | [06-github.md](./06-github.md) | GitHub Releases 来源 | `[done]` |
| 07 | [07-acceptance.md](./07-acceptance.md) | 交付自测与用户测试清单 | `[in-progress]`：等待用户测试 |
| 09 | [09-implementation-report.md](./09-implementation-report.md) | 实施报告 | - |

阶段 01 到 03 完成后，winget 的整条链路已经可用；之后的 scoop 和 GitHub 只是新增来源适配器，不改界面和队列。

## 验证标准

- 每个阶段结束：`go vet ./...`、`go test ./...` 通过。
- 测试分三类：纯逻辑用单元测试（版本比较、选包、合并、校验）；只读的来源查询用集成测试，直接调用本机真实的 winget、scoop 和 GitHub API，来源不可用时跳过；会真实安装软件的测试默认不运行，只有设置 `APPSHELF_INSTALL_TEST=1`、并且用户授权和指定了测试软件后才运行。
- 视图用 `ui.NewTester` 测试关键交互。
- 最终由用户按 [07-acceptance.md](./07-acceptance.md) 实际测试。

## 回滚

项目是新建的，每个阶段只新增包和文件。阶段失败时，删除或还原该阶段新增的文件即可。建议每个阶段结束时做一次检查点提交，但 commit 需要用户对 appshelf 仓库单独授权。

## 完成后

稳定事实（数据格式、目录结构、运行方式）同步到 `docs/api.md`、`docs/architecture/project-structure.md`、`docs/operations.md`，并更新 README 的功能特性；实施报告写到 `09-implementation-report.md`。
