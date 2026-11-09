# crenspire/glass-ui

- 仓库：<https://github.com/crenspire/glass-ui>（站点 <https://glass-ui.crenspire.com>）
- 访问：2026-11-08，`develop` 分支 commit `e74f533689525b60f28aab1c370ee44416dc508f`（2026-10-14）
- 许可证：README 写明 MIT，但仓库没有 LICENSE 文件，GitHub 也识别不出许可证。只借鉴思路，不原样复制代码。
- 关系：「冰川蓝 · 轻玻璃」改版（[计划](../../../plans/2026-11-08-glacier-glass/00-overview.md)）的质感参考。

## 结构

仿 Apple Liquid Glass 的 shadcn registry 组件库（Next.js 16、React 19）。

| 位置 | 内容 |
|---|---|
| `app/globals.css` 的 `.lg` 层 | CSS 材质：`--lg-base` 底色、`--lg-blur` / `--lg-sat` 模糊和饱和度、`--lg-rim-hi` / `--lg-rim-lo` 渐变细边、`--lg-highlight` 内高光、`--lg-drop` 投影 |
| `lib/liquid-glass/material.ts` | 用途（surface、sheet、bar、control、knob、inline、overlay）× 材质（regular、clear、tinted）→ 类名和 WebGL 用的 `data-glass` 属性 |
| `components/ui/*.tsx` | 改过的 shadcn 底层组件，默认 `variant="liquid"`，按钮 `h-11` 胶囊 |
| `components/ui/glass/*.tsx` | 包一层底层组件，加悬停效果 |
| `components/ui/liquid/`、`lib/liquid-glass/engine.ts` | WebGL 折射：一张全屏画布在页面后面，只折射一张背景图，最多 32 个形状 |

## 结论

| 点 | 结论 |
|---|---|
| 渐变细边：`padding-box` 底色 + `border-box` 135° 渐变（两角亮、中间暗） | 采纳，改用 oklch token |
| 内高光 + 近处投影 + 大范围柔和投影三层光影 | 采纳，替换原来的蓝色发光 |
| 三种材质 × 按用途固定圆角和模糊 | 采纳思路，圆角按桌面高密度工具收小（面板 16px、控件 8–10px） |
| `prefers-reduced-transparency` 下接近不透明 | 采纳 |
| `backdrop-filter: url(#lens)` SVG 透镜（只在 Chromium 有效，WebView2 可用） | 试用在少数元素，性能不合格就去掉 |
| 直接 `shadcn add @glass-ui/...` 装组件 | 不采纳：每项都会覆盖 `components/ui/` 的底层组件和全局 CSS；尺寸没有 `xs` / `icon-sm`，依赖是 23 个独立 `@radix-ui/react-*` 加 `framer-motion`、`next-themes`，与本项目的 `radix-ui`、`cn` 重复 |
| WebGL 折射 | 不采纳：只折射固定背景图、不折射页面内容，对以列表为主的界面帮助小，还要多一个 GPU 画布 |
