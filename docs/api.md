# API 与接口说明

> 本文件维护项目 API、schema、事件合同和跨模块接口。

AppShelf 是桌面应用，不对外提供网络 API。需要稳定维护的合同有两类：磁盘上的两份 JSON 文件，以及代码内部的来源接口。

## 数据文件

两份文件都放在 `%AppData%\AppShelf\`（MyGo 的 `App.Path(PathUserData)`，由 `main.go` 的 `app.SetName("AppShelf")` 决定，开发版和安装版共用）。写入方式是先写同目录临时文件再改名，不会写出半份文件。启动时如果文件无法解析，会把它改名为 `<原名>.broken-YYYYMMDD-HHMMSS` 留存，用空数据启动，并在界面上提示。

| 文件 | 内容 | 是否随清单导出 |
|---|---|---|
| `catalog.json` | 软件清单：分类、标签、软件和安装来源 | 是：导入导出就是这份格式 |
| `state.json` | 本机状态：GitHub 软件的安装记录 | 否 |

winget 和 scoop 软件的安装状态、版本和更新每次都从来源实时读取，不落盘。

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

## 内部接口

| 接口/模块 | 职责 | 说明 |
|---|---|---|
| `source.Source`（`internal/source`） | 一种安装来源 | `Type`、`Check`（可用性）、`Statuses`（批量读状态）、`Recheck`（单项复查）、`Install`、`Upgrade`；能搜索的来源再实现 `source.Searcher` |
| `source.Status` | 一个软件在本机的状态 | `Unknown`（未知）、`NotInstalled`、`Installed`、`UpdateAvailable`、`Unavailable`（来源不可用）；`Blocked()` 为真时不允许安装或更新；`External` 表示由检测规则认出、不是从该来源装的 |
| `queue.Executor`（`internal/queue`） | 队列执行一项所需的能力 | `Status`（执行前复查）、`Run`、`OpenLog`、`Finished`（执行后复查）；由 `internal/core` 实现 |
| `core.Core`（`internal/core`） | 清单、状态、来源和队列的汇总 | 界面只通过它读写数据；后台结果通过 `Config.Post` 回到 UI 线程 |

### 防误装流程

1. 状态读取失败、来源不可用、结果无法解析时，状态记为“未知”或“不可用”，不允许安装。
2. 预览只用于展示；队列执行到某一项时再复查一次：已安装就跳过，状态不确定就记为失败、不执行。
3. 执行后再复查：安装程序报告成功、但仍检测不到软件时，记为失败。
4. winget 安装使用精确 ID 并指定 `winget` 源；scoop 按软件名匹配所有 bucket（scoop 同名软件只能装一个）；GitHub 安装包无法唯一确定时拒绝安装，下载后校验大小和 SHA-256。

## 变更规则

- API 或 schema 变更应写清兼容性影响。
- 计划阶段的草案写入 `plans/`；确定后的长期合同同步到本文件或对应专题文档。
