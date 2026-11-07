# 发现页搜索提速

状态：`[in-progress]`：实现和自动检查已完成，等待用户在真实窗口里验收。

## 目标

“发现”页输入宽泛的词（如 `code`、`ob`、`python`）时，winget 结果要 30–55 秒才出现；同一个词第二次搜索又只要 0.4 秒，快慢不定。目标是任何词都在 1 秒内出结果，版本号随后补上。

## 原因（本机实测）

- `Find-WinGetPackage` 本身很快：50 项 0.1–0.2 秒。
- 慢在读取结果对象的 `Version`：模块为每个结果从 CDN 下载 `versionData.mszyml` 和清单，逐个进行，每个 0.3–1.8 秒（winget 诊断日志可见）。下载过的会被 winget 缓存，所以重复的词快。
- `winget search` 命令行显示同样的版本只要约 0.45 秒：它读本地索引里的最新版本，不下载。
- 每次搜索还要新起一个 PowerShell 并导入模块，约 0.35 秒。

## 不变量

- 搜索结果只用于展示，不参与是否已装的判断；加入清单用模块返回的精确 ID。
- 版本只用于显示：读不到时只缺版本，结果照常显示。
- 新的搜索仍取消上一次；不新增依赖；不自己读 winget 的索引数据库。

## 实现

| 改动 | 文件 |
|---|---|
| 常驻 PowerShell `runner.Worker`：一个进程逐个执行脚本，base64 JSON 按行收发并带请求编号；调用方放弃时短请求跑完、超过 2 秒结束进程；超时或脚本退出后下次自动重启；空闲 3 分钟退出 | `internal/runner/worker.go`、`powershell.go`（抽出共用的 `hostSetup`） |
| winget `Search` 在 Worker 里执行，只读 ID 和名称（`-Count 50`）；新增 `SearchVersions`（`winget search` 表格，按精确 ID 取下一列）和 `Warm` | `internal/source/winget/winget.go` |
| 统一搜索并行读结果和版本：结果先发，版本到了同一来源再发一次；编辑时搜索等版本一起返回；按来源和查询缓存 10 分钟、最多 64 次 | `internal/api/discover.go`、`sources.go`、`search.go`、`app.go` |
| 发现页显示时调用 `Discover.warm()`；输入停顿从 400 毫秒改为 250 毫秒 | `frontend/src/pages/discover/index.tsx`、生成的 `src/mygo.ts` |

scoop 搜索只受益于缓存：本机没有 scoop，无法实测常驻进程对 scoop 脚本的影响，没有改动它的执行方式。

## 验证结果

| 项目 | 改前 | 改后 |
|---|---|---|
| 新词 `music`（旧脚本） | 55.0 秒 | — |
| 宽泛词 `player` / `manager` / `tool`（winget 层） | 30–55 秒 | 结果 0.14–0.19 秒，50 项版本 0.33–0.40 秒 |
| 经服务端到端 `browser` / `editor` | — | 第一批 0.19–0.20 秒，带版本的第二批 0.36–0.37 秒，再搜 0.5 毫秒（缓存） |
| PowerShell 单次请求 | 0.35 秒（新进程） | 约 0.6 毫秒（常驻进程） |

自动检查：`gofmt -l .`、`go vet ./...`、`go tool mygo vet`、`CGO_ENABLED=0 go test ./...` 通过；前端改动的文件通过 `vp check`，`bun run --cwd frontend test` 通过。

## 待用户验收

在真实窗口里打开“发现”页，输入 `code`、`ob`、`python` 这类宽泛的词：结果应在 1 秒内出现，版本号随后补上；删字回到搜过的词时结果立即出现。

## 回滚

还原上表的文件即可；没有数据格式或配置变化。
