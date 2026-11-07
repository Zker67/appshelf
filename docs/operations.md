# 运行与部署说明

> 本文件维护项目如何启动、配置、构建、测试和部署。README 只保留最短入口。

## 本地启动

需要 Go，版本以 `go.mod` 为准。原生 UI 不依赖 WebView2 或 Bun。MyGo CLI 作为 Go tool 锁定在 `go.mod` 中，不需要全局安装。

```bash
go tool mygo doctor   # 检查本机环境
go tool mygo dev      # 开发模式，热重载
```

AppShelf 只允许运行一个实例；再次启动时，已经打开的窗口会回到前台。

## 运行环境

| 依赖 | 用途 | 没有时 |
|---|---|---|
| winget（App Installer） | winget 来源的安装和更新 | winget 软件显示“不可用”，不能安装 |
| PowerShell 模块 `Microsoft.WinGet.Client` | 读取 winget 软件的安装状态、可用更新和搜索 | 设置页提示“安装模块”，确认后执行 `Install-Module Microsoft.WinGet.Client -Scope CurrentUser` |
| scoop | scoop 来源，可选 | scoop 软件显示“不可用：未安装 scoop”，不影响其他来源 |
| 网络访问 `api.github.com` | GitHub 来源的最新 release 和下载 | 是否已装照常显示（检测只看本机注册表），但读不到最新 release：不能检查更新，安装会失败并说明原因 |

AppShelf 调用的是系统自带的 Windows PowerShell 5.1（`%SystemRoot%\System32\WindowsPowerShell\v1.0\powershell.exe`），不需要 `pwsh`。

## 配置

- `mygo.json`：`name`、`identifier`、`version`、`out`（构建输出目录）、`updates`（自更新的公钥和 GitHub 仓库）。
- `CHANGELOG.md`：每个版本一节，标题写成 `## 版本号`；构建时成为自更新窗口里的更新说明。
- 环境变量 `GH_TOKEN` 或 `GITHUB_TOKEN`（可选）：GitHub API 的 token。不设置时每个 IP 每小时只有 60 次请求，检查几次 GitHub 软件就会用完；用完后 GitHub 软件附注“GitHub 请求额度已用完”，不能检查更新和安装，直到额度恢复。token 只从环境变量读取，不存盘、不写日志；只读公开仓库，不需要任何权限范围。设置页显示当前是否使用了 token 以及剩余额度。

### 数据位置

