# 运行与部署说明

> 本文件维护项目如何启动、配置、构建、测试和部署。README 只保留最短入口。

## 本地启动

需要 Go（版本以 `go.mod` 为准）、Bun（前端的依赖和构建）和 WebView2（Windows 11 自带）。MyGo CLI 作为 Go tool 锁定在 `go.mod` 中，不需要全局安装。

```bash
bun install --cwd frontend   # 第一次运行前安装前端依赖
go tool mygo doctor          # 检查本机环境
go tool mygo dev             # 开发模式：启动 Vite+ 开发服务器（6173 端口），前端热更新，Go 改动后重启
```

`mygo.json` 的 `devCommand`、`buildCommand` 分别运行 `frontend/` 的 `dev`、`build` 脚本，构建结果在 `frontend/dist/`，打包时嵌入 exe。

AppShelf 只允许运行一个实例；再次启动时，已经打开的窗口会回到前台。

## 运行环境

| 依赖 | 用途 | 没有时 |
|---|---|---|
| winget（App Installer） | winget 来源的安装和更新 | winget 软件显示“不可用”，不能安装 |
| PowerShell 模块 `Microsoft.WinGet.Client` | 读取 winget 软件的安装状态、可用更新和搜索；安装、更新、卸载经它调用 winget 的 COM 接口，显示真实下载进度（模块无法加载时退回 winget 命令，只有阶段） | 第一次检测到缺少时弹出「准备安装来源」，可直接安装；之后在“设置 → 安装来源”点“安装模块”。执行 `Install-Module Microsoft.WinGet.Client -Scope CurrentUser` |
| scoop | scoop 来源，可选 | scoop 软件显示“不可用：未安装 scoop”，不影响其他来源；第一次检测到时在「准备安装来源」里可直接安装，之后在“设置 → 安装来源”点“安装 scoop”。执行官方安装器 `irm get.scoop.sh \| iex`（当前用户，不需要管理员；以管理员运行 AppShelf 时不能安装），再 `scoop install aria2` |
| aria2（scoop 的辅助组件） | scoop 多线程下载 | 没有时 scoop 用自带下载器，照常可用；已有 scoop 但缺 aria2 时，「准备安装来源」和“设置 → 安装来源”提示安装；执行 `scoop config aria2-enabled false` 后不再提示 |
| 网络访问 `api.github.com` | GitHub 来源的最新 release 和下载 | 是否已装照常显示（检测只看本机注册表），但读不到最新 release：不能检查更新，安装会失败并说明原因 |

AppShelf 调用的是系统自带的 Windows PowerShell 5.1（`%SystemRoot%\System32\WindowsPowerShell\v1.0\powershell.exe`），不需要 `pwsh`。每个子进程会补齐该宿主的用户、系统和内置模块搜索目录，并保留自定义路径，避免从 PowerShell 7 启动时读不到已安装的模块；不修改父进程或系统环境。

## 配置

- `mygo.json`：`name`、`identifier`、`version`、`out`（构建输出目录）、`updates`（自更新的公钥和 GitHub 仓库）、`windows.installerInclude`（安装包附加脚本 `installer.nsh`，给 NSIS 安装和卸载程序加上简体中文：中文系统上显示中文，其他系统显示英文）。
- `CHANGELOG.md`：每个版本一节，标题写成 `## 版本号`；构建时成为 GitHub Release 的更新说明。
- GitHub token（可选）：不设置时每个 IP 每小时只有 60 次请求，检查几次 GitHub 软件就会用完；用完后 GitHub 软件附注“GitHub 请求额度已用完”，不能检查更新和安装，直到额度恢复。只读公开仓库，不需要任何权限范围。两种设置方式：
  - 在“设置 → 安装来源 → GitHub 请求额度”点“填写 token”粘贴：先向 GitHub 的 `/rate_limit` 验证（不占额度），GitHub 不接受的不保存；保存后立即生效，不用重启。token 用 Windows DPAPI 以当前用户加密后存到 `github-token.bin`，界面读不回，“清除”删除该文件。
  - 环境变量 `GH_TOKEN` 或 `GITHUB_TOKEN`，启动时读取。两者都有时用设置里保存的。

  token 只放在请求头里，不写日志、不随清单导出。设置页显示 token 来自哪里以及剩余额度。

### 数据位置

