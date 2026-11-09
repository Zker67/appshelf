# API 与接口说明

> 本文件维护项目 API、schema、事件合同和跨模块接口。

AppShelf 是桌面应用，不对外提供网络 API。需要稳定维护的合同有三类：磁盘上的 JSON 文件、前端调用的 Go 绑定服务，以及代码内部的来源接口。

## 数据文件

两份文件都放在 `%AppData%\AppShelf\`（MyGo 的 `App.Path(PathUserData)`，由 `main.go` 的 `app.SetName("AppShelf")` 决定，开发版和安装版共用）。写入方式是先写同目录临时文件再改名，不会写出半份文件。启动时如果文件无法解析，会把它改名为 `<原名>.broken-YYYYMMDD-HHMMSS` 留存，用空数据启动，并在界面上提示。

| 文件 | 内容 | 是否随清单导出 |
|---|---|---|
| `catalog.json` | 软件清单：分类、标签、软件和安装来源 | 是：导入导出就是这份格式 |
| `state.json` | 本机状态：GitHub 软件的安装记录 | 否 |
| `settings.json` | 界面设置：`{"theme": "system" \| "light" \| "dark"}`，读不出时按 `system` | 否 |

winget 和 scoop 软件的安装状态、版本和更新每次都从来源实时读取，不落盘。

清理卸载残留时，注册表项和值先用 `reg export` 导出到 `%AppData%\AppShelf\backups\leftovers-<YYYYMMDD-HHMMSS>-<软件名>\NN-<路径>.reg`，双击可以导回；文件和目录移到回收站，不在这里备份。AppShelf 不会自动删除这些备份。

缓存放在 `%LocalAppData%\AppShelf\`（MyGo 的 `PathCache`），只用于展示，删掉后会重新下载或读取：

| 目录 | 内容 |
|---|---|
| `icons/` | UniGetUI 图标库 `database.json`（每周检查一次更新）、下载的图标（按地址的 SHA-256 命名），以及失败标记 `.fail`（24 小时内不再重试） |
| `details/` | 软件详情：winget 7 天、scoop 1 天、GitHub 1 小时；GitHub 读不到 release 时不缓存 |
| `installed.json` | 上次读到的本机已装列表（含大小、上次打开时间、注册表项）。启动时先显示它，同时照常重新读取，读完后替换并写回；卸载只用这次重新读到的列表，不用缓存里可能过期的卸载命令 |

### 软件清单 `catalog.json`（schemaVersion 1）

```json
{
  "schemaVersion": 1,
  "categories": [{ "id": "c_7f3k2m9q", "name": "开发" }],
  "tags": [{ "id": "t_a8d2kq4n", "name": "常用" }],
  "apps": [
    {
      "id": "a_m3x9p2kd",
      "name": "Visual Studio Code",
      "categoryId": "c_7f3k2m9q",
      "tagIds": ["t_a8d2kq4n"],
      "source": { "type": "winget", "id": "Microsoft.VisualStudioCode" },
      "note": "## 安装后
- [ ] 登录 GitHub 打开设置同步"
    },
    {
      "id": "a_q8w2e4rt",
      "name": "7-Zip",
      "source": { "type": "scoop", "bucket": "main", "name": "7zip" },
      "detect": { "displayName": "^7-Zip" }
    },
    {
      "id": "a_z5c7v9bn",
      "name": "GitHub CLI",
      "source": { "type": "github", "repo": "cli/cli", "assetPattern": "windows_amd64\\.msi$" },
      "detect": { "displayName": "^GitHub CLI$" }
    }
  ]
}
```

| 字段 | 规则 |
|---|---|
| `schemaVersion` | 必填。比程序支持的版本新时拒绝读取，提示先更新 AppShelf |
| `categories[]` / `tags[]` | `id` 为 1–64 位字母、数字、`_`、`-`；`name` 不能为空、不能有首尾空格或换行、最多 200 字；名称不区分大小写地唯一 |
| `apps[].id` | 同上的 ID 格式，唯一 |
| `apps[].name` | 同上的名称规则 |
| `apps[].categoryId` | 可省略（未分类）；填写时必须是已有分类。一个软件最多一个分类 |
| `apps[].tagIds` | 可省略；每项必须是已有标签，不能重复 |
| `apps[].source` | 必填，见下表。两个软件的来源不能相同（比较时不区分大小写） |
| `apps[].note` | 可省略：用户自己的备注，Markdown 原文，最多 20000 字（按 Unicode 码点计），不能含 NUL。保存时换行统一为 `
`，去掉首尾空行和末尾空白；空白备注不写入。只用于展示，不参与安装和检测判断 |
| `apps[].detect` | 可选（GitHub 来源必填）：认出“不是从该来源装的”同一软件。`displayName` 是正则表达式，不区分大小写地匹配注册表 Uninstall 项的 DisplayName；`exePath` 是可执行文件路径，可以写 `%LOCALAPPDATA%` 这样的环境变量。两项至少填一项，任一项命中就算已安装 |

| `source.type` | 字段 | 规则 |
|---|---|---|
| `winget` | `id` | winget 包 ID，格式 `^[A-Za-z0-9][A-Za-z0-9.+_-]{0,127}$`。只能填写 `id` |
| `scoop` | `bucket`、`name`、`bucketUrl` | `bucket` 和 `name` 必填；`bucketUrl` 可选，必须是 `https://` 开头。bucket 没有添加时，安装前先添加：有 `bucketUrl` 就用它，否则要求它在 `scoop bucket known` 里 |
| `github` | `repo`、`assetPattern`、`installerArgs` | `repo` 写成 `owner/name`；`assetPattern` 可选，是不区分大小写的正则表达式，用来在多个安装包中限定一个；`installerArgs` 可选，不能换行、最多 1024 字符 |

