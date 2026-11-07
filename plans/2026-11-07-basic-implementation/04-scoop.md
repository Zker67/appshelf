# 阶段 04：scoop 来源

状态：`[partial]`（本机没有 scoop，只读集成测试跳过，真实运行未验证）

## 目标

按阶段 01 的来源接口接入 scoop。界面和队列不改。

## 步骤 1：技术核验（内部）

| 核验项 | 预期 |
|---|---|
| `scoop export` | 输出 JSON，含 `buckets` 和 `apps`（Name、Version、Source 等） |
| `scoop status` 转 JSON | 是对象输出，可以 `ConvertTo-Json`，字段有 Name、Installed Version、Latest Version |
| `scoop search <q>` 转 JSON | 返回 Name、Version、Source |
| 失败时的退出码 | 记录实测结果；不可靠时完全以复查结果为准 |

核验结果写在本文件末尾。

## 步骤 2：scoop 适配器（`internal/source/scoop`）

| 方法 | 实现 |
|---|---|
| `Check` | PowerShell 中能找到 `scoop` 命令 |
| `Installed` | `scoop export` 得到已装列表；`scoop status` 得到可更新列表（会联网更新 bucket）。按 `bucket/name` 精确匹配清单 |
| `Recheck` | 再执行一次 `scoop export`，查找对应的 `bucket/name` |
| `Search` | `scoop search <q>` |
| `Install` | bucket 未添加时先添加：在 `scoop bucket known` 列表里的用 `scoop bucket add <bucket>`，否则用清单里的 `bucketUrl`，两者都没有就记为失败。然后执行 `scoop install <bucket>/<name>` |
| `Upgrade` | `scoop update <name>` |

- scoop 调用共用一把全局锁。
- 预览里，需要先添加 bucket 的项注明“将添加 bucket：extras”。
- 清单里的软件用 scoop 装，但本机已经用别的方式装过（例如系统安装版）时，靠 `detect` 规则判断为“已安装（来源外）”，不再装 scoop 版本。

## 成功标准

- 集成测试（只读，真实 scoop，没有 scoop 时跳过）：`Check`、`Installed`、`Search`、对不存在的 app 执行 `Recheck` 返回“未安装”。
- 安装集成测试：在 `APPSHELF_INSTALL_TEST=1` 下进行，默认不运行。
- 没有 scoop 的机器上，scoop 软件显示“不可用：未安装 scoop”，不影响 winget 软件。

## 失败与回滚

`scoop status` 无法转成结构化数据时，检查更新改为对比 `scoop export` 的已装版本和 `scoop info <bucket>/<name>` 的最新版本；仍不可行时回写计划。

## 核验结果

本机没有安装 scoop，以下按 scoop 源码核对，**没有实际运行**：

| 核验项 | 结果 |
|---|---|
| `scoop export` | 输出 JSON：`apps` 每项有 Name、Version、Source、Updated、Info，另有 `buckets`。较旧的 scoop 输出文本，无法解析时提示先运行 `scoop update` |
| `scoop status` | 输出对象，字段有 Name、Installed Version、Latest Version、Info，可以转 JSON。`-l` 只看本地 bucket、不联网，所以批量查询先执行 `scoop update` 更新 scoop 和 bucket，再执行 `scoop status -l`；单项复查不联网 |
| `scoop search <q>` | 输出对象（Name、Version、Source）。查询词按正则表达式处理，所以先转义 |
| 失败时的退出码 | 未实测。实现上只把退出码用作失败提示，成功与否以执行后的复查为准 |
| 其他 | scoop 会向错误流写内容，脚本在调用 scoop 前把 `$ErrorActionPreference` 设为 `Continue`，避免被当成异常中断 |

实施中的调整：

- 已装检测按软件名匹配所有 bucket（scoop 同名软件只能装一个），从别的 bucket 装的也算已装，并注明“通过 X 安装”，不再按 `bucket/name` 精确匹配。
- 搜索不加锁，其余 scoop 调用共用一把锁。

验证方法：在装有 scoop 的机器上运行 `CGO_ENABLED=0 go test ./internal/source/scoop -v`，只读测试会自动执行；再按 [07-acceptance.md](./07-acceptance.md) 的场景 4 和 14 实测。
