# 本机已装、更新、卸载的详情面板

状态：`[in-progress]`

## 目标

“本机已装”“更新”“卸载”三页点开一行，在右侧显示详情面板，和“我的清单”“发现”一样用 `DetailPanel`：

| 页面 | 详情的重点 |
|---|---|
| 更新 | 当前版本 → 新版本、更新方式与暂缓状态、**更新日志**（逐版列出）、来源介绍 |
| 卸载 | **实测空间占用**（程序目录 + 可能相关的数据目录）、**打开位置**、卸载方式与命令、签名、上次打开 |
| 本机已装 | 上面两者的合集：本机信息、来源介绍、有更新时的更新日志、空间、卸载方式 |

## 决定

- 详情形式：右侧面板，三页统一，复用 `DetailPanel` / `DetailTabs` / `PackageInfo`。
- 更新日志的来源，按顺序取：
  1. GitHub 来源：读仓库的 releases，列出已装版本之后、目标版本及以前的每一版，最多 10 版，超出时注明还有更早的。
  2. winget：清单的主页、更新说明链接或发布者链接指向 GitHub 仓库时，走第 1 条；否则用清单的 `ReleaseNotes` 正文；都没有时只给 `ReleaseNotesUrl` 链接。
  3. scoop：主页是 GitHub 仓库时走第 1 条，否则说明来源没有提供更新日志。
  4. 版本号和 tag 对不上时不猜，不显示别的版本的日志，只给发行页链接。
- 空间占用分两项：
  - 程序目录：实测目录大小（注册表登记的大小只是估计，一并显示）。目录取注册表登记的安装目录（或由图标、卸载程序推断的，标明“推断”）、scoop 的应用目录、应用商店的包目录；共享目录（`detect.OwnDirectory` 为假）不统计。
  - 可能相关的数据：用 `leftover.Find` 只读查找到的数据目录（不含安装目录、开始菜单目录、低分项），标明“推测”，不参与任何删除。
- 打开位置：前端只传 key（和数据目录的查找 ID），由 Go 解析出路径再打开，与卸载“前端只传 key”的不变量一致。

## 不变量

- 详情只用于展示：读不到日志、空间统计失败都不影响安装、更新、卸载的判断。
- 卸载仍走原来的预览确认；详情里的“卸载”按钮只是打开同一个预览。
- 不新增依赖。GitHub 请求只在打开更新详情时发起，结果在内存里缓存 1 小时，避免耗尽未登录时每小时 60 次的额度。

## 实现清单

### Go

- `internal/source/github`：`Release` 加 `Body`；新增 `Releases(ctx, repo)` 读最近 30 个 release。
- `internal/discover/notes.go`：`ReleaseNotes` / `ReleaseNote` 类型、`Details.ReleaseNotes(ctx, ref, from)`、从链接认出 GitHub 仓库；winget 清单补读 `ReleaseNotes` 正文。`Fetchers` 加 `GitHubReleases`。
- `internal/api`：
  - `Discover.ReleaseNotes(ctx, ref, from)`。
  - `Installed.Space(ctx, key)`：实测程序目录、查找并实测数据目录，记下这次的数据目录供打开。
  - `Installed.OpenLocation(key, id)`：`id` 为空打开程序目录，否则打开 `Space` 找到的那个数据目录。
- `main.go` 接上 `GitHubReleases`；`go tool mygo generate` 更新 `frontend/src/mygo.ts`。

### 前端

- `components/package-info.tsx`：导出 `FactGrid`；`PackageInfo` 可在自己的页签前放调用方的页签。
- `components/program-info.tsx`：`ReleaseNotesView`、`SpaceView`、`UninstallView`、本机信息的事实格。
- `pages/installed/detail.tsx`、`pages/updates/detail.tsx`、`pages/uninstall/detail.tsx`：三个面板；三页接入右侧可调宽度的详情栏。

## 验收标准

- Go：`gofmt -l .`、`go vet ./...`、`go tool mygo vet`、`CGO_ENABLED=0 go test ./...` 通过；新增测试覆盖 GitHub 版本区间筛选、winget 正文与 GitHub 回退、tag 对不上时不显示、空间统计与数据目录过滤、打开位置只接受 key 与已知 ID。
- 前端：`bun run --cwd frontend check`、`bun run --cwd frontend test` 通过；`go tool mygo build` 能打包。
- 用户在真实窗口检查：三页点一行出现详情，更新详情有日志，卸载详情有空间和“打开位置”。

## 回滚

改动集中在新增文件和三页的详情栏接入，回退对应提交即可；Go 新增接口不改变已有接口的行为。

## 实施记录（2026-11-09）

- Go 与前端按上面的清单实现；`go vet`、`go tool mygo vet`、`CGO_ENABLED=0 go test ./...`、前端 `check` 与 `test`、`go tool mygo build` 均通过。
- 真实读取（只读）：ripgrep 的日志来自 GitHub release；Git 的 tag 是 `v2.56.0.windows.4`、winget 版本是 `2.56.0.4`，按“忽略数字之间的单词”匹配后同样来自 GitHub；VS Code 的 winget 清单没有正文，只给更新说明链接。
- 详情里的更新日志、空间统计、卸载方式在选中行停留约 0.35 秒后才读取，按住方向键快速经过的行不发请求。
- 待用户在真实窗口验收界面。