兼容规则：只增加可省略的字段时不升级 `schemaVersion`；改变已有字段含义、删除字段时升级，并在 `internal/catalog` 中迁移旧版本。

### 导入合并规则

导入不会覆盖现有清单，而是合并：

- 分类、标签按名称匹配（不区分大小写），已有的复用，没有的新增。
- 软件按来源匹配。已有的软件保留自己的名称、分类、检测规则、安装参数和备注，只追加导入文件里的标签；它没有分类、没有备注时才采用导入文件的分类、备注（备注不拼接）。没有的软件新增。
- 导入文件中的 ID 与现有 ID 冲突时重新生成，不会串到别的项上。
- 合并前先在确认框中显示将新增和合并的数量。

### 本机状态 `state.json`（schemaVersion 1）

```json
{
  "schemaVersion": 1,
  "github": {
    "a_z5c7v9bn": {
      "tag": "v2.63.0",
      "assetName": "gh_2.63.0_windows_amd64.msi",
      "digest": "sha256:…",
      "installedAt": "2026-11-07T06:45:06Z"
    }
  }
}
```

`github` 以软件 `id` 为键，记录 AppShelf 最后一次为它安装的 release。软件的 GitHub 仓库改变或软件被删除时，对应记录一并删除。

可省略的字段（按兼容规则不升级 `schemaVersion`，旧版本读取时忽略）：

- `updates`：以来源 key（`winget:<id>`、`scoop:<bucket>/<name>`、`github:<owner>/<repo>`，小写）为键的更新方式 `{ "mode": "auto" | "ignore", "skip": "<跳过的版本>", "until": "<暂缓到的时间>", "name": "<名称>" }`；`mode` 省略即“提醒我”，全为默认时删除该键。按来源而不是软件 ID 记录，未加入清单的本机软件也能设置；软件从清单删除时保留。
- `history`：最近 200 条更新记录，新的在前：`{ key, name, from, to, result: "succeeded" | "skipped" | "failed", message, auto, at }`。