| 路径 | 内容 |
|---|---|
| `%AppData%\AppShelf\catalog.json` | 软件清单，格式见 [API 与接口说明](./api.md) |
| `%AppData%\AppShelf\state.json` | 本机状态（GitHub 安装记录、每个软件的更新方式、最近更新记录） |
| `%AppData%\AppShelf\settings.json` | 外观（跟随系统、浅色、深色）；更新设置 `updates`：`checkEvery`（`off`、`1h`、`3h`、`6h`、`12h`、`24h`，默认 `6h`）、`quiet`（不发通知）、`exitOnClose`（关闭窗口即退出；默认不写，关闭窗口后留在托盘）；`sourceSetupSeen`（已回答首次「准备安装来源」提示，不再自动弹出） |
| `%AppData%\AppShelf\github-token.bin` | 设置里填写的 GitHub token，以当前 Windows 用户的 DPAPI 加密；复制到别的电脑或用户下无法解密。不入库、不随清单导出 |
| 当前用户的 `HKCU\...\Run` 启动项 | 打开“开机时启动”后由 MyGo 写入，可在设置里关闭，或在任务管理器的“启动应用”里禁用 |
| `%AppData%\AppShelf\logs\` | 每次安装、更新或卸载一份日志，`YYYYMMDD-HHMMSS-<install|upgrade|uninstall>-<软件 ID>.log`（Windows 不允许的字符换成 `_`）；任务面板里直接显示日志内容，“打开日志文件”打开完整文件 |
| `%AppData%\AppShelf\backups\` | 清理卸载残留前导出的注册表，每次清理一个 `leftovers-<时间>-<软件名>\` 目录，里面是 `.reg` 文件，双击可导回；不会自动删除。文件和目录残留移到回收站，不在这里 |
| `%AppData%\AppShelf\updater.json` | MyGo 更新器的设置 |
| `%AppData%\AppShelf\window-state.json` | MyGo 记住的窗口位置和大小 |
| `%LocalAppData%\AppShelf\icons\`、`details\` | 图标和软件详情的缓存，只用于展示，可以随时删除 |
| `%LocalAppData%\AppShelf\WebView2\` | WebView2 的页面数据，包括各页的分栏宽度和排序方式 |
| 系统临时目录下的 `AppShelf\` | GitHub 安装包的下载位置，安装结束后删除 |

## 测试与构建

```bash
gofmt -l .                      # 应无输出
go vet ./...
go tool mygo vet
CGO_ENABLED=0 go test ./...     # 绑定服务的测试用假来源，不需要窗口
bun run --cwd frontend check    # 前端格式、lint 和类型检查（vp check）
bun run --cwd frontend test     # 前端单元测试（vp test）
go tool mygo build              # 先构建前端，再输出到 build/windows-amd64/：exe 和 NSIS 安装包
```

- 改了 `internal/api` 的服务或类型后运行 `go tool mygo generate`，重新生成 `frontend/src/mygo.ts`（入库）。
- `bun run --cwd frontend fmt` 按项目格式整理前端代码；`frontend/src/components/ui/` 是 shadcn 生成的组件，不参与格式和 lint 检查。

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

- 会真实卸载软件的测试同样默认不运行，需要用户授权并指定一个可以卸载的测试程序：

| 环境变量 | 作用 |
|---|---|
| `APPSHELF_UNINSTALL_TEST=1` | 打开卸载集成测试 |
| `APPSHELF_UNINSTALL_REG` | 测试程序的注册表项，写成 `<Machine|User>\<X64|X86>\<Uninstall 下的键名>` |

例如：`APPSHELF_UNINSTALL_TEST=1 APPSHELF_UNINSTALL_REG='User\X64\<键名>' CGO_ENABLED=0 go test ./internal/uninstall -run TestUninstallRealOptIn -v`。它按注册表的卸载命令静默卸载，结束后检查注册表项已不在。

- 只读的本机测试（`internal/uninstall` 为本机每个程序生成卸载方式、读签名；`internal/watch` 监听真实注册表；`internal/detect` 扫描注册表）默认运行，不改变本机。`internal/leftover` 和 `internal/api` 的残留测试会在 `%LOCALAPPDATA%\AppShelfTest-*` 和 `HKCU\Software\AppShelfTest-*` 下造测试数据再清理：被清理的测试文件会进回收站，其余测试数据在测试结束时删除。

## 部署

自更新使用 MyGo 的更新机制：`mygo.json` 的 `updates.github` 指向公开仓库 `Zker67/appshelf`，安装版的 AppShelf 读取该仓库最新正式 release 中的 `update-windows-amd64.json`，下载更新包并用内置公钥验证签名后替换自己。取文件的方式见 [selfupdate.go](../selfupdate.go)：清单地址带随机参数 `?t=` 避开中间缓存；github.com 6 秒内不开始响应或出错时，改走 GitHub API（`api.github.com/repos/Zker67/appshelf/releases/...`，不登录每小时 60 次，只在 github.com 失败时才用）取同一个文件；网络问题、超时、429 或 5xx 时自动重试，共 5 次。下载到一半断开不会接着下，右下角提示失败后点「重试」即可。开发版（`go tool mygo dev`）不能自更新，“设置 → 关于”会说明。

检查和安装的流程与 Vibe Atlas 相同（[internal/api/selfupdate.go](../internal/api/selfupdate.go)），不用 MyGo 的 updater 插件和更新窗口：启动 3 秒后在后台检查一次，之后按“设置 → 关于 → 自动检查新版本”（启动时检查 / 每小时 / 每 6 小时 / 每天 / 不自动检查）；发现新版本时窗口右下角提示「去更新」「稍后」，点「去更新」才下载，提示里显示进度，装好后自动重新打开；「稍后」在本次运行内不再提示。“设置 → 关于”可以手动检查，也能看到所有版本。

### 签名密钥

- 用 `go tool mygo keygen` 生成，私钥在 `%AppData%\mygo\update-keys\mygo-update.key`，公钥已写入 `mygo.json`。
- 私钥绝不能提交到仓库；它丢失后，已安装的 AppShelf 再也收不到更新，泄露后别人可以给用户推送代码。请另外备份到安全的位置。

### 发布一个版本

1. 修改 `mygo.json` 的 `version`，在 `CHANGELOG.md` 顶部写上 `## 新版本号` 和更新说明。
2. 本地验证：`gofmt -l .`、`go vet ./...`、`go tool mygo vet`、`CGO_ENABLED=0 go test ./...`，以及前端 `check` 和 `test`；核对文档入口和计划状态。获得对应授权后提交并推送代码，为该提交创建并推送 `v<版本号>` 标签，保证发布标签对应构建源码。
3. 带私钥构建并上传（需要已安装并登录 GitHub CLI `gh`）：

   ```powershell
   try {
       $env:MYGO_UPDATER_PRIVATE_KEY = [IO.File]::ReadAllText(
           (Join-Path $env:APPDATA 'mygo/update-keys/mygo-update.key')
       ).Trim()
       go tool mygo build -upload
       if ($LASTEXITCODE -ne 0) { throw '构建或上传失败' }
   } finally {
       Remove-Item Env:\MYGO_UPDATER_PRIVATE_KEY -ErrorAction SilentlyContinue
   }
   ```

   `-upload` 通过 `gh` 在 `Zker67/appshelf` 上为 `v<版本号>` 创建**草稿** release，上传安装包、更新包、`update-windows-amd64.json`、`install.ps1` 和 `SHA256SUMS`。
