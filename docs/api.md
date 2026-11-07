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

缓存放在 `%LocalAppData%\AppShelf\`（MyGo 的 `PathCache`），只用于展示，删掉后会重新下载或读取：

| 目录 | 内容 |
|---|---|
| `icons/` | UniGetUI 图标库 `database.json`（每周检查一次更新）、下载的图标（按地址的 SHA-256 命名），以及失败标记 `.fail`（24 小时内不再重试） |
| `details/` | 软件详情：winget 7 天、scoop 1 天、GitHub 1 小时；GitHub 读不到 release 时不缓存 |

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
      "source": { "type": "winget", "id": "Microsoft.VisualStudioCode" }
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
- 软件按来源匹配。已有的软件保留自己的名称、分类、检测规则和安装参数，只追加导入文件里的标签；它没有分类时才采用导入文件的分类。没有的软件新增。
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

## 前端接口

前端（`frontend/`）只通过 `internal/api` 绑定的服务使用 Go 的能力。`go tool mygo generate` 按 Go 类型生成带类型的客户端 `frontend/src/mygo.ts`，类型名和 JSON 字段以 `internal/api/dto.go` 为准；改了服务或类型后要重新生成。

### 绑定服务

| 服务 | 方法 | 说明 |
|---|---|---|
| `Shelf` | `Snapshot`、`Refresh` | 清单和每个软件的状态（`status` 为 `null` 表示还没读到）；`Refresh` 就是“检查更新”：检查来源、重读全部状态，再刷新本机已装 |
| | `SaveApp`、`AddPackages`、`DeleteApps`、`UndoDelete` | 手动添加或编辑一个软件；批量加入搜索结果或本机程序（可同时新建分类和标签，每项单独成败）；删除返回撤销令牌，撤销时恢复软件和 GitHub 安装记录 |
| | `SetCategory`、`SetTag`、`Add/Rename/DeleteCategory`、`Add/Rename/DeleteTag` | 批量设分类、加或去标签；分类和标签的管理 |
| | `Export`、`ReadImport`、`ApplyImport` | 导出到用户选择的文件；导入先读文件返回合并预览和令牌，确认后再合并 |
| `Tasks` | `Preview`、`Start` | 预览每个软件将安装、更新还是跳过及原因；`Start` 重新预览后只把可执行的加入队列，没有可执行的返回错误 |
| | `List`、`Stop`、`Resume`、`Retry`、`ClearDone`、`OpenLog` | 队列 |
| `Sources` | `Overview`、`InstallModule`、`Search`、`CheckGitHub` | 来源是否可用和 GitHub 额度；安装 winget 的 PowerShell 模块；编辑软件时搜索 winget 或 scoop；检查 GitHub 仓库会装哪个文件 |
| `Discover` | `Search(query, Channel<SearchBatch>)` | winget 和 scoop 并行搜索，每个来源的结果一到就发出；新的搜索取消上一次 |
| | `SearchGitHub`、`Details`、`SuggestRules` | GitHub 搜索（按需）；软件详情（只用于展示）；GitHub 软件的检测规则建议（先本机已装的程序，最后是仓库名） |
| `Installed` | `List`、`Refresh` | 本机已装的程序（winget、scoop、注册表、应用商店），标出已在清单的和可更新的；第一次调用时开始读取 |
| | `PreviewUpdates`、`StartUpdates` | 按已识别的本机 key 批量预览、确认后直接更新 winget / scoop 软件，不要求先加入清单 |
| `About` | `Info`、`CheckForUpdates`、`OpenLogs`、`OpenDataDir`、`OpenURL`、`SetTheme` | 版本、自更新、目录、用默认浏览器打开链接（只允许 http/https）、外观 |

### 事件

只有一个事件 `changed`：`{ kinds: ("shelf" | "tasks" | "sources" | "installed")[], revision }`。80 毫秒内的多次变化合并成一次。前端收到后按 `kinds` 重新读取对应数据。

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
| `queue.Executor`（`internal/queue`） | 队列执行一项所需的能力 | `Status`（执行前复查）、`Run`、`OpenLog`、`Finished`（执行后复查）；由 `internal/core` 实现 |
| `core.Core`（`internal/core`） | 清单、状态、来源和队列的汇总 | 清单与状态图只在 `internal/api` 的循环 goroutine 上访问；`PreviewUpdates` 可在循环外批量读取来源，不改清单或状态图；后台结果通过 `Config.Post` 交回循环 |
| `api.App`（`internal/api`） | 绑定服务、事件 | 方法经循环 goroutine 读写 core，写完标记变化种类，由通知器合并后发 `changed` |
| `inventory`、`discover`、`icons` | 本机已装、搜索排序与详情、图标 | inventory 可提供直接更新的精确候选 key 和来源；安装状态及是否可更新仍由来源复查决定，详情和图标不参与判断 |

### 防误装流程

1. 状态读取失败、来源不可用、结果无法解析时，状态记为“未知”或“不可用”，不允许安装。
2. 预览只用于展示；队列执行到某一项时再复查一次：已安装就跳过，状态不确定就记为失败、不执行。
3. 执行后再复查：安装程序报告成功、但仍检测不到软件时，记为失败。
4. winget 安装使用精确 ID 并指定 `winget` 源；scoop 按软件名匹配所有 bucket（scoop 同名软件只能装一个）；GitHub 安装包无法唯一确定时拒绝安装，下载后校验大小和 SHA-256。

## 变更规则

- API 或 schema 变更应写清兼容性影响。
- 计划阶段的草案写入 `plans/`；确定后的长期合同同步到本文件或对应专题文档。

本机列表的 `Notes` 保留 winget 预检查不可用和读取失败的原因；当来源不可用时，本机注册表仍可显示，但不将缺少更新信息解释成已确认没有更新。

### 本机直接更新

- `Installed.PreviewUpdates(keys)`：只接受本机列表 key，Go 端解析精确 winget / scoop 来源，分组批量读取当前状态，返回现有 PreviewEntry；不写清单。
- `Installed.StartUpdates(keys)`：重新解析和检查后，只以 Upgrade 动作加入共享队列；执行前后复查，失败可重试。
- 已在清单的软件复用原 App ID；其他目标使用由来源 key 派生的临时 ID，任务按来源身份去重，避免清单入口与本机入口重复排队。
- 未安装、状态未知、来源不可用、来源外安装或无更新时跳过；来源不明确的本机 / 商店项不接受直接更新。原有安装入口仍要求软件在清单里。
- 不自动创建 catalog.json、添加软件、分类或标签；更新完成后通过现有队列通知重新读取本机状态。
