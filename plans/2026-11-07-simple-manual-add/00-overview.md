# 手动添加简化与弹窗滚动修复

状态：`[in-progress]`：实现与自动检查完成，等待用户在真实窗口验收。

## 用户诉求

- 发现页的“手动添加”太复杂，要让新手也能一次填对，界面更现代。
- 修复手动添加弹窗内容压住底部按钮的显示问题。

## 实施

1. `components/dialogs/app-editor.tsx` 改为“先找软件、再补信息”：
   - 一个输入框：输入软件名时同时搜索 winget 和 scoop（复用 `Discover.search`），点结果选中；粘贴 GitHub 链接或 `owner/name` 时识别为仓库，点“使用这个仓库”选中。去掉来源切换按钮。
   - 选中后显示软件卡片：图标、可直接改的名称、来源和 ID、“更换”；GitHub 软件自动读取最新版本并显示将安装的文件（去掉“检查最新版本”按钮），检测规则自动采用第一条建议。
   - 分类、标签在选中后出现；安装包匹配、安装参数、检测规则、bucket 地址收进默认收起的“高级设置”，GitHub 缺检测规则时保存会自动展开。
   - 编辑模式：winget、scoop 来源仍锁定；GitHub 可“更换”，只接受 GitHub 链接。
2. 滚动修复：对话框外层只有 `max-h`，`ScrollArea` 的 viewport 用百分比高度拿不到确定高度，内容溢出盖住底栏。改为让 `ScrollArea` 本身是纵向 flex 容器、viewport 作为 `flex-1 min-h-0` 子项收缩。同样写法的 `add-dialog`、`preview-dialog` 一并修复。
3. 使用说明里“手动添加”一句改为新的用法。

## 不变量

- 不改 Go 服务和数据结构；保存仍走 `Shelf.saveApp`，来源仍需精确选择（winget ID、scoop bucket/name、GitHub owner/name），不按名称模糊安装。
- GitHub 软件仍必须有检测规则。

## 验收

- `bun run --cwd frontend check`、`bun run --cwd frontend test` 通过。
- 用户在真实窗口确认：手动添加的搜索选择、GitHub 链接识别与最新版本显示、高级设置折叠、编辑软件、窗口较矮时弹窗内容可滚动且不压住底栏。

## 回滚

改动只在 `frontend/` 和文档，按文件撤回即可。