4. 检查标签提交、草稿附件与 SHA-256；若已用私钥完成本地构建，也可以用 `gh release create --draft --verify-tag` 上传这些产物，避免重复构建。检查草稿内容后发布：`gh release edit v<版本号> --repo Zker67/appshelf --draft=false`。发布后它成为 latest release，已安装的 AppShelf 才能看到更新。

只构建、不带私钥时，`build/` 里没有更新包和 `update-windows-amd64.json`，这样的版本不能发布为更新。

用户首次安装可以下载 release 里的 `appshelf-setup-<版本号>-amd64.exe`，或者执行 `irm https://github.com/Zker67/appshelf/releases/latest/download/install.ps1 | iex`（安装到 `%LOCALAPPDATA%\Programs\AppShelf`，不需要管理员权限）。

## 故障排查

| 现象 | 处理 |
|---|---|
| 只剩“本机”类型且无法显示可更新，或 winget 提示缺少模块 | 先使用 0.3.0 或更新版本重新启动，修复继承 PowerShell 7 模块路径的问题；若确实没有安装模块，再在“设置 → 安装来源”点“安装模块”，或手动执行 `Install-Module Microsoft.WinGet.Client -Scope CurrentUser` |
| 安装 scoop 失败 | 在“设置 → 关于 → 安装日志”打开 `*-setup-scoop.log` 查看官方安装器的输出；以管理员身份运行的 AppShelf 不能安装 scoop，请以普通用户打开 |
| GitHub 软件附注“GitHub 请求额度已用完” | 在“设置 → 安装来源”填写 GitHub token（立即生效），或设置 `GH_TOKEN` 后重新启动 AppShelf，或等待提示里的恢复时间 |
| 某一项安装失败 | 任务面板里点“日志”查看安装命令和完整输出；安装程序需要管理员权限时会弹出 UAC，取消 UAC 会记为“已取消管理员授权” |
| 启动时提示清单文件无法读取 | 原文件已改名为 `catalog.json.broken-<时间>`，修好后可以从“我的清单”左下角的“导入”导回 |
| 图标显示成首字母 | 图标库里没有、本机程序也没有图标时就是这样；网络恢复后图标会补上，优先使用本机图标；临时下载失败 2 分钟后可重试，可在“本机已装”点刷新重新请求；没有开始菜单入口的软件包会读取自身的 Logo |

### 空间统计只读核对

`$env:APPSHELF_SPACE_TEST='1'; $env:CGO_ENABLED='0'; go test ./internal/api -run TestSpaceLocalReadOnly -v -count=1`：按本机注册表核对 Bitwarden、Docker Desktop，并检查存在的 Dia 应用包数据目录，输出路径、文件逻辑长度和是否部分统计；不安装、卸载或清理软件。未安装的软件不参与此核对。
