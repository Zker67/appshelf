# scoop 下载进度、下载速度与 aria2

状态：`[in-progress]`

## 用户诉求

- scoop 安装、更新时也显示真实下载进度；下载速度一起显示。
- aria2 和 scoop 一样是需要安装的组件：准备 scoop 时一起提醒安装。

## 现状（2026-11-09 读本机 scoop 源码）

- scoop 默认下载器（`lib/download.ps1` 的 `Start-Download`）：`$progress = [console]::isoutputredirected -eq $false`，输出被重定向时不画进度条，只打印 `Downloading <url> (<大小>)...`；大小由 `filesize` 写成一位小数的 B / KB / MB / GB（1024 进制）。下载写到缓存的 `<app>#<版本>#<url>.download`，`FileStream` 顺序写入，不预分配，结束后改名。
- 装了 aria2 且 `aria2-enabled` 不为 false 时用 aria2：scoop 把 aria2 的读数行写成 `` `rDownload: [#<gid> 12MiB/50MiB(24%) CN:5 DL:3.2MiB ETA:12s] ``。aria2 默认预分配文件，文件大小不可用。
- UniGetUI 同样重定向输出：默认下载器下也只有那一行；它显示的 scoop 进度来自 aria2 的读数行。

## 方案

1. **scoop 进度**（`internal/source/scoop/progress.go`）
   - 默认下载器：`Downloading … (<大小>)` 给出总大小；下载阶段每 500 毫秒用 `os.Stat` 读缓存目录里本次写入的 `<app>#*.download` 的实际大小作为已下载。缓存目录按 scoop 的顺序：`SCOOP_CACHE`、`config.json` 的 `cache_path`、`SCOOP`（或 `~\scoop`）下的 `cache`。
   - aria2：解析 `Download: [#… 已下载/总量(百分比)…]` 得到字节。
   - 解压、运行安装程序等阶段没有进度，保持阶段文字和不确定动画。
2. **下载速度**：`source.Progress` 增加 `Speed`（字节/秒），由 `tasklog` 按最近约 3 秒的已下载字节变化算出，来源不用报告；阶段变化或字节回退时重新计算。只有字节的来源有速度（scoop、GitHub）；winget 只有百分比，没有速度。经 `TaskEntry.speed` 到前端，显示为“12.3 MB / 50 MB · 3.2 MB/s”。
3. **aria2 一起安装**
   - `scoop.Check` 在找到 scoop 时检查 aria2：没装、且用户没有把 `aria2-enabled` 设为 false，就报告缺少辅助组件 aria2（`Availability.Helper`）。
   - `scoop.Fix`：没有 scoop 时先装 scoop，然后 `scoop install main/aria2`；已有 scoop 时只装 aria2。
   - 接口：`SourceInfo.helper`（来源可用但缺少的辅助组件）。首次「准备安装来源」：缺 scoop 时卡片为“scoop + aria2”，已有 scoop 只缺 aria2 时卡片为“aria2”；设置页的 scoop 行同样给出安装按钮。
   - 用户显式点击才安装，与安装 scoop 相同；不在后台自动安装。

## 不变量

- 进度只用于显示，成败仍由执行前后的复查决定。
- 没有真实数值不编造；下载和安装不拼成一个整体百分比。
- 不新增 Go 或前端依赖。

## 验收

- 单元测试：`Downloading` 行大小解析、aria2 读数行解析、缓存目录查找、下载文件大小轮询；`tasklog` 速度计算；`scoop.Check` 的 aria2 判断（脚本输出）；前端进度文字含速度。
- 本机只读验证：`scoop download <app>` 只下载到缓存、不安装，确认输出格式和 `.download` 文件增长，测完删除缓存。
- 安装 aria2 和真实安装 scoop 软件会改动本机，需要用户授权。
- 全套检查：`gofmt`、`go vet ./...`、`go tool mygo vet`、`go tool mygo generate`、`CGO_ENABLED=0 go test ./...`、前端 `check` / `test`、`go tool mygo build`。

## 回滚

三部分互相独立：去掉 scoop 进度轮询、`Speed` 字段为 0 时前端不显示速度、`Helper` 为空时不出现 aria2 提示。
