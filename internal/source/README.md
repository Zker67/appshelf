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

winget 和 scoop 另有 `Uninstall`，不在接口里：由 [internal/uninstall](../uninstall/README.md) 直接调用，卸载是否成功同样以执行后的复查为准。

## 进度

`Progress{Stage, Done, Total, Percent, Speed}` 是一个任务当前的阶段（如“正在下载”）、下载字节数（未知时为 0）、来源只给百分比时的百分比（0–100，未知时为 0），以及下载速度（字节/秒，由任务日志按最近约 3 秒的字节变化算出，来源不报告）。界面有字节总数或百分比时显示确定进度，都没有时显示不确定动画，不编造数值；下载和安装不拼成一个整体百分比。来源用 `source.ReportProgress(log, p)` 报告：交给来源的日志实现了 `ProgressReporter`（[internal/tasklog](../tasklog/README.md)）时记下进度，否则忽略。

| 来源 | 阶段 | 下载进度 |
|---|---|---|
| winget | 正在准备 → 正在下载 → 正在安装 / 正在更新 / 正在卸载 → 正在完成，来自 COM 接口的进度记录 | COM 回报的真实百分比（`Percent`），没有字节数；安装、卸载阶段有百分比时同样显示（多数安装程序没有）。退回命令行时：命令行重定向输出没有进度条，只按输出里的下载地址识别阶段，并读 `%TEMP%\WinGet\<ID>.<版本>\` 的大小作为已下载；本机 winget 用 Delivery Optimization 下载时文件会一次预分配到接近全部大小，这个数不可靠 |
| scoop | 按 scoop 输出的英文行首识别：正在添加 bucket、正在下载、正在校验、正在解压、正在运行安装程序 / 安装脚本、正在完成 | 自带下载器：输出被重定向时不画进度条，只写 `Downloading <url> (75.4 MB)...`，总量取这里的大小（按区域格式、一位小数，1024 进制）；已下载每 500 毫秒用 `os.Stat` 读缓存里本次写入的 `<app>#*.download`（顺序写入、不预分配）。aria2：解析 scoop 转发的读数行 `Download: [#… 12MiB/50MiB(24%) …]`，多个下载相加。缓存目录：`SCOOP_CACHE`、配置的 `cache_path`、`SCOOP` / 配置的 `root_path` / `~\scoop` 下的 `cache` |
| GitHub | 正在下载 → 正在校验 → 正在运行安装程序 | 下载时的实际字节；总量取 Release 资产的大小，没有时用 `Content-Length` |

状态：`Unknown`（读取失败，不允许安装）、`NotInstalled`、`Installed`、`UpdateAvailable`、`Unavailable`（来源不可用）。`External` 表示软件是由清单里的检测规则认出的，不是从该来源装的：不安装、也不从该来源更新。

## winget（`winget/`）

| 方法 | 实现 |
|---|---|
| `Check` | 找得到 `winget`，并且已安装 PowerShell 模块 `Microsoft.WinGet.Client`；缺模块时 `Fix` 为当前用户安装（`Install-Module -Scope CurrentUser`） |
| `Statuses` | 一次 `Get-WinGetPackage` 全量查询（本机实测 318 项约 2.3 秒，含 PowerShell 启动），按 ID 不区分大小写地挑出清单里的软件；`IsUpdateAvailable` 和 `AvailableVersions` 给出可用更新 |
| `Recheck` | `Get-WinGetPackage -Id <id> -MatchOption EqualsCaseInsensitive` |
| `Search` | `Find-WinGetPackage -Query <q> -Source winget -Count 50`，只读 ID 和名称，在常驻的 PowerShell（`runner.Worker`）里执行；不读版本，版本由 `SearchVersions` 补 |
| `Install` | 在 `Start-Job` 里 `Install-WinGetPackage -Id <id> -MatchOption EqualsCaseInsensitive -Source winget -Mode Default`（winget COM 接口）；先 `Get-WinGetPackage` 查到已装就返回“已安装”，不安装。模块无法加载时退回 `winget install --id <id> --exact --source winget --accept-package-agreements --accept-source-agreements --disable-interactivity --no-upgrade` |
| `Upgrade` | `Update-WinGetPackage`，参数同上；退回命令行时动词换成 `upgrade`，没有 `--no-upgrade` |
| `Uninstall` | `Uninstall-WinGetPackage -Source <winget 或 msstore> -Mode Silent`，用卸载程序窗口重试时 `-Mode Interactive`；先查不到包就视为已不在，由复查确认。退回命令行时 `winget uninstall --id <id> --exact --source <来源> --silent`（或 `--interactive`）`--accept-source-agreements --disable-interactivity`，找不到包（`0x8A150014`）同样视为已不在 |

