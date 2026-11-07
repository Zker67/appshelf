# internal/source

安装来源。`source.go` 定义所有来源共用的接口和状态类型，三个子包分别接入 winget、scoop 和 GitHub Releases。界面和队列只认接口，新增来源不需要改它们。

## 接口

| 方法 | 作用 | 谁调用 |
|---|---|---|
| `Type()` | 来源类型，对应清单里的 `source.type` | `core` |
| `Check(ctx)` | 来源是否可用；`Fixable` 表示 AppShelf 可以帮用户修好（目前只有 winget 缺模块） | 启动时、设置页“重新检查” |
| `Statuses(ctx, apps)` | 一次读出该来源所有软件的状态 | 启动时、“检查更新” |
| `Recheck(ctx, app)` | 只读一个软件的状态 | 队列执行每一项的前后 |
| `Install` / `Upgrade` | 安装、更新，输出写进该项的日志 | 队列 |
| `Search(ctx, q)`（`Searcher`） | 搜索 winget、scoop；GitHub 用自己的 `SearchRepos` | 发现页、编辑对话框 |

三个来源另外有只用于展示的方法（本机已装、软件详情、GitHub 搜索），见各节末尾。它们的结果不参与安装判断。

状态：`Unknown`（读取失败，不允许安装）、`NotInstalled`、`Installed`、`UpdateAvailable`、`Unavailable`（来源不可用）。`External` 表示软件是由清单里的检测规则认出的，不是从该来源装的：不安装、也不从该来源更新。

## winget（`winget/`）

| 方法 | 实现 |
|---|---|
| `Check` | 找得到 `winget`，并且已安装 PowerShell 模块 `Microsoft.WinGet.Client`；缺模块时 `InstallModule` 可以为当前用户安装 |
| `Statuses` | 一次 `Get-WinGetPackage` 全量查询（本机实测 318 项约 2.3 秒，含 PowerShell 启动），按 ID 不区分大小写地挑出清单里的软件；`IsUpdateAvailable` 和 `AvailableVersions` 给出可用更新 |
| `Recheck` | `Get-WinGetPackage -Id <id> -MatchOption EqualsCaseInsensitive` |
| `Search` | `Find-WinGetPackage -Query <q> -Source winget`，取前 50 项 |
| `Install` | `winget install --id <id> --exact --source winget --accept-package-agreements --accept-source-agreements --disable-interactivity --no-upgrade` |
| `Upgrade` | 同上，动词换成 `upgrade`，没有 `--no-upgrade` |