## 前端接口

前端（`frontend/`）只通过 `internal/api` 绑定的服务使用 Go 的能力。`go tool mygo generate` 按 Go 类型生成带类型的客户端 `frontend/src/mygo.ts`，类型名和 JSON 字段以 `internal/api/dto.go` 为准；改了服务或类型后要重新生成。

### 绑定服务

| 服务 | 方法 | 说明 |
|---|---|---|
| `Shelf` | `Snapshot`、`Refresh` | 清单和每个软件的状态（`status` 为 `null` 表示还没读到）；`Refresh` 就是“检查更新”：检查来源、重读全部状态，再刷新本机已装 |
| | `SaveApp`、`AddPackages`、`DeleteApps`、`UndoDelete` | 手动添加或编辑一个软件；批量加入搜索结果或本机程序（可同时新建分类和标签，每项单独成败）；删除返回撤销令牌，撤销时恢复软件和 GitHub 安装记录 |
| | `SetCategory`、`SetTag`、`Add/Rename/DeleteCategory`、`Add/Rename/DeleteTag` | 批量设分类、加或去标签；分类和标签的管理 |
| | `SetNote` | 改一个软件的备注（Markdown），返回保存后的内容，空字符串即删除；不重新读取本机状态。`SaveApp` 编辑已有软件时保留原备注，只有新建时采用传入的备注 |
| | `Export`、`ReadImport`、`ApplyImport` | 导出到用户选择的文件；导入先读文件返回合并预览和令牌，确认后再合并 |
| `Tasks` | `Start` | 安装、更新不预览，界面点了就调用：先等正在读取的状态（如刚加入的软件，最多 2 分钟），再逐个核对，只把可执行的加入队列，没有可执行的返回错误。动作只接受 `install`、`upgrade`；`uninstall` 只能经 `Installed.StartUninstall` 加入 |
| | `List`、`Stop`、`Resume`、`Retry`、`ClearDone`、`OpenLog` | 队列。每项有动作（`install`、`upgrade`、`uninstall`）、状态、阶段 `stage`（如“正在下载”）和下载字节 `done` / `total`（未知时省略）；`interactive` 表示这个失败的卸载可以用卸载程序窗口重试，`leftovers` 表示这个成功的卸载可以查找残留 |
| | `RetryInteractive(id)` | 失败的卸载改为打开卸载程序窗口再执行一次（winget 用 `--interactive`，注册表程序用不带静默参数的卸载命令） |
| | `Log(id, from)` | 应用内日志：从第 `from` 行起的整理后的行 `lines`、实际起点 `start`（更早的行已丢弃时大于 `from`）、下次的游标 `next`、还在刷新的当前行 `live`、任务是否结束 `done`。内存里每个任务最多保留最后 2000 行，完整内容用 `OpenLog` 打开日志文件 |
| | `FindLeftovers(id)` | 为卸载成功的任务查找残留（目录、`HKCU\Software` 下的设置、快捷方式、开机启动项），返回令牌和每一项的 ID、类型、路径、分数、等级（`high` / `medium` / `low`）、理由和是否默认勾选；只读 |
| | `CleanLeftovers(token, ids)` | 清理这次查找里选中的项（只按 ID 选择，前端传不了路径）：文件和目录移到回收站，注册表先导出备份再删除；每项清理前重新检查。返回每项结果、清理数、失败数和备份目录 |
| | `OpenBackup(token)` | 打开这次清理的注册表备份目录 |
| `Sources` | `Overview`、`Fix`、`DismissSetup`、`SetGitHubToken`、`ClearGitHubToken`、`Search`、`CheckGitHub` | 来源是否可用、能否由 AppShelf 修复（`fixable`）、首次准备提示是否已回答（`setupSeen`）、GitHub token 来自哪里（`githubTokenFrom`：`saved`、`env` 或空，token 本身不回到前端）和 GitHub 额度；`SetGitHubToken` 验证后加密保存并立即使用，`ClearGitHubToken` 删除保存的 token、回落到环境变量；`Fix` 按来源安装缺少的部分（winget 的 PowerShell 模块、scoop 本身）；编辑软件时搜索 winget 或 scoop；检查 GitHub 仓库会装哪个文件 |
| `Discover` | `Search(query, Channel<SearchBatch>)` | winget 和 scoop 并行搜索，每个来源的结果一到就发出；新的搜索取消上一次。winget 先发不带版本的结果，读到版本后同一来源再发一次（前端按来源整批替换）；各来源结果缓存 10 分钟 |
| | `Warm()` | 页面显示时调用，立即返回；在后台启动 winget 搜索用的 PowerShell 并打开 winget 源，让第一次搜索也快 |
| | `SearchGitHub`、`Details`、`SuggestRules` | GitHub 搜索（按需）；软件详情（只用于展示）；GitHub 软件的检测规则建议（先本机已装的程序，最后是仓库名） |
| `Installed` | `List`、`Refresh` | 本机已装的程序（winget、scoop、注册表、应用商店），标出已在清单的和可更新的，带注册表里的大小 `size`、安装日期 `installed`（`YYYY-MM-DD`）、安装目录 `location` 和上次打开时间 `lastUsed`（`internal/usage`，没有记录时省略；列表的 `usageTracked` 为假表示 Windows 没有记录程序启动）；第一次调用时开始读取，先给出只含注册表的列表（`partial` 为真），读完 winget、scoop 和应用商店后替换。AppShelf 之外安装或卸载软件时自动重读 |
| | `StartUpdates` | 按已识别的本机 key 直接更新 winget / scoop 软件，不要求先加入清单；核对当前状态后只排队能更新的 |
| | `PreviewUninstall(keys)` | 按本机 key 预览卸载，什么都不执行：卸载方式 `method`（winget、scoop、应用商店、卸载程序）、安装程序类型 `installer`、命令 `command`、是否静默 `quiet`、能否执行 `runnable` 和原因 `plan`、提示 `notes`（正在运行、需要管理员权限、运行库等）、卸载程序的签名 `signer` / `signed`、对应的清单软件 `inShelf` |
| | `StartUninstall(keys)` | 重新预览后把能卸载的加入队列，一个都不能时返回原因；清单里的软件保留，卸载后重读它的状态 |
| `Updates` | `List`、`Check` | 清单软件和本机 winget / scoop 软件按来源 key 合并的更新（含被搁置的，`count` 只数提供的）、更新方式不是默认的软件（`rules`）、检查时间和 AppShelf 自身的新版本；`Check` 立即检查 |
| | `Start` | 按来源 key 开始更新：清单里已读到状态的走 `Tasks` 的路径，其余走 `Installed` 的路径；前端只传 key，Go 重新解析来源 |
| | `SetMode`、`Skip`、`Snooze`、`Restore` | 设为提醒 / 自动更新 / 忽略（不能自动更新时返回原因；开启时有更新立即排队）；跳过当前最新版本；暂缓 1–90 天；恢复提醒 |
| | `History`、`ClearHistory`、`Settings`、`SetSettings` | 更新记录；检查间隔、通知、托盘、开机启动、AppShelf 自身的检查和自动下载 |
| `About` | `Info`、`CheckForUpdates`、`OpenLogs`、`OpenDataDir`、`OpenURL`、`SetTheme` | 版本、自更新、目录、用默认浏览器打开链接（只允许 http/https）、外观 |