- 检测不区分大小写，宁可多认出已装、跳过安装；安装按完整 ID 匹配（COM 不区分大小写，winget 源里的 ID 不区分大小写也唯一；命令行用 `--exact`），并指定来源，不按名称模糊安装。
- 安装、更新、卸载的进度：模块的 cmdlet 把进度写成 PowerShell 进度记录，控制台宿主读不到，嵌套 `[powershell]::Create()` 运行会一直等待（本机实测）；放进 `Start-Job` 后由外层每 300 毫秒读 `ChildJobs[0].Progress`，按行写出 `@@progress<Tab>状态<Tab>百分比`，Go 端（`com.go` 的 `comLog`）报告成进度，不写进日志；日志只记执行的 cmdlet、阶段变化和结果。结果按 `ExtendedErrorCode` 的 HRESULT 复用命令行退出码的中文说明。
- 结束 AppShelf 会终止外层 PowerShell，作业进程随之退出，但 COM 服务端已开始的安装可能继续完成，与命令行方式相同。
- 没能关联到 winget 源的已装软件 ID 以 `ARP\`、`MSIX\` 开头，不会和清单里的 ID 混淆。
- 安装、更新和卸载同一时间只运行一个（COM 和命令行共用一把锁）。
- 搜索不读模块对象的 `Version`：它要为每个结果从 CDN 下载 `versionData.mszyml` 和清单，逐个进行，50 个结果实测 30–55 秒。改为 ID、名称和版本分开读后，宽泛的词（如 `player`、`browser`）50 项约 0.2 秒、版本约 0.4 秒（本机实测）。
- 退出码：`0x8A150061`（已安装）记为跳过；`0x8A150109` 成功但需要重启；`0x8A150014`、`0x8A150016`、`0x8A15002B`、`0x8A150101`、`0x8A150102`、`0x8A15010C` 各有中文原因；其他退出码附上输出末尾。

只用于展示：

| 方法 | 实现 |
|---|---|
| `Installed(ctx, maxAge)` | `Get-WinGetPackage` 列出全部已装软件（含没能关联到源的 `ARP\`、`MSIX\` 项）和可用更新，供“本机已装”页使用；比 `maxAge` 新的结果直接复用 |
| `SearchVersions(ctx, q)` | `winget search --query <q> --source winget --count 100`，从表格里按 ID 取其后一列作为版本（winget 直接读本地索引，不下载）；ID 被截断（带 `…`）的查不到，只缺版本 |
| `Warm(ctx)` | 启动搜索用的 PowerShell、导入模块并打开 winget 源，供发现页显示时预热 |
| `LatestVersion(ctx, id)` | `Find-WinGetPackage -Id <id> -MatchOption EqualsCaseInsensitive -Source winget` 的版本，用来读 winget-pkgs 里对应版本的清单文件（软件详情） |

## scoop（`scoop/`）

| 方法 | 实现 |
|---|---|
| `Check` | PowerShell 中找得到 `scoop` 命令（找不到时先补上注册表里的用户和系统 PATH，认出 AppShelf 启动后才装的 scoop）；没有时，非管理员运行可由 `Fix` 安装。有 scoop 但用户和全局目录下都没有 `apps\aria2\current`、且配置没有把 `aria2-enabled` 设为 false 时，报告缺少辅助组件 `Helper: "aria2"` |
| `Fix` | 官方安装器 `irm get.scoop.sh \| iex` 装到当前用户；PowerShell 不允许本地脚本且没有组策略决定时，先按官方说明 `Set-ExecutionPolicy RemoteSigned -Scope CurrentUser`；装完找不到 `scoop` 即失败。以管理员运行时官方安装器拒绝，直接报错。之后（或已有 scoop 时只做这一步）缺 aria2 就 `scoop install aria2`；aria2 失败时报错并说明 scoop 本身可以使用 |
| `Statuses` | `scoop export`（JSON）读已装软件；先 `scoop update` 更新 scoop 和 bucket，再 `scoop status -l` 读可用更新 |
| `Recheck` | `scoop export` 加 `scoop status -l`，不联网更新 |
| `Search` | `scoop search <q>`，`q` 先转义（scoop 把它当正则表达式），取前 30 项 |
| `Install` | bucket 没有添加时先添加：有 `bucketUrl` 就 `scoop bucket add <bucket> <url>`，否则要求 bucket 在 `scoop bucket known` 中；然后 `scoop install <bucket>/<name>` |
| `Upgrade` | `scoop update <name>` |
| `Uninstall` | `scoop uninstall <name>`，为所有用户安装的加 `--global`（需要管理员权限） |

- scoop 同名软件只能装一个，所以按软件名匹配所有 bucket：从别的 bucket 装的也算已装，并注明“通过 X 安装”。
- 读取状态、安装、更新和卸载的 scoop 调用同一时间只运行一个（搜索不受限制）。scoop 的退出码不总是可靠，最终以执行后的复查为准。
- 本机没有 scoop，scoop 的实现按 scoop 源码编写，真实运行未验证；只读集成测试在没有 scoop 时跳过。2026-11-09 起本机装有 scoop（无其他软件），下载进度用 `scoop download go`（只下载到缓存）实测过输出格式和 `.download` 文件增长。

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
- token 来自设置里保存的（`api` 用 `internal/secret` 加密保存）或环境变量 `GH_TOKEN` / `GITHUB_TOKEN`，只放在请求头里。`UseToken` 先向 `/rate_limit` 验证（不占额度）再换用并记下新额度，401 返回 `ErrBadToken`、不换；`SetToken` 不经验证直接换用（启动时、清除时）。

只用于展示：

| 方法 | 实现 |
|---|---|
| `SearchRepos(ctx, q, n)` | `GET /search/repositories?q=<q> in:name,description`；没有 token 时每分钟 10 次，只在用户要求时搜索 |
| `SearchRate()` | 搜索接口的额度，按响应头 `X-RateLimit-Resource: search` 单独记录 |
| `Repository(ctx, repo)` | `GET /repos/<repo>`：简介、主页、许可证、星标、是否归档 |
| `Describe(ctx, app)` | 最新 release 和按选包规则会装的文件，用于编辑对话框的“检查最新版本”和软件详情 |
