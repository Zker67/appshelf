# internal/inventory

“本机已装”页的数据：把 winget、scoop、注册表和应用商店应用合并成一张列表，并标出哪些已在清单里。结果只用于展示和加入清单，**不参与安装判断**：是否已装只由来源查询和检测规则决定（`internal/core`）。

## 合并规则（`Build`）

| 来源 | 种类 `Kind` | 说明 |
|---|---|---|
| winget 源的包 | `winget` | 按名称关联注册表项（版本相同的优先），取发布者、图标和安装位置；名称对不上时（winget 用的是清单里的名称，如 `Everything` 对 `Everything 1.4.1.1032 (x64)`），在其余包都关联完后，再找名称互为整词前缀、且版本一致的注册表项（`looseEntry`：注册表版本相同，或注册表名称里带着这个版本号） |
| winget 列出但没有源的程序（`ARP\<范围>\<架构>\<键名>`） | `local` | 按注册表键名关联，同架构优先 |
| `MSIX\…` 和 msstore 源的包 | `store` | 应用商店应用，图标按包家族名找开始菜单里的应用 |
| scoop 装的软件 | `scoop` | 不在注册表里，排在后面 |
| 只在注册表里的程序 | `local` | 只在读不到 winget，或 winget 还没读完时出现；winget 读得到时它已经列出全部程序 |

每项有唯一的 `Key`（小写），例如 `winget:git.git`、`arp:machine\x64\<键名>`、`msix:<包名>`、`msstore:<ID>`、`scoop:7zip`；重复时加 `#n`。只在注册表里的程序也用 winget 会给它的 `arp:` 键（`ArpKey`），所以 winget 读完后键不变，页面上的选择和卸载对象不会因此失效。列表按名称排序。

关联到注册表项的条目还带有：

| 字段 | 内容 |
|---|---|
| `Size`、`InstallDate` | 注册表里的估计大小（字节）和安装日期（`YYYY-MM-DD`），没有时为空；应用商店应用和 scoop 软件的 `Size` 是统计目录得到的（见下文 `Sizes`） |
| `LastUsed` | 上次打开时间（`SetLastUsed`，见 `internal/usage`）：按安装目录、图标和卸载程序所在目录、应用包族名、scoop 名称归属；没有记录时为零值 |
| `Reg` | 整个注册表项（`detect.Entry`），卸载时用它的卸载命令、范围和键名 |
| `PackageFullName` | `MSIX\<包全名>` 条目的包全名，用 `Remove-AppxPackage` 卸载 |
| `StoreID` | msstore 源条目的 ID，用 `winget uninstall --source msstore` 卸载 |

## 与清单对照（`Match`）

一项属于清单里的哪些软件：winget ID 相同、scoop 软件名相同，或者某个软件的检测规则匹配它的名称（`RuleName`：注册表里的名称，没有时用显示名称）。

## 读取（`Load`）

`Sources` 里的 winget、scoop、注册表扫描和启动记录（`Usage`）并行读取；读不到的来源写进 `List.Notes`，其余照常列出。`List.UsageTracked` 表示 Windows 有程序启动记录。

`List.Partial` 表示只读了注册表：`internal/api` 打开“本机已装”时先用注册表扫描给出列表（`Partial` 为真），winget、scoop 和应用商店应用读完后再换成完整列表。

### 目录大小（`Sizes`）

注册表不给应用商店应用和 scoop 软件大小。`Sources.Sizes` 不为空时，`Load` 会统计它们的目录（每次同时统计 4 个），结果写进 `Size`：

| 条目 | 统计的目录 |
|---|---|
| `MSIX…` 应用包 | `PackagePath(包全名)`（`GetPackagePathByFullName`），只统计主包，不含依赖的框架包 |
| scoop 软件 | `ScoopDir(名称, 是否全局)<版本>`，没有这个版本目录时取 `current` 指向的目录 |
| msstore 源的包、名称对不上的 winget 包 | 不统计 |

`DirSize` 不跟随符号链接和 junction（例如 scoop 的 persist 数据），读不了的文件直接跳过。路径里带版本号，所以同一个目录只统计一次：`Sizes` 保留上次读取时列表里还在的目录的结果，刷新时不再重新统计（本机首次约 1.5 秒，之后约 50 毫秒）。`internal/api` 每个应用只用一个 `Sizes`。

## 测试

`inventory_test.go`：合并、关联和对照的单元测试，以及一次真实的只读读取（本机实测 318 项）。`size_test.go`：宽松关联、目录统计（含 junction）、缓存，以及真实应用包路径的测试。
