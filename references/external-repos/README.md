# 外部仓库

> 本目录记录可参考的外部代码库、参考实现和对比分析。

## 建议结构

重要仓库可以按 `owner__repo` 建子目录：

```text
references/external-repos/
├── README.md
└── owner__repo/
    ├── README.md        # 仓库 URL、commit、license、为什么参考
    ├── structure.md     # 上游目录结构和关键模块
    ├── comparison.md    # 与当前项目的差异和可借鉴点
    └── patches/         # 可选，仅放补丁说明，不放 vendored 仓库
```

## 仓库索引

| 仓库 | commit/release | license | 参考范围 | 状态 |
|---|---|---|---|---|
| [Devolutions/UniGetUI](./Devolutions__UniGetUI/README.md) | 未锁定（2026-11-07） | MIT | 已装检测、检查更新、忽略更新、操作队列 | 已调研 |
| [komi-store/komi-store](./komi-store__komi-store/README.md)（原 GitHub Store） | 未锁定（2026-11-07） | 待核对 | GitHub Releases 选包、版本比较、检查更新 | 已调研 |
| [crenspire/glass-ui](./crenspire__glass-ui/README.md) | `e74f533`（2026-10-14） | README 写 MIT，无 LICENSE 文件 | 玻璃材质、渐变细边、光影层次 | 借鉴思路 |
| [BCUninstaller/Bulk-Crap-Uninstaller](./BCUninstaller__Bulk-Crap-Uninstaller/README.md) | `30da609`（2026-09-08） | Apache-2.0 | 卸载器识别、静默参数、执行后确认、残留打分 | 借鉴规则 |

## 使用边界

- 先记录结构、接口和设计取舍，再决定是否借鉴。
- 复制代码前必须确认许可证、来源标注和维护成本。
- 当前项目的最终实现事实应写入 `docs/` 或源码，不以外部仓库说明为准。
