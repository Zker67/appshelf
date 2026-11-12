# internal/leftover

查找卸载后留下的东西：安装目录、数据目录、当前用户的注册表设置、开始菜单和桌面的快捷方式、开机启动项。每一项都按“支持”和“反对”的理由打分，由用户勾选后才清理。这个包自己从不清理：`Clean` 只处理调用方传入的项，文件和目录移到回收站，注册表先导出再删除。

打分方式参考 Bulk Crap Uninstaller 的 ConfidenceRecords（Apache-2.0），见 [references/external-repos/BCUninstaller__Bulk-Crap-Uninstaller](../../references/external-repos/BCUninstaller__Bulk-Crap-Uninstaller/README.md)。

## 接口

| 名称 | 作用 |
|---|---|
| `Program` | 已卸载的程序：名称、发布者、安装目录、scoop 名称 |
| `Other` | 仍在本机的程序（名称、发布者、安装目录）：它们用到的东西不算残留 |
| `DefaultConfig(protected...)` | 默认查找位置：Program Files（两个）、ProgramData、AppData、LocalAppData、`LocalAppData\Programs`；`HKCU\Software`；两个开始菜单和两个桌面；`HKCU\…\CurrentVersion\Run`；scoop 的 `persist`。`protected` 是永不清理的目录（AppShelf 自己的） |
| `Find(ctx, cfg, p, others)` | 只读查找，返回按分数从高到低排的 `Item`：类型、路径（注册表项是 `HKEY_CURRENT_USER` 下的路径）、值名、分数、等级、理由 |
| `Clean(ctx, cfg, items, others, own, backupDir)` | 清理传入的项，每项返回 `Outcome`（成功、错误、注册表备份文件）；清理前逐项重新检查 |
| `Protected(cfg, path, others, own)`、`RegPathOK(cfg, item)` | 文件和注册表位置能否清理，`Find` 和 `Clean` 都用它们 |

## 空间展示专用查找

`DataConfig` / `FindData` 用于仍在本机的软件，仅按应用标识、安装元数据名称、发布者与子目录识别数据归属；不调用残留内容评分，不因 DLL 或大量文件而隐藏目录。只查数据根目录和明确的应用包 / scoop persist，排除安装目录重叠、其他软件安装目录和保护路径。结果只用于空间展示，不能交给 `Clean`；清理仍只接收 `FindLeftovers` 的服务端令牌。

安装目录位于 `Apps`、`bin`、`current` 或版本目录时，另沿这些程序布局向上最多四层，在数据根目录内查找应用容器；只将其中直接并列、名称明确表示数据的目录（如 `User Data`、`Cache`、`config`）作为候选。容器不能是共享或发布者公共目录，不能包含其他软件安装或保护目录，路径各级不得经过链接。未知兄弟目录和旧版程序不算数据，不按软件名称维护白名单，也不搜索自定义磁盘位置。候选仍经过原有重叠保护和空间接口去重；理由显示在详情中。

`data_test.go` 覆盖大目录、DLL、包标识、发布者层级、别名、保护路径与残留评分隔离；`data_container_test.go` 覆盖并列数据、本地化名称、版本布局、深度限制、其他安装及链接保护。

## 找什么

| 类型 | 位置 | 怎么算是它的 |
|---|---|---|
| `dir` | 安装目录 | 还在就列出（安装目录不能是共享目录） |
| `dir` | 各查找根目录的子目录，以及“发布者目录”下的子目录 | 名称与程序相同，或部分相同；发布者目录下只剩它的子目录或已经空了时，发布者目录本身也列出 |
| `dir` | scoop 的 `persist\<名称>` | scoop 卸载的程序 |
| `regkey` | `HKCU\Software` 的子键，以及发布者键下的子键 | 名称相同或部分相同；发布者键下只剩它的设置且自己没有值时，发布者键也列出 |
| `shortcut` | 开始菜单、桌面的 `.lnk` | 指向安装目录，或名称相同且指向的程序已不存在 |
| `dir` | 开始菜单里以程序命名的文件夹 | 里面只剩它的快捷方式，或已经空了 |
| `regvalue` | `HKCU\…\Run` 的值 | 命令在安装目录下，或名称相同且程序已不存在 |

名称比较前去掉版本号、架构（x64、64-bit 等）、标点和大小写差异；发布者去掉 Inc.、Ltd.、有限公司之类的后缀。Microsoft、Google、Adobe、update、cache、data 等很多程序共用的名称不参与比较。

## 打分

| 理由 | 分数 |
|---|---|
| 这是它的安装目录 / 快捷方式或开机启动项指向它的安装目录 / scoop 为它保留的数据 | +4 |
| 目录是空的 / 里面只剩它的快捷方式 | +4 |
| 名称与软件相同 | +2 |
| 在发布者的目录下 / 只剩这个软件的设置 | +2 |
| 指向的程序已不存在 | +2 |
| 名称只是部分相同 | −2 |
| 文件很多（超过 200 个） | −2 |
| 里面还有程序文件（.exe、.dll、.sys、.msi） | −4 |
| 还有其他已装软件用这个名称 / 这个发布者还有其他已装软件 | −4 |

等级：4 分及以上为“很可能是残留”（`high`，默认勾选），1 到 3 分为“可能是残留”（`medium`），0 分及以下为“不确定”（`low`）。目录内容最多读 5000 项。

## 保护

以下位置不会被列出，`Clean` 也会拒绝清理：

- 盘符根目录、相对路径、Windows 目录和所有程序共用的目录（`detect.OwnDirectory` 判断）。
- 查找根目录本身（Program Files、AppData、开始菜单、桌面、scoop `persist` 等）。
- `Config.Protected` 里的目录、它们的上级目录和下级目录。
- 其他已装程序的安装目录、它们的上级目录和下级目录（被卸载程序自己的安装目录除外）。
- 注册表只处理 `HKCU\Software` 下的子键和 Run 键里的值；Run 键本身、包含 Run 键的键、带 `..` 或空段的路径都拒绝。

## 清理

- 文件、目录、快捷方式：`SHFileOperationW` + `FOF_ALLOWUNDO`，移到回收站，可以还原。
- 注册表项和值：先 `reg export HKCU\<路径> <备份目录>\NN-<名称>.reg`，成功后才删除（项用 `RegDeleteTreeW`，值用 `DeleteValue`）；导出失败就不删除。
- 每项清理前重新检查：已经不在的报告“已经不在了”，变成受保护的报告“受保护”，都不算成功。

## 谁用

`internal/api` 的 `Tasks.FindLeftovers` 为卸载成功的任务查找残留，结果留在服务端，前端只拿到令牌和每项的 ID；`Tasks.CleanLeftovers` 按 ID 从这次查找的结果里取项再交给 `Clean`，前端传不了任意路径。注册表备份在 `<数据目录>\backups\leftovers-<时间>-<软件名>`，`Tasks.OpenBackup` 打开它。

## 测试

`leftover_test.go`：

- 名称和发布者的规范化、名称匹配、保护规则、注册表位置检查。
- `TestFindAndClean`（Windows）：在 `%LOCALAPPDATA%\AppShelfTest-*` 下造出一个已卸载程序“Foo App”的安装目录、数据目录、发布者目录、快捷方式，在 `HKCU\Software\AppShelfTest-*` 下造出设置和开机启动项；检查每一项的分数，然后清理其中一部分：选中的进回收站或被删除并留下备份，没选中的和受保护的保留。测试结束删除临时目录和注册表键；被清理的测试文件会留在回收站里。
- `TestScoopPersist`：scoop 的 `persist` 目录。
