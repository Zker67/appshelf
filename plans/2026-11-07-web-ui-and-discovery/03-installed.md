# 阶段 03：本机已装

状态：`[partial]`：已实现，用户验收与运行验证边界见 [实施报告](./09-implementation-report.md)。

## 目标

新增“本机已装”页，做到 UniGetUI 已装页的核心效果：

- 列出本机全部已装软件，标出来源、版本、发布者和“可更新”；
- 标出哪些已在清单；
- 没在清单的一步加入清单，能识别来源的直接带上精确 ID。

## 数据来源（2026-11-07 本机实测）

| 来源 | 内容 | 本机数量 |
|---|---|---|
| `Get-WinGetPackage` 全量 | 关联到 winget 源的软件（Id 为包 ID，Source 为 `winget`） | 147 |
| 同上 | 没关联到源的注册表软件，Id 为 `ARP\{Machine\|User}\{X64\|X86}\<卸载键名>` | 70 |
| 同上 | Microsoft Store 和系统自带的 MSIX 应用，Id 为 `MSIX\<包全名>` | 100 |
| 同上 | msstore 源的软件 | 1 |
| `scoop export` | scoop 安装的软件（不进注册表） | 本机没有 scoop |
| 注册表 Uninstall 三处 | 显示名、版本、发布者、DisplayIcon、安装位置；已经过滤 `SystemComponent=1` 和带 `ParentKeyName` 的项 | 209 |

`GetPackageVersionInfo(<已装版本>)` 能给出 winget 软件的发布者和 ProductCodes，每项约 11 毫秒，147 项约 1.6 秒。

## 数据模型

```go
// InstalledEntry 是本机的一个已装软件。
type InstalledEntry struct {
    Key         string // 稳定的唯一键，例如 "winget:Git.Git"、"arp:Machine/X64/<键名>"、"scoop:7zip"
    Name        string
    Version     string
    Publisher   string
    Kind        string // "winget"、"scoop"、"local"（只在注册表里）、"store"（MSIX 或 msstore）
    WingetID    string // Kind 为 winget 时
    Scoop       string // Kind 为 scoop 时，bucket/name
    Update      string // 有可用更新时的新版本
    ShelfAppIDs []string // 清单里对应的软件
    IconRef     string // 阶段 05 使用
}
```

## 步骤

1. **一次全量查询，两处使用**：把 winget 的状态脚本改为返回全部已装项（Id、Name、InstalledVersion、Source、IsUpdateAvailable；只对可更新的项读取 AvailableVersions）。`core` 从中按 ID（不区分大小写）挑出清单软件的状态，行为与现在一致，现有测试保持通过；全部结果交给新包 `internal/inventory`。
2. **补充 winget 项的发布者**：后台对关联到源的项调用 `GetPackageVersionInfo`，取发布者和 ProductCodes；用 ProductCodes 对上注册表项，拿到 DisplayIcon 和安装位置。这一步不阻塞列表显示，完成后再广播一次更新。
3. **合并**（`internal/inventory`）：
   - `ARP\` 项按“范围、架构、键名”对上注册表项，合成一行，补上发布者和图标路径；
   - 已经被 winget 覆盖的注册表项不重复列出；剩下的注册表项作为“本机”项列出；
   - scoop 软件单独成行；
   - `MSIX\` 和 msstore 项归为“应用商店”。
4. **与清单对照**：winget ID 相同（不区分大小写）、scoop 软件名相同、或清单软件的检测规则命中这一项，都算“已在清单”，并记下是哪个软件。对照结果只用于展示，不参与安装判断。
5. **加入清单**：
   - winget 项、scoop 项：直接加入，来源带精确 ID，名称用已装项的名称；可以多选后批量加入，在弹层里统一设分类和标签；
   - “本机”项和应用商店项没有可用来源：点“查找来源”，用它的名称打开“发现”页搜索（阶段 04）。选中结果加入时，自动用这一项的显示名生成检测规则：转义正则特殊字符，去掉末尾的版本号和架构，例如 `7-Zip 23.01 (x64)` 生成 `^7-Zip`。用户可以修改后再保存。这样即使 winget 没把它关联到这个包，AppShelf 也会认出它已装，不会重复安装。
6. **页面**：
   - 顶部：按名称、发布者过滤；来源筛选（全部、winget、scoop、本机、应用商店）；“只看未加入清单”开关；上次刷新时间和“刷新”按钮。
   - 列表：图标、名称、发布者、版本、来源、状态（已在清单、可更新）、操作（加入清单、查找来源）。
   - 刷新与“我的清单”的“检查更新”共用一次查询。
7. **服务和事件**：`Installed.List()`、`Installed.Add(keys, categoryID, tagIDs)`；事件 `installed:changed`。

## 成功标准

- 单元测试：合并去重（`ARP\` 与注册表对上、winget 覆盖的注册表项不重复）、清单对照（ID、软件名、检测规则三种）、检测规则生成（转义、去版本号和架构）。
- 集成测试（只读，本机真实 winget 和注册表）：列表非空；本机确定已装的 Git 出现，并带 winget ID；至少一个 `ARP\` 项能对上注册表项，拿到发布者。
- 一次刷新只执行一次 `Get-WinGetPackage` 全量查询。

## 失败与回滚

- ProductCodes 对不上注册表项时，winget 项的发布者改从详情缓存（阶段 04）获取，图标只用图标库。
- 全量返回导致状态刷新明显变慢（比现在多 0.5 秒以上）时，改为状态和已装列表分开查询，已装列表只在打开页面时加载。
- 本阶段只新增 `internal/inventory` 和页面，并修改 winget 状态脚本的返回范围；回退时还原这三处。

## 核验结果

（执行时填写）
