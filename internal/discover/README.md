# internal/discover

“发现”页用到的三件事：搜索结果排序、软件详情、GitHub 软件的检测规则建议。全部只用于展示和帮助填写，不参与安装判断。

| 文件 | 内容 |
|---|---|
| `rank.go` | `Fold` 把文字规范成小写、去掉空格和标点；`Rank` 按名称排序：完全相同的在前，其次是以搜索词开头的，其余保持来源给出的顺序 |
| `details.go` | `Details.Get(ref)`：读一个软件包的详情（简介、发布者、主页、许可证、版本、标签、更新说明，GitHub 另有星标、是否归档、最新 release 和会装的文件） |
| `notes.go` | `Details.ReleaseNotes(ref, from)`：一个软件包从已装版本到目标版本之间每一版的更新日志；`RepoOf(link)` 从链接认出 GitHub 仓库 |
| `suggest.go` | `SuggestRules(repo, entries)`：为 GitHub 仓库提出检测规则 |

## 软件详情

| 来源 | 读取方式 | 缓存 |
|---|---|---|
| winget | 先用 `LatestVersion` 得到版本，再读 winget-pkgs 仓库 `manifests/<ID 首字母小写>/<ID 按点拆开的各段>/<版本>/` 下的 YAML：单文件清单直接读；多文件清单同时读默认语言和 `zh-CN`，有中文时用中文的简介、介绍和标签 | 7 天 |
| scoop | `scoop info <bucket>/<name>` | 1 天 |
| GitHub | 仓库信息加最新 release，release 用与安装相同的选包规则说明会装哪个文件 | 1 小时；读不到 release 时不缓存 |

缓存在内存和 `%LocalAppData%\AppShelf\details\` 各一份，键为来源、ID 和版本。

## 更新日志

| 来源 | 读取方式 |
|---|---|
| GitHub | 仓库最近 30 个 release（不含草稿）：列出已装版本之后、目标版本及以前的每一版，最新的在前，最多 10 版；中间的预发布版跳过，除非目标就是它。不知道已装版本或无法比较时只列目标版本 |
| winget | 读目标版本的清单。更新说明链接、主页或安装包下载地址（必要时再读 `.installer.yaml`）指向 GitHub 仓库时按 GitHub 读取；否则用清单的 `ReleaseNotes` 正文（有中文用中文）；都没有时只给 `ReleaseNotesUrl` 链接 |
| scoop | 主页是 GitHub 仓库时按 GitHub 读取，否则说明没有更新日志 |

tag 先按原样和版本号比较（`1.2` 等于 `1.2.0`，`1.2.2-rc.1` 不等于 `1.2.2`），对不上再忽略数字之间的单词比较（`v2.56.0.windows.4` 对应 `2.56.0.4`，带 rc、beta 等预发布字样的不参与）；仍然对不上时不显示别的版本的日志，只给发行页链接和原因。结果在内存里保留 1 小时；读 GitHub 出错时不保留，额度恢复后再打开即可重读。

## 检测规则建议

依次给出：

1. 本机已装程序里名称像这个仓库的（仓库名去掉 `-releases`、`-desktop`、`-app`、`-windows` 等后缀后，按单词开头匹配），或发布者就是仓库 owner 的，最多 5 条；规则由 `detect.SuggestRule` 从程序名生成，版本号部分换成通配。
2. 最后一条按仓库名生成（`detect.SuggestRuleFromRepo`），用于还没装过的软件。

## 测试

`discover_test.go`：文字规范、排序、清单路径与解析、winget 和 GitHub 详情、缓存过期、规则建议的单元测试，以及一次真实读取 VS Code 的 winget 详情。

`notes_test.go`：版本区间的选择（预发布、超过 10 版、tag 带前缀或单词）、仓库链接识别、winget 正文与 GitHub 回退、GitHub 出错不缓存、scoop 的单元测试，以及一次真实读取 Git、VS Code、ripgrep 的更新日志（只读）。
