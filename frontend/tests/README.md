# 浏览器组件回归

- [图标缓存页面](./icon-cache.html) / [测试组件](./icon-cache.tsx)：复用真实 IconImage，在 React StrictMode 和 Activity 下检查首次解码、隐藏恢复和 URL 切换，不加载业务数据。
- 根目录运行 `bun run --cwd frontend dev`，打开 `http://localhost:6173/tests/icon-cache.html`。
- 首次加载后图片应满足 `complete=true`、`naturalWidth>0`、class 含 `opacity-100`；隐藏再恢复三次仍满足，URL 不变时 DOM 图片节点不应重建；更换 URL 后同样能完成。
- 这是 DOM 状态回归，真实应用窗口的视觉与交互由用户验收。