### 事件

`changed`：`{ kinds: ("shelf" | "tasks" | "sources" | "installed" | "updates")[], revision }`。80 毫秒内的多次变化合并成一次。前端收到后按 `kinds` 重新读取对应数据；`shelf`、`tasks`、`installed` 也会让更新列表重新读取。

`navigate`：页面名（目前只有 `"updates"`），点击系统通知后由 Go 发出，前端切到该页。

### 图标协议

图标由 Go 的 `icon` 协议提供，Windows 上地址是 `http://icon.localhost<路径>`（`About.Info` 的 `iconBase`）。路径由 Go 给出，前端不拼外部网址：

| 路径 | 图标 |
|---|---|
| `/w/<winget ID>` | 本机已装程序的 DisplayIcon、MSI ProductIcon、安装目录或明确匹配的开始菜单图标，其次图标库 |
| `/s/<scoop 软件名>` | 图标库 |
| `/g/<owner>/<repo>` | 图标库，其次 GitHub 上 owner 的头像 |
| `/i/<本机已装的 key>` | 按种类：winget 程序同 `/w/`，scoop 程序同 `/s/`，应用商店应用优先用包清单的 Logo，缺少时用开始菜单图标，其他程序用注册表的 DisplayIcon、MSI ProductIcon、安装目录或明确匹配的开始菜单图标 |

