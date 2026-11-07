# internal/icons

软件图标。`main.go` 把 `Service` 注册为 MyGo 的 `icon` 协议（Windows 上地址是 `http://icon.localhost/...`），前端只用 Go 给出的路径显示图标，不直接加载外部网址。路径格式见 [API 与接口说明](../../docs/api.md#图标协议)。

## 图标从哪来

| 文件 | 内容 |
|---|---|
| `database.go` | UniGetUI 公开的图标库 `screenshot-database-v2.json`（约 12000 项，其中约 6000 项有图标）。键名：winget ID 去掉发布者、转小写，`_ . / ,` 换成 `-`；scoop 用软件名（去掉 `.app`）；GitHub 用 `owner/repo` |
| `icons.go` | 按路径找图标：图标库优先；winget 软件其次用注册表的 DisplayIcon；GitHub 其次用 owner 的头像；应用商店应用用开始菜单里应用的图标；本机文件图标由 `mygo.Shell.FileIcon` 绘制（96 像素） |
| `fetch.go` | 下载：只允许公网的 http(s) 地址（不含 localhost 和内网地址），8 秒超时，最多 2 MB，最多 6 个并发，同一地址同时只下载一次；按内容识别格式，只接受 PNG、JPEG、GIF、WebP、SVG、ICO |

## 缓存

都在 `%LocalAppData%\AppShelf\icons\`：

- `database.json` 和 `database.meta.json`（ETag、检查时间）：每周检查一次更新，失败后 10 分钟再试，最多 32 MB。
- 下载的图标按地址的 SHA-256 命名；下载失败写 `.fail`，24 小时内不再请求。
- 本机文件图标只在内存中缓存，最多 2000 个。

## 响应

只接受 GET 和 HEAD；找不到图标返回 404，前端显示首字母图标；SVG 带上禁止脚本的 CSP。

## 测试

`icons_test.go`：键名规则、DisplayIcon 解析、格式识别、地址检查、按图标库提供图标、下载限制、同一地址只下载一次、图标库缓存与 ETag、本机图标的单元测试，以及一次真实下载图标库。
