# komi-store/komi-store（原 GitHub Store）

- 仓库：https://github.com/komi-store/komi-store（`rainxchzed/Github-Store` 已重定向到这里）
- 访问日期：2026-11-07；commit 未锁定
- 许可证：待核对
- 技术栈：Kotlin Multiplatform，桌面端为 JVM
- 参考范围：GitHub Releases 选包、版本比较、预发布、检查更新、速率限制。只借鉴思路，不复制代码。

## 借鉴点

| 主题 | GitHub Store 的做法 | 关键源码 | AppShelf 结论 |
|---|---|---|---|
| 可安装格式 | Windows 只认 `.msi` / `.exe`，`.msi` 优先 | `core/data/src/jvmMain/.../services/DesktopInstaller.kt` | 参考；是否支持 zip 便携包另行决定 |
| 选包排序 | 扩展名 > 风味（stable > 预发布 > nightly > debug）> 架构（完全匹配 > 通用包 > 兼容回退）> 体积 > 名称 | `core/domain/.../utils/AssetSelector.kt` | 采纳排序思路；清单里允许为每个软件写 asset 匹配规则覆盖默认 |
| 架构识别 | 带词边界的正则：`x86_64\|amd64\|win64\|x64`、`aarch64\|arm64`、`i386..i686\|win32\|x86` | `AssetArchitectureMatcher.kt` | 采纳 |
| 版本比较 | 去掉 `refs/tags/`、`v` 前缀、`+build` 和变体后缀；无法按数字比较时改看发布时间 / asset digest | `VersionMath.kt` | 采纳 |
| 预发布 | 过滤草稿；API 标志或 tag 标记任一为预发布即视为预发布；每个应用可单独开启 | `core/data/.../repository/GithubReleaseWindow.kt` | 采纳 |
| 检查更新 | 按应用级规则解析要追踪的 asset，综合 tag、时间、digest 和跳过版本得出结论；拉取结果为空视为失败，保留旧状态 | `InstalledAppsRepositoryImpl.kt` | 采纳，尤其是“结果为空视为失败” |
| 速率限制 | 读 `X-RateLimit-*` 响应头，额度耗尽时提示；token 走 Device Flow | `RateLimitInterceptor.kt` | 采纳读响应头；token 方案另定，不入库 |
| 本机检测 | 桌面端 `DesktopPackageMonitor.kt` 是空实现，只信自己的安装记录 | — | **不能照搬**：AppShelf 必须自己检测本机已装软件 |
| 应用发现 | 自建后端 `api.github-store.org` | `BackendEndpoints.kt` | 不需要：AppShelf 的软件来自用户自己的清单 |