本机列表图标 URL 带本次扫描时间作为版本参数，刷新后重新尝试；未改变路由或数据类型。找不到图标时返回 404，前端显示首字母图标并有限重试。缓存、并发与回退策略见 [图标服务](../internal/icons/README.md)。下载只允许公网的 http(s) 地址，最多 2 MB，只接受 PNG、JPEG、GIF、WebP、SVG、ICO；SVG 以禁止脚本的 CSP 返回。

## 内部接口

| 接口/模块 | 职责 | 说明 |
|---|---|---|
| `source.Source`（`internal/source`） | 一种安装来源 | `Type`、`Check`（可用性）、`Statuses`（批量读状态）、`Recheck`（单项复查）、`Install`、`Upgrade`；能搜索的来源再实现 `source.Searcher` |
| `source.Status` | 一个软件在本机的状态 | `Unknown`（未知）、`NotInstalled`、`Installed`、`UpdateAvailable`、`Unavailable`（来源不可用）；`Blocked()` 为真时不允许安装或更新；`External` 表示由检测规则认出、不是从该来源装的 |
| `queue.Executor`（`internal/queue`） | 队列执行一项所需的能力 | `Status(ctx, item)`（执行前后复查；卸载项由 `queue` 换算成“还在 / 已不在”）、`Run(ctx, item, log)`（输出写进 `tasklog.Log`）、`OpenLog`、`Finished`；由 `internal/core` 实现 |
| `source.Progress`、`source.ReportProgress` | 任务的阶段和下载字节 | 来源报告给任务日志，`queue` 每 250 毫秒最多通知一次 |
| `tasklog.Log`（`internal/tasklog`） | 一个任务的日志 | 原样写文件，同时整理成行留在内存（最多 2000 行），供 `Tasks.Log` 增量读取 |
| `uninstall.Runner`（`internal/uninstall`） | 卸载 | `PlanFor` 只按读到的信息给出卸载方式；`Inspect` 读本机现状和签名；`Run` 执行并等卸载程序副本结束；`Present` 判断程序是否还在 |
| `leftover`（`internal/leftover`） | 卸载残留 | `Find` 只读查找并打分；`Clean` 只清理传入的项，文件进回收站、注册表先备份 |
| `watch.Watch`（`internal/watch`） | 外部变化 | 监听 Uninstall 键、应用包仓库键和 scoop `apps` 目录，变化时通知 `internal/api` 重读本机已装 |
| `core.Core`（`internal/core`） | 清单、状态、来源和队列的汇总 | 清单与状态图只在 `internal/api` 的循环 goroutine 上访问；`PreviewUpdates` 可在循环外批量读取来源，不改清单或状态图；后台结果通过 `Config.Post` 交回循环 |
| `api.App`（`internal/api`） | 绑定服务、事件 | 方法经循环 goroutine 读写 core，写完标记变化种类，由通知器合并后发 `changed` |
| `inventory`、`discover`、`icons` | 本机已装、搜索排序与详情、图标 | inventory 可提供直接更新的精确候选 key 和来源；安装状态及是否可更新仍由来源复查决定，详情和图标不参与判断 |

