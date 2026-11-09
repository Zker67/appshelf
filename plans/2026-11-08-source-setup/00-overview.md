# 来源首次准备：直接安装 winget 模块和 scoop

## 背景

用户反馈：winget 缺少模块时只在「设置 → 安装来源」里能装，要找；希望第一次提示时就能直接安装；scoop 没装时也要能直接装；界面要现代化。

## 目标

1. 启动后来源检查完成，如果 winget 缺 `Microsoft.WinGet.Client`，或本机没有 scoop，弹出一次「准备安装来源」窗口，每个来源一张卡片，可以直接安装；选「以后再说」或装完后不再自动弹出（`settings.json` 的 `sourceSetupSeen`）。
2. scoop 未安装时可以由 AppShelf 安装：调用官方安装器 `irm get.scoop.sh | iex`，装到当前用户，不需要管理员；以管理员身份运行时官方安装器默认拒绝，这时不提供安装，提示原因。
3. 「设置 → 安装来源」中 winget 和 scoop 行都有对应的安装按钮，与首次窗口共用确认与进度逻辑。

## 不变量

- 只安装来源本身（模块 / scoop），不安装任何软件；都需要用户点击确认。
- 安装后重新检查来源并重新读取状态，不猜测结果。
- scoop 安装后，AppShelf 进程的 PATH 不会自动更新：scoop 脚本前导在找不到 `scoop` 时从注册表补上用户和系统 PATH 再找。

## 实现清单

- `internal/source/winget`：`InstallModule` 改为 `Fix`（来源修复的统一入口）。
- `internal/source/scoop`：`Check` 缺 scoop 时 `Fixable`（非管理员）；新增 `Fix`；前导补 PATH。
- `internal/core`：`CanFix(t)`、`Fix(t, done)` 取代 `CanInstallModule` / `InstallModule`。
- `internal/api`：`Sources.Fix(sourceType)` 取代 `InstallModule`；`SourceInfo.Fixable` 只在能修复时为真；`SourcesOverview.SetupSeen` 与 `Sources.DismissSetup()`。
- 前端：`useFixSource` 共用安装逻辑；`SourceSetupDialog` 首次窗口；设置页按钮。
- 文档：`docs/api.md`、`docs/operations.md`、`internal/source/README.md`。

## 验收

- Go 检查、前端 check / test、`go tool mygo build` 通过。
- 用户在真实窗口确认：首次弹窗出现一次、可直接安装、关闭后不再弹出；设置页两个按钮可用。真实安装 scoop / 模块会修改本机，由用户自己点击执行。

## 回滚

回退本计划涉及的提交即可；`settings.json` 多出的 `sourceSetupSeen` 字段旧版本会忽略。