- 检测不区分大小写，宁可多认出已装、跳过安装；安装用 `--exact`，大小写不符时 winget 找不到包而失败，也不会装错。
- 没能关联到 winget 源的已装软件 ID 以 `ARP\`、`MSIX\` 开头，不会和清单里的 ID 混淆。
- 安装和更新的 winget 命令同一时间只运行一个。
- 退出码：`0x8A150061`（已安装）记为跳过；`0x8A150109` 成功但需要重启；`0x8A150014`、`0x8A150016`、`0x8A15002B`、`0x8A150101`、`0x8A150102`、`0x8A15010C` 各有中文原因；其他退出码附上输出末尾。

只用于展示：

| 方法 | 实现 |
|---|---|
| `Installed(ctx, maxAge)` | `Get-WinGetPackage` 列出全部已装软件（含没能关联到源的 `ARP\`、`MSIX\` 项）和可用更新，供“本机已装”页使用；比 `maxAge` 新的结果直接复用 |
| `LatestVersion(ctx, id)` | `Find-WinGetPackage -Id <id> -MatchOption EqualsCaseInsensitive -Source winget` 的版本，用来读 winget-pkgs 里对应版本的清单文件（软件详情） |

## scoop（`scoop/`）

| 方法 | 实现 |
|---|---|
| `Check` | PowerShell 中找得到 `scoop` 命令 |
| `Statuses` | `scoop export`（JSON）读已装软件；先 `scoop update` 更新 scoop 和 bucket，再 `scoop status -l` 读可用更新 |
| `Recheck` | `scoop export` 加 `scoop status -l`，不联网更新 |
| `Search` | `scoop search <q>`，`q` 先转义（scoop 把它当正则表达式），取前 30 项 |
| `Install` | bucket 没有添加时先添加：有 `bucketUrl` 就 `scoop bucket add <bucket> <url>`，否则要求 bucket 在 `scoop bucket known` 中；然后 `scoop install <bucket>/<name>` |
| `Upgrade` | `scoop update <name>` |

- scoop 同名软件只能装一个，所以按软件名匹配所有 bucket：从别的 bucket 装的也算已装，并注明“通过 X 安装”。
- 读取状态、安装和更新的 scoop 调用同一时间只运行一个（搜索不受限制）。scoop 的退出码不总是可靠，最终以执行后的复查为准。
- 本机没有 scoop，scoop 的实现按 scoop 源码编写，真实运行未验证；只读集成测试在没有 scoop 时跳过。

只用于展示：

| 方法 | 实现 |
|---|---|
| `Installed(ctx)` | `scoop export` 加 `scoop status -l`，列出 scoop 装的全部软件和本地 bucket 已知的更新；不联网更新 bucket |
| `Info(ctx, bucket, name)` | `scoop info <bucket>/<name>`，软件详情里的简介、主页、许可证和版本 |

## GitHub Releases（`github/`）

| 方法 | 实现 |
|---|---|
| `Check` | 总是可用；没有 token 时提示额度限制 |
| `Statuses` / `Recheck` | 扫描一次注册表，按检测规则判断是否已装、读出已装版本；`GET /repos/<repo>/releases/latest` 读最新正式版，最多 4 个并发请求 |
| `Install` / `Upgrade` | 选出安装包 → 下载到临时目录 → 校验大小和 SHA-256（GitHub 给出 `digest` 时）→ 启动安装程序并等待 → 删除安装包 → 在 `state.json` 记下 tag 和文件名 |

- 选包（参考 GitHub Store）：只考虑 `.msi` 和 `.exe`，去掉 debug、symbols、pdb；有 `assetPattern` 时先按它筛选；`.msi` 优先于 `.exe`，再按本机架构（amd64、arm64、386）打分。最高分有两个以上并列时**拒绝安装**，提示填写 `assetPattern`。
- 更新判断：AppShelf 记录的 tag 等于最新 tag 时为最新；否则比较注册表里的版本和最新 tag；版本无法比较但有记录时视为可更新；都没有时注明“无法比较版本”。
- 启动安装程序用 `ShellExecuteExW`，需要管理员权限的安装程序会弹出 UAC。`.msi` 用 `msiexec /i "<文件>" /norestart` 加安装参数，没有填写参数时用 `/passive`；`.exe` 直接以安装参数为命令行，不填写时就是交互式安装。
- 退出码：`0` 成功；`3010`、`1641` 成功但需要重启；`1602` 已取消；`1618` 另一个安装正在进行；其他记为失败。
- token 从环境变量 `GH_TOKEN` 或 `GITHUB_TOKEN` 读取，只放在请求头里。

只用于展示：

| 方法 | 实现 |
|---|---|
| `SearchRepos(ctx, q, n)` | `GET /search/repositories?q=<q> in:name,description`；没有 token 时每分钟 10 次，只在用户要求时搜索 |
| `SearchRate()` | 搜索接口的额度，按响应头 `X-RateLimit-Resource: search` 单独记录 |
| `Repository(ctx, repo)` | `GET /repos/<repo>`：简介、主页、许可证、星标、是否归档 |
| `Describe(ctx, app)` | 最新 release 和按选包规则会装的文件，用于编辑对话框的“检查最新版本”和软件详情 |
