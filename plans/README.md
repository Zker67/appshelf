# 计划文档索引

> 单一入口：所有实施计划、设计草案、改进追踪和验收状态都在 `plans/` 目录下管理。当前项目长期事实以 `docs/` 为准。

## 何时创建计划

- 新项目启动或需要明确架构边界时。
- 重大重构、跨模块改动或高风险变更前。
- 新功能涉及多个阶段、多个文件或需要验收标准时。

## 与其他文档的边界

- `docs/`：当前代码和程序的长期事实。计划完成后，把稳定结论同步到对应 `docs/` 文档。
- `references/`：外部文档和外部仓库参考。计划可链接参考资料，但不复制外部资料结构。

## 文档结构约定

```text
plans/
├── README.md
├── YYYY-MM-DD-feature-name/
│   ├── 00-overview.md
│   ├── 01-phase-name.md
│   └── 09-improvements.md
└── archive/
```

## 每个阶段文档建议包含

1. 目标：本阶段要达成什么。
2. 不变量：哪些行为或接口不能破坏。
3. 实现清单：文件、接口、数据流和边界条件。
4. 验收标准：如何确认完成。
5. 回滚说明：失败时如何恢复。

## 状态标注

| 标注 | 含义 |
|---|---|
| `[done]` | 已完成并验证 |
| `[in-progress]` | 正在执行或等待验收 |
| `[partial]` | 核心可用但有遗留问题 |
| `[planned]` | 已规划，未执行 |
| `[dropped]` | 已废弃或被替代 |

## 计划列表

