# 自更新的取文件方式：避开缓存、自动重试、改走 GitHub API

> 状态：`[in-progress]`：已实现并通过自动检查，随下一个版本发布（与 scoop 进度一起）；再往后一个版本发布时，在用户本机实际检查和更新一次验收
>
> 来源：用户 2026-11-09 要求把 vibe-atlas 的「网络补救」搬过来。vibe-atlas 遇到过本机代理转发 `github.com/.../releases/download/` 持续返回 504、刚发布后仍显示「已经是最新版本」。

## 目标

AppShelf 的更新窗口（MyGo 的 updater 插件）不变，只换它取文件的方式（`mygo.Updater.SetSource`）：

1. 取更新清单时地址带随机参数 `?t=`，请求头 `Cache-Control: no-cache`，避开中间缓存的旧跳转。
2. github.com 6 秒内不开始响应或出错时，改走 GitHub API（`api.github.com/repos/Zker67/appshelf/releases/...`）取同一个文件。
3. 网络问题、超时、429 或 5xx 时自动重试，共 5 次，间隔 1、2、4、8 秒；404、签名不对等不会因重试而好转的错误不重试。

## 不变量

- 更新包照样用 `mygo.json` 的公钥校验签名，取文件方式与签名校验无关。
- 只用 HTTPS；经系统代理（与 MyGo 自己相同的 `netproxy`）。
- 不登录 GitHub：API 每小时 60 次，只在 github.com 失败时才用。

## 实现清单

| 位置 | 内容 |
|---|---|
| `selfupdate.go` | `githubFeed`（`Manifest`、`Open`、`fromAPI`）、`retry`、`transient`；`useUpdateFeed` |
| `main.go` | 注册 updater 插件后调用 `useUpdateFeed()` |
| `selfupdate_test.go` | 仓库与 `mygo.json` 一致；重试的次数和条件；本地 TLS 服务器模拟 github.com 504、慢、恢复，以及 API 备用路；真实读取 GitHub 最新清单（`-short` 跳过，网络不通或 API 额度用完时跳过） |

## 已知限制

- 重试在“打开文件”这一层：更新包下载到一半断开时不会接着下，更新窗口报错，再检查一次重新开始。
- 本机出口 IP 的匿名 API 额度可能已被别人用完（2026-11-09 实测 0/60），这时 API 备用路也会失败。

## 验收标准

- `go vet ./...`、`CGO_ENABLED=0 go test ./...` 通过；`go tool mygo build` 能打包。
- 带这项改动的版本发布后，它及以后的版本检查更新走新方式；再下一个版本发布时，用户本机检查更新能立即看到并装上。

## 回滚说明

删除 `selfupdate.go` 和 `main.go` 里的 `useUpdateFeed()`，更新窗口回到 MyGo 默认的取文件方式。
