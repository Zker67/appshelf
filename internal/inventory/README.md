# internal/inventory

“本机已装”页的数据：把 winget、scoop、注册表和应用商店应用合并成一张列表，并标出哪些已在清单里。结果只用于展示和加入清单，**不参与安装判断**：是否已装只由来源查询和检测规则决定（`internal/core`）。

## 合并规则（`Build`）

| 来源 | 种类 `Kind` | 说明 |
|---|---|---|
| winget 源的包 | `winget` | 按名称关联注册表项（版本相同的优先），取发布者、图标和安装位置 |
| winget 列出但没有源的程序（`ARP\<范围>\<架构>\<键名>`） | `local` | 按注册表键名关联，同架构优先 |
| `MSIX\…` 和 msstore 源的包 | `store` | 应用商店应用，图标按包家族名找开始菜单里的应用 |
| scoop 装的软件 | `scoop` | 不在注册表里，排在后面 |
| 只在注册表里的程序 | `local` | 只在读不到 winget 时出现；winget 读得到时它已经列出全部程序 |

每项有唯一的 `Key`（小写），例如 `winget:git.git`、`arp:machine\x64\<键名>`、`msix:<包名>`、`scoop:7zip`；重复时加 `#n`。列表按名称排序。

## 与清单对照（`Match`）

一项属于清单里的哪些软件：winget ID 相同、scoop 软件名相同，或者某个软件的检测规则匹配它的名称（`RuleName`：注册表里的名称，没有时用显示名称）。

## 读取（`Load`）

`Sources` 里的 winget、scoop、注册表扫描并行读取；读不到的来源写进 `List.Notes`，其余照常列出。

## 测试

`inventory_test.go`：合并、关联和对照的单元测试，以及一次真实的只读读取（本机实测 318 项）。
