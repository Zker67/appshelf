# 阶段 05：软件图标

状态：`[partial]`：已实现，用户验收与运行验证边界见 [实施报告](./09-implementation-report.md)。

## 目标

“我的清单”“发现”“本机已装”的每一行都有图标。常见软件显示真实图标，其余用首字母图标兜底，列表不会因为图标而变慢。

## 图标来源（按顺序尝试）

| 对象 | 来源 | 说明 |
|---|---|---|
| winget 软件 | UniGetUI 图标库 | 键名：包 ID 去掉第一段（发布者）、转小写，`_` `.` 空格 `/` `,` 换成 `-`，例如 `Obsidian.Obsidian` → `obsidian`；规则与 UniGetUI 的 `GenerateIconId` 相同 |
| scoop 软件 | UniGetUI 图标库 | 键名：软件名去掉 `.app` 后按同样规则转换 |
| GitHub 软件 | UniGetUI 图标库（键名 `owner/repo`，不区分大小写），否则用 owner 头像 `https://avatars.githubusercontent.com/<owner>` | 头像地址不占 GitHub API 额度 |
| 本机已装项 | 注册表 `DisplayIcon` | `.ico`、`.png` 直接使用；指向 `.exe`、`.dll` 时用 Windows API 提取图标（`ExtractIconExW`）并转成 PNG，提取失败就跳过 |
| 以上都没有 | 首字母图标 | 前端生成：取名称首字（中文取第一个字），底色由名称哈希决定，深浅色模式下都清楚 |

已在清单的软件按来源取图标；已装但不在清单的按本机项取。

## 图标库

- 地址：`https://raw.githubusercontent.com/Devolutions/UniGetUI/main/WebBasedData/screenshot-database-v2.json`（2026-11-07 为 1.5 MB、12423 条）。
- 首次需要时下载，缓存在 `PathCache` 下，每 7 天用 ETag 检查一次更新；下载失败时继续用旧缓存，没有缓存就只用其他来源。
- 只读取 `icons_and_screenshots` 中每项的 `icon`，不使用截图。
- 许可：UniGetUI 仓库是 MIT；图标本身属于各自的权利人。AppShelf 只在运行时下载到本机缓存，不随程序分发。记录到 `references/external-repos/Devolutions__UniGetUI/README.md`。

## 下载与缓存（Go 端）

- 前端不直接加载外部网址。所有图标由 Go 下载、缓存，再通过自定义协议提供：`mygo.Protocol.Handle("icon", …)`。Windows 上页面用 `http://icon.localhost/<类型>/<键>` 访问。
- 下载限制：只接受 `http(s)`；超时 8 秒；最大 2 MB；`Content-Type` 必须是 png、jpeg、gif、webp、svg、ico 之一；最多同时下载 6 个。
- 缓存：以来源网址的哈希作为文件名，存放在 `PathCache\icons\`；下载失败的网址记 1 天，期间不再重试。
- SVG 只在 `<img>` 中使用，返回时加 `Content-Security-Policy: default-src 'none'; style-src 'unsafe-inline'`，避免其中的脚本执行。
- 协议处理器返回 404 时，前端显示首字母图标；图片使用懒加载，只有滚动到可见范围才请求。

## 步骤

1. 新包 `internal/icons`：图标库下载和查询、键名规则、远程图标下载缓存、注册表图标提取、协议处理器。
2. `InstalledEntry`、搜索结果、清单软件都带一个 `IconRef`（类型加键），由 Go 计算，前端拼成协议地址。
3. 前端组件 `AppIcon`：加载中显示占位，失败显示首字母图标；三种列表统一使用。

## 成功标准

- 单元测试：键名规则（winget、scoop、GitHub，含大小写和特殊字符）；下载限制（超大、错误类型、超时）；失败的网址 1 天内不重试。
- 集成测试（只读，真实网络和本机）：图标库能下载并解析，`obsidian` 有图标；本机至少一个已装软件能从注册表提取出 PNG。
- 本机已装页（约 300 行）滚动流畅：首屏只请求可见行的图标。

## 失败与回滚

- 从 `.exe` 提取图标在 Go 中实现成本过高时，只支持 `.ico`、`.png`，其余用首字母图标，并在本文件记录。
- 图标库下载很慢或无法访问时，不阻塞任何页面，图标逐个出现或保持首字母图标。
- 回退时删除 `internal/icons` 和协议注册，前端全部显示首字母图标。

## 核验结果

（执行时填写）
