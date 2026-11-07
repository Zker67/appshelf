# internal/icons

软件图标。`main.go` 把 `Service` 注册为 MyGo 的 `icon` 协议（Windows 上地址是 `http://icon.localhost/...`），前端只用 Go 给出的路径显示图标，不直接加载外部网址。路径格式见 [API 与接口说明](../../docs/api.md#图标协议)。

## 图标从哪来

| 文件 | 内容 |
|---|---|
| `database.go` | UniGetUI 公开的图标库 `screenshot-database-v2.json`（约 12000 项，其中约 6000 项有图标）。键名：winget ID 去掉发布者、转小写，`_ . / ,` 换成 `-`；scoop 用软件名（去掉 `.app`）；GitHub 用 `owner/repo` |
| [icons.go](./icons.go) | 按路径找图标：已装软件优先读取本机，winget 缺本机图标时再用图标库；GitHub 其次用 owner 的头像 |
| [local.go](./local.go) | 注册表 DisplayIcon、MSI ProductIcon、按软件名选择的主 EXE、明确的图标或唯一可用 EXE；应用商店的包 Logo，以及按明确软件名匹配的开始菜单图标；合并相同文件的提取和软件包扫描，本机 Shell 图标绘制为 96 像素 |
| [logos.go](./logos.go) | 从 AppModel 注册表取得包路径，解析 XML 中的 Square44x44Logo 和 Properties.Logo，支持 scale/targetsize 资源；稀疏包回退到 WindowsApps 下的清单与资源，不启动 PowerShell |
| [registry_windows.go](./registry_windows.go)、[registry_other.go](./registry_other.go)、[identity.go](./identity.go) | Windows 的 MSI 产品代码与 ProductIcon、AppModel 包路径；来自注册表的明确 winget 便携包 ID 仅用于查找图标，不改变安装来源 |
| `fetch.go` | 下载：只允许公网的 http(s) 地址（不含 localhost 和内网地址），8 秒超时（含排队），最多 2 MB，最多 6 个并发，同一地址同时只下载一次；按内容识别格式，只接受 PNG、JPEG、GIF、WebP、SVG、ICO |

## 缓存

都在 `%LocalAppData%\AppShelf\icons\`：

- `database.json` 和 `database.meta.json`（ETag、检查时间）：每周检查一次更新，失败后 10 分钟再试，最多 32 MB，下载最多等待 10 秒；本机图标不等待图标库。
- 下载的图标按地址的 SHA-256 命名；下载失败写 `.fail`，2 分钟后可重试，避免临时断网锁住一整天。
- 本机成功图标保存在 `local/`，磁盘缓存最多复用 24 小时；文件路径、大小、修改时间作为缓存键，文件变化自动失效。内存最多 2000 项，成功结果 10 分钟，提取失败 30 秒。
- 开始菜单名称匹配去掉版本后要求一致；多个不同入口同名时不选取。软件目录先检查同名主 EXE，再尝试有限目录扫描，排除卸载、更新和 console 辅助程序。
- 开始菜单和软件包清单启动时后台预热，所有图标共享一次扫描；10 分钟内复用，失败 30 秒后重试，本机列表刷新不再清空扫描缓存。

## 响应

只接受 GET 和 HEAD；找不到图标返回 404，前端显示首字母图标，并最多自动重试 3 次；刷新本机列表也会更新图标 URL，重新尝试缺失图标。SVG 带上禁止脚本的 CSP。

## 测试

`icons_test.go`：键名规则、DisplayIcon 解析、格式识别、地址检查、按图标库提供图标、下载限制、同一地址只下载一次、图标库缓存与 ETag、本机图标的单元测试，以及一次真实下载图标库。

[local_test.go](./local_test.go) 覆盖本机图标不依赖网络、并发去重、重启复用与文件变化失效、共享软件包扫描与取消等待、安装目录和包 Logo 回退、临时失败恢复及下载排队超时。真实只读测试读取本机 Appx Logo，不运行安装。

[identity_test.go](./identity_test.go) 覆盖 MSI 产品代码、XML 命名空间与 Logo 优先级、多 EXE 和大目录里的主程序选择、开始菜单名称回退及便携包 ID；真实测试逐项请求本机软件包图标，并解码验证 PNG。

前端的图标状态按 URL 标识保存，Activity 隐藏恢复时不重置为 loading；恢复 Effect 时核对现有图片的 complete 和 naturalWidth。切换页面命中缓存无需重新等待加载事件，组件回归见 [前端测试](../../frontend/tests/README.md)。
