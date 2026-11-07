# internal/discover

“发现”页用到的三件事：搜索结果排序、软件详情、GitHub 软件的检测规则建议。全部只用于展示和帮助填写，不参与安装判断。

| 文件 | 内容 |
|---|---|
| `rank.go` | `Fold` 把文字规范成小写、去掉空格和标点；`Rank` 按名称排序：完全相同的在前，其次是以搜索词开头的，其余保持来源给出的顺序 |
| `details.go` | `Details.Get(ref)`：读一个软件包的详情（简介、发布者、主页、许可证、版本、标签、更新说明，GitHub 另有星标、是否归档、最新 release 和会装的文件） |
| `suggest.go` | `SuggestRules(repo, entries)`：为 GitHub 仓库提出检测规则 |

## 软件详情

| 来源 | 读取方式 | 缓存 |
|---|---|---|
| winget | 先用 `LatestVersion` 得到版本，再读 winget-pkgs 仓库 `manifests/<ID 首字母小写>/<ID 按点拆开的各段>/<版本>/` 下的 YAML：单文件清单直接读；多文件清单同时读默认语言和 `zh-CN`，有中文时用中文的简介、介绍和标签 | 7 天 |
| scoop | `scoop info <bucket>/<name>` | 1 天 |
| GitHub | 仓库信息加最新 release，release 用与安装相同的选包规则说明会装哪个文件 | 1 小时；读不到 release 时不缓存 |

缓存在内存和 `%LocalAppData%\AppShelf\details\` 各一份，键为来源、ID 和版本。

## 检测规则建议

依次给出：

1. 本机已装程序里名称像这个仓库的（仓库名去掉 `-releases`、`-desktop`、`-app`、`-windows` 等后缀后，按单词开头匹配），或发布者就是仓库 owner 的，最多 5 条；规则由 `detect.SuggestRule` 从程序名生成，版本号部分换成通配。
2. 最后一条按仓库名生成（`detect.SuggestRuleFromRepo`），用于还没装过的软件。

## 测试

`discover_test.go`：文字规范、排序、清单路径与解析、winget 和 GitHub 详情、缓存过期、规则建议的单元测试，以及一次真实读取 VS Code 的 winget 详情。
