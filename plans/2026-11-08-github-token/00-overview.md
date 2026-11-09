# 设置里填写 GitHub token

## 背景

用户反馈：GitHub token 只能设环境变量，设置页只有提示；要在设置里直接填写。

## 目标

- “设置 → 安装来源 → GitHub 请求额度”的 token 行可以填写、更换、清除，显示 token 来自设置、环境变量还是未设置。
- 保存前向 GitHub `/rate_limit` 验证（不占额度），不接受的不保存；保存后立即生效，并重新读取状态。
- 设置里保存的优先于 `GH_TOKEN` / `GITHUB_TOKEN`；清除后回落到环境变量。

## 不变量

- token 不回到前端：前端只能写入和清除，`Overview` 只返回来源 `githubTokenFrom`。
- 不明文落盘：`internal/secret` 用 Windows DPAPI（当前用户 + AppShelf 熵值）加密写入 `%AppData%\AppShelf\github-token.bin`；不进 `settings.json`、不随清单导出、不写日志和错误信息。
- 只接受字母、数字、`_`、`-`，最长 255，先在 Go 端校验再发给 GitHub。
- 不新增依赖：DPAPI 来自已有的 `golang.org/x/sys/windows`。

## 实现

- `internal/secret`：`Save`、`Load`、`Delete`。
- `internal/source/github`：token 加锁可换；`UseToken`（验证并换用、记下新额度）、`SetToken`、`ErrBadToken`。
- `internal/api`：`Config.TokenFile`、`Config.EnvToken`；启动时读取保存的 token；`Sources.SetGitHubToken`、`Sources.ClearGitHubToken`；`SourcesOverview.GitHubTokenFrom`。
- 前端：`components/github-token.tsx` 的 `TokenRow`。

## 验收

- `internal/secret` 真实 DPAPI 往返、文件不含明文；`github` 假服务器与真实 GitHub 拒绝无效 token；`api` 测试校验、拒绝不保存、保存优先、重启读取、清除回落。
- 用户在真实窗口填写自己的 token 验证界面与生效。

## 回滚

回退相关提交；删除 `github-token.bin` 即回到只用环境变量。