| 计划 | 状态 | 说明 |
|---|---|---|
| [2026-11-07 基本实现](./2026-11-07-basic-implementation/00-overview.md) | `[partial]` | 清单、分类和标签、本机检测与防误装、批量安装、检查更新、导入导出、自更新；来源为 winget、scoop、GitHub Releases。已发布 v0.1.0；用户试用后认为界面简陋、加软件不便，界面部分由下一个计划替代；scoop 未实测，见 [实施报告](./2026-11-07-basic-implementation/09-implementation-report.md) |
| [2026-11-07 Web 前端、本机已装与发现软件](./2026-11-07-web-ui-and-discovery/00-overview.md) | `[partial]` | 0.2.0 已实现并通过自动检查；Web 前端、“本机已装”、发现、详情和图标已交付。用户验收、真实安装与更新、scoop 和跨版本自更新待实测，见 [实施报告](./2026-11-07-web-ui-and-discovery/09-implementation-report.md) |
| [本机已装图标优化](./2026-11-07-installed-icons/00-overview.md) | `[partial]` | 本机优先、并发去重、持久缓存、软件包 Logo 回退与有限重试；技术改动已进入 v0.2.1 检查点标签（未发布），后续缓存修复与新功能见软件图书馆计划 |
| [软件图书馆与本机直接更新](./2026-11-07-library-and-installed-updates/00-overview.md) | `[partial]` | 0.3.0 实现与技术验证完成：切页缓存、软件图书馆名称、本机单项/多选更新；真实窗口与实际软件更新待用户验收 |
| [页面布局调整](./2026-11-07-layout-adjustments/00-overview.md) | `[done]` | 顶栏顺序改为本机已装、我的清单、发现；三个页面统一左侧栏，设置移到左下角；去掉原生滚动条 |
| [视觉质感与切换动效](./2026-11-07-visual-polish/00-overview.md) | `[done]` | 取图标的钴蓝渐变与玻璃质感：主按钮、页签、侧栏、菜单和弹窗换装；切页按方向滑入，列表级联入场，概览数字滚动 |
| [手动添加简化与弹窗滚动修复](./2026-11-07-simple-manual-add/00-overview.md) | `[in-progress]` | 手动添加改为一个输入框：搜软件名或粘贴 GitHub 链接，选中即可加入，次要项收进高级设置；修复弹窗内容压住底栏 |
| [一键加入与发现页改进](./2026-11-07-discover-add-target/00-overview.md) | `[in-progress]` | 搜索框旁“加入到”设好分类和标签，发现和本机已装一键加入（可撤销、Enter 加入）；“已在清单 →”按钮；winget 与 scoop 同名结果合并；GitHub 仍弹窗确认检测规则 |
| [详情栏压缩与分页签](./2026-11-07-detail-tabs/00-overview.md) | `[in-progress]` | 详情分「概览」「介绍」（清单另有「安装规则」）页签各自滚动；信息改为 2 列信息格和链接按钮，ID 挪到标题下，往下滚时头部收缩 |
| [发现页搜索提速](./2026-11-07-faster-discover-search/00-overview.md) | `[in-progress]` | winget 搜索不再逐个下载版本信息，改为常驻 PowerShell 取 ID 和名称、`winget search` 补版本、结果缓存与页面预热；宽泛关键词从 30–55 秒降到约 0.2 秒出结果、0.4 秒补齐版本；等待用户在真实窗口验收 |
| [现代化的软件更新](./2026-11-07-modern-updates/00-overview.md) | `[in-progress]` | 本机已装按来源、更新、清单三组组合筛选；按软件设“自动更新 / 提醒我 / 忽略”，跳过某个版本或暂缓 7 天；统一的“更新”页和角标；定期检查、系统通知、托盘与开机后台启动；只对 scoop 和用户范围的 winget 软件自动更新，且程序未在运行 |
| [冰川蓝 · 轻玻璃](./2026-11-08-glacier-glass/00-overview.md) | `[in-progress]` | 视觉方向重做：冰川蓝配色、渐变细边与三层光影的轻玻璃材质、缓慢流动的冰雾背景、浮动玻璃面板布局；借鉴 glass-ui 的 CSS 材质思路，不装它的组件 |
| [来源首次准备](./2026-11-08-source-setup/00-overview.md) | `[in-progress]` | 第一次检测到 winget 缺模块或没有 scoop 时弹出「准备安装来源」直接安装；scoop 用官方安装器装到当前用户；设置页两个来源都有安装按钮；等待用户在真实窗口验收 |
| [设置里填写 GitHub token](./2026-11-08-github-token/00-overview.md) | `[in-progress]` | 设置页直接填写、更换、清除 GitHub token：先向 GitHub 验证，DPAPI 加密保存在本机，立即生效，优先于环境变量；token 不回到前端；等待用户在真实窗口验收 |
| [软件备注](./2026-11-07-app-notes/00-overview.md) | `[in-progress]` | 每个软件的 Markdown 备注存进清单 `apps[].note`：详情「备注」页签（任务可勾选）、编辑 / 分栏 / 预览的编辑弹窗（工具栏、快捷键、自动保存）、行内备注图标、搜索备注、导入合并补备注；等待用户在真实窗口验收 |
| [任务进度、日志、卸载与残留清理](./2026-11-08-tasks-uninstall/00-overview.md) | `[in-progress]` | 四期：任务阶段与进度、软件内实时日志；注册表读全字段、先显示后补全、监听自动刷新；按来源卸载（winget / scoop / 应用商店 / 注册表卸载命令，执行后复查）；卸载后残留打分、逐项确认、注册表备份、文件进回收站；追加独立的“卸载”页签（按大小和安装时间挑选，后由下一个计划改为上次打开时间）。已实现并通过自动检查和本机只读测试，真实卸载和窗口验收待用户完成，见 [实施报告](./2026-11-08-tasks-uninstall/09-implementation-report.md) |
| [卸载页按上次打开时间挑选](./2026-11-09-last-opened/00-overview.md) | `[in-progress]` | 新增 `internal/usage` 读取 UserAssist 启动记录（开始菜单、任务栏、桌面、资源管理器）和正在运行的进程，按目录 / 应用包 / scoop 归到程序；卸载页“安装时间”分组和排序换成“上次打开”；等待用户在真实窗口验收 |
| [修复卸载误报失败](./2026-11-09-uninstall-false-failure/00-overview.md) | `[in-progress]` | winget 卸载后也等 NSIS / Inno Setup 的卸载程序副本结束再复查；重试时程序已不在算卸载成功；跳过的卸载任务立刻移出本机已装列表；等待用户真实卸载验收 |
| [常驻托盘与托盘菜单](./2026-11-09-tray/00-overview.md) | `[in-progress]` | 托盘图标常驻，「关闭窗口后留在托盘」默认打开；右键菜单查看更新、全部更新、检查更新、任务、退出（有任务时确认）；有更新时图标带橙点，悬停显示更新和任务；等待用户在真实窗口验收 |
| [winget 改走 COM 接口显示真实进度](./2026-11-09-winget-com-progress/00-overview.md) | `[in-progress]` | winget 安装、更新、卸载经 `Microsoft.WinGet.Client` 模块在后台作业里调用 COM，读出真实下载百分比；进度模型增加百分比；模块不可用时退回命令行 |
| [scoop 下载进度、下载速度与 aria2](./2026-11-09-scoop-progress-aria2/00-overview.md) | `[in-progress]` | scoop 默认下载器按 `Downloading … (大小)` 和缓存里 `.download` 文件的实际大小算进度，aria2 解析读数行；`tasklog` 按字节变化算下载速度；准备 scoop 时一起安装 aria2，已有 scoop 缺 aria2 时也提示 |