### 防误装流程

1. 状态读取失败、来源不可用、结果无法解析时，状态记为“未知”或“不可用”，不允许安装。
2. 预览只用于展示；队列执行到某一项时再复查一次：已安装就跳过，状态不确定就记为失败、不执行。
3. 执行后再复查：安装程序报告成功、但仍检测不到软件时，记为失败。
4. winget 安装使用精确 ID 并指定 `winget` 源；scoop 按软件名匹配所有 bucket（scoop 同名软件只能装一个）；GitHub 安装包无法唯一确定时拒绝安装，下载后校验大小和 SHA-256。

### 防误删流程

1. 卸载对象只来自 Go 自己读到的本机已装列表：前端只传 key，不能传命令、路径或注册表位置。
2. 不卸载 winget、scoop 本身和 AppShelf 自己，不卸载注册表标明 `NoRemove` 的程序、框架包和 Windows 自带的应用包；找不到卸载命令或 MSI 产品代码时不执行。
3. 每次卸载都先预览，由用户确认；排队后执行前再复查，程序已不在就跳过。
4. 执行后复查：卸载程序报告成功、但程序还在时，记为失败；复查无法确认时也记为失败；卸载程序报错、但程序已不在时，记为成功并附上卸载程序的报告。
5. 残留只在卸载成功后由用户点“查找残留”时查找，从不自动清理；只有“很可能是残留”的默认勾选。盘符根目录、系统和共享目录、AppShelf 自己的目录、其他已装程序的目录（含上下级）一律不列出、不清理；注册表只处理 `HKCU\Software` 下的键和当前用户开机启动项的值。文件进回收站，注册表先备份，备份失败就不删除。

## 变更规则

- API 或 schema 变更应写清兼容性影响。
- 计划阶段的草案写入 `plans/`；确定后的长期合同同步到本文件或对应专题文档。

本机列表的 `Notes` 保留 winget 预检查不可用和读取失败的原因；当来源不可用时，本机注册表仍可显示，但不将缺少更新信息解释成已确认没有更新。

### 本机直接更新

- `Installed.StartUpdates(keys)`：只接受本机列表 key，Go 端解析精确 winget / scoop 来源，分组批量读取当前状态，只以 Upgrade 动作加入共享队列；执行前后复查，失败可重试。
- 已在清单的软件复用原 App ID；其他目标使用由来源 key 派生的临时 ID，任务按来源身份去重，避免清单入口与本机入口重复排队。
- 未安装、状态未知、来源不可用、来源外安装或无更新时跳过；来源不明确的本机 / 商店项不接受直接更新。原有安装入口仍要求软件在清单里。
- 不自动创建 catalog.json、添加软件、分类或标签；更新完成后通过现有队列通知重新读取本机状态。

### 自动更新

- 只对更新方式为“自动更新”的软件，在一次检查结束后（或刚开启时）排队，走与手动更新相同的预览、`Upgrade` 和执行前后复查。
- 能自动更新的条件（`autoBlock`）：来源是 winget 或 scoop；不是来源外安装；本机列表里找得到；安装范围是当前用户（注册表 HKCU；scoop 非 `--global`），同一软件同时有机器和用户两份时视为不确定；版本已知；能确定安装目录（`InstallLocation`，否则 `DisplayIcon` 所在目录，排除 Program Files、系统目录等共享目录；scoop 为 `<SCOOP>\apps\<name>`）。
- 安装目录下有进程在运行时不排队，等下次检查。手动更新不弹预览，正在运行的软件可能在更新时被关闭。
- 被忽略、跳过或暂缓的更新不自动执行、不发通知；同一版本只通知一次；窗口在前台或关闭了通知时不发系统通知。
