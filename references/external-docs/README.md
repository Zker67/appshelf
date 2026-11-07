# 外部文档

> 本目录记录外部官方文档、标准、教程、论文或资料站点。

## 建议结构

重要资料可以按来源建子目录：

```text
references/external-docs/
├── README.md
└── source-slug/
    ├── README.md        # 来源、版本、许可、适用范围
    ├── structure.md     # 上游文档结构或目录摘要
    ├── notes.md         # 与当前项目相关的摘录和判断
    └── snapshots/       # 可选，小段合规摘录或本地生成摘要
```

只是一条轻量链接时，可以先登记在下面的表格；值得长期跟踪时再建子目录。

## 文档索引

| 名称 | 来源 | 版本/日期 | 用途 | 状态 |
|---|---|---|---|---|
| MyGo 文档 | https://mygo.egoist.dev/docs | v0.8.1（2026-11-07） | 桌面框架、原生 UI、打包与自更新 | 使用中 |
| winget 文档 | https://learn.microsoft.com/windows/package-manager/winget/ | 2026-11-07 | 安装参数、退出码 | 参考 |
| Microsoft.WinGet.Client | https://github.com/microsoft/winget-cli/tree/master/src/PowerShell/Help/Microsoft.WinGet.Client | 2026-11-07 | `Get-WinGetPackage` 结构化输出 | 待验证 |
| scoop | https://github.com/ScoopInstaller/Scoop | 2026-11-07 | `scoop export` / `scoop status` | 参考 |

## winget / scoop 集成要点

调研于 2026-11-07，[推断] 标注的结论尚未实测。

- winget CLI 没有 JSON 输出（[#184](https://github.com/microsoft/winget-cli/issues/184) 未关闭，[#2032](https://github.com/microsoft/winget-cli/issues/2032) 建议改用 PowerShell 模块或 COM API）。文本输出有 `…` 截断、表头随系统语言变化、进度字符混入等问题。
- `Get-WinGetPackage` 返回对象，字段有 Id、Name、InstalledVersion、Source、IsUpdateAvailable、AvailableVersions，可以 `ConvertTo-Json`；`-MatchOption Equals` 是精确匹配。Id 以 `ARP\` / `MSIX\` 开头的是没能关联到任何源的本机软件。
- 退出码：`0x8A150014` 未找到，`0x8A150016` 多个匹配，`0x8A15002B` 没有可用更新，`0x8A150061` 已安装。
- 防误装参数：`--id X --exact --source winget`，加 `--no-upgrade`（已装就跳过，不顺手升级）。默认查询是不区分大小写的子串匹配，不能直接用。
- `scoop export` 输出 JSON（buckets 和 apps）；`scoop status -l` 只看本地。[推断] 可以用 `scoop status -l | ConvertTo-Json` 拿结构化数据。
- [推断] scoop 装的软件不进注册表 ARP，必须单独查。兜底读注册表要扫三处：`HKLM\...\Uninstall`、`WOW6432Node` 下对应键、`HKCU\...\Uninstall`，并过滤掉 `SystemComponent=1` 和带 `ParentKeyName` 的条目。
- [推断] GitHub Releases 安装的软件需要清单里填检测规则（注册表 DisplayName 正则或可执行文件路径），再加上自己的安装记录（release id 和 asset digest）。

## 采纳记录

- 自更新使用 MyGo updates，见 [运行与部署说明](../../docs/operations.md#部署)。