| 路径 | 内容 |
|---|---|
| `%AppData%\AppShelf\catalog.json` | 软件清单，格式见 [API 与接口说明](./api.md) |
| `%AppData%\AppShelf\state.json` | 本机状态（GitHub 安装记录） |
| `%AppData%\AppShelf\logs\` | 每次安装或更新一份日志，`YYYYMMDD-HHMMSS-<install|upgrade>-<软件 ID>.log`；任务页的“日志”按钮直接打开 |
| `%AppData%\AppShelf\updater.json` | MyGo 更新器的设置 |
| `%AppData%\AppShelf\window-state.json` | MyGo 记住的窗口位置和大小 |
| 系统临时目录下的 `AppShelf\` | GitHub 安装包的下载位置，安装结束后删除 |

## 测试与构建

```bash
gofmt -l .                      # 应无输出
go vet ./...
go tool mygo vet . && go tool mygo vet ./internal/view
CGO_ENABLED=0 go test ./...     # 视图测试不需要窗口
go tool mygo build              # 输出到 build/windows-amd64/：exe 和 NSIS 安装包
```

- `go test -race` 需要 cgo，本项目的构建环境没有 cgo，所以不用 `-race`。
- winget 和 GitHub 的只读集成测试会直接调用本机 winget 和 GitHub API；本机没有 scoop 时 scoop 的集成测试自动跳过。GitHub 测试在额度用完时跳过，设置 `GH_TOKEN` 后可以稳定运行。
- 会真实安装软件的测试默认不运行，需要用户授权并指定测试软件：

| 环境变量 | 作用 |
|---|---|
| `APPSHELF_INSTALL_TEST=1` | 打开安装集成测试 |
| `APPSHELF_TEST_WINGET_ID` | winget 测试软件的包 ID，例如一个小工具 |
| `APPSHELF_TEST_SCOOP_APP` | scoop 测试软件，写成 `bucket/name` |
| `APPSHELF_TEST_GITHUB_REPO` | GitHub 测试软件的仓库 `owner/name` |
| `APPSHELF_TEST_GITHUB_DETECT` | 它的检测规则（注册表 DisplayName 正则） |
| `APPSHELF_TEST_GITHUB_ARGS` | 可选：它的安装参数 |

例如：`APPSHELF_INSTALL_TEST=1 APPSHELF_TEST_WINGET_ID=<包 ID> CGO_ENABLED=0 go test ./internal/source/winget -run TestInstall -v`。

## 部署

自更新使用 MyGo 的更新机制：`mygo.json` 的 `updates.github` 指向公开仓库 `Zker67/appshelf`，安装版的 AppShelf 读取该仓库最新正式 release 中的 `update-windows-amd64.json`，下载更新包并用内置公钥验证签名后替换自己。开发版（`go tool mygo dev`）不能自更新，设置页会说明。

### 签名密钥

- 用 `go tool mygo keygen` 生成，私钥在 `%AppData%\mygo\update-keys\mygo-update.key`，公钥已写入 `mygo.json`。
- 私钥绝不能提交到仓库；它丢失后，已安装的 AppShelf 再也收不到更新，泄露后别人可以给用户推送代码。请另外备份到安全的位置。

### 发布一个版本

1. 修改 `mygo.json` 的 `version`，在 `CHANGELOG.md` 顶部写上 `## 新版本号` 和更新说明。
2. 本地验证：`gofmt -l .`、`go vet ./...`、`CGO_ENABLED=0 go test ./...`。
3. 带私钥构建并上传（需要已安装并登录 GitHub CLI `gh`）：

   ```bash
   MYGO_UPDATER_PRIVATE_KEY="$(cat "$APPDATA/mygo/update-keys/mygo-update.key")" go tool mygo build -upload
   ```

   `-upload` 通过 `gh` 在 `Zker67/appshelf` 上为 `v<版本号>` 创建**草稿** release，上传安装包、更新包、`update-windows-amd64.json`、`install.ps1` 和 `SHA256SUMS`。
4. 检查草稿内容后发布：`gh release edit v<版本号> --repo Zker67/appshelf --draft=false`。发布后它成为 latest release，已安装的 AppShelf 才能看到更新。

只构建、不带私钥时，`build/` 里没有更新包和 `update-windows-amd64.json`，这样的版本不能发布为更新。

用户首次安装可以下载 release 里的 `appshelf-setup-<版本号>-amd64.exe`，或者执行 `irm https://github.com/Zker67/appshelf/releases/latest/download/install.ps1 | iex`（安装到 `%LOCALAPPDATA%\Programs\AppShelf`，不需要管理员权限）。

## 故障排查

| 现象 | 处理 |
|---|---|
| winget 软件全部显示“未知”，设置页提示缺少模块 | 在设置页点“安装模块”，或手动执行 `Install-Module Microsoft.WinGet.Client -Scope CurrentUser` |
| GitHub 软件附注“GitHub 请求额度已用完” | 设置 `GH_TOKEN` 后重新启动 AppShelf，或等待提示里的恢复时间 |
| 某一项安装失败 | 任务页点“日志”查看安装命令和完整输出；安装程序需要管理员权限时会弹出 UAC，取消 UAC 会记为“已取消管理员授权” |
| 启动时提示清单文件无法读取 | 原文件已改名为 `catalog.json.broken-<时间>`，修好后可以从设置页“导入清单”导回 |
