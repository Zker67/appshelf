# 阶段 06：GitHub Releases 来源

状态：`[done]`（真实安装测试默认不运行，等待用户指定测试软件）

## 目标

只有 winget 和 scoop 里都找不到的软件，才登记为 GitHub 来源。规则参考 GitHub Store（见 [借鉴记录](../../references/external-repos/komi-store__komi-store/README.md)），但本机检测必须自己做。

## 获取 release

- 调用 `GET /repos/{owner}/{repo}/releases/latest`。它只返回正式版，不包括草稿和预发布。
- 有 `GITHUB_TOKEN` 或 `GH_TOKEN` 时带上 token。
- 读取 `X-RateLimit-Remaining` 和 `X-RateLimit-Reset`。额度用完时，状态记为“未知：GitHub 请求额度用完”，并显示恢复时间；设置页显示剩余额度。
- 请求失败或返回空结果时，视为查询失败，状态记为“未知”，不当作“没有 release”。
- 批量查询时最多同时发 4 个请求。

## 选择安装包

1. 只考虑 `.msi` 和 `.exe`。
2. 清单里填写了 `assetPattern` 时，只保留匹配的安装包。
3. 按以下顺序排序：
   - 扩展名：`.msi` 优先于 `.exe`。
   - 排除文件名里带 debug、symbols 的安装包。
   - 架构：与本机完全匹配优先，其次是文件名里没写架构的，amd64 机器最后才考虑 x86。架构识别用带词边界的正则：`x86_64|amd64|win64|x64`、`aarch64|arm64`、`i[3-6]86|win32|x86`。
4. **排在第一的不止一个、无法区分时，状态记为“未知：无法确定安装包，请填写匹配规则”，不安装。**

编辑对话框里提供“检查”按钮：读取最新 release，显示将要选中的安装包，让用户确认后再保存。

## 版本与状态

- tag 规范化：去掉 `refs/tags/`、开头的 `v`、`+build` 后缀，然后按数字段比较。
- 已装判断：用必填的 `detect` 规则，扫描注册表或检查可执行文件路径。
- 已装版本：优先取注册表的 DisplayVersion，其次取 `state.json` 里的安装记录。
- 可更新：最新 tag 比已装版本新。两者无法比较时，与安装记录的 tag 和 digest 比较；仍无法判断时，显示“已安装（无法比较版本）”，不提示更新。

## 安装与更新

1. 下载安装包到 `App.Path(mygo.PathTemp)` 下的 `AppShelf` 子目录，核对文件大小。asset 带有 `digest` 时，同时核对 SHA-256，不一致就删除文件并记为失败。
2. 运行安装包：
   - `.msi`：`msiexec /i <文件> /norestart`，后面加上 `installerArgs`。没有填写参数时用 `/passive`，实施时先确认 `/passive` 在需要提权的 msi 上能正常弹出 UAC。
   - `.exe`：直接运行，加上 `installerArgs`。没有填写参数时以交互方式运行，由用户在安装程序里操作。
3. 等待进程退出。退出码 `0` 视为成功，`3010` 视为成功但需要重启（在任务页注明），其他值记为失败。
4. 删除下载的文件，不保留安装包。
5. 用 `detect` 复查。确认已装后写入 `state.json` 的安装记录（tag、文件名、digest、时间）；没有检测到就记为失败。

更新的流程与安装相同，使用新版本的安装包。

## 成功标准

- 单元测试：tag 规范化和版本比较；选包排序和“无法区分时拒绝”的情况，用真实仓库的安装包文件名作为样例。
- 集成测试（只读，真实 GitHub API）：读取一个公开仓库的最新 release，并选出安装包；额度信息能被正确解析。
- 安装集成测试：在 `APPSHELF_INSTALL_TEST=1` 下进行，使用用户指定的 GitHub 软件，默认不运行。

## 失败与回滚

本阶段只新增 `internal/source/github`，以及编辑对话框里 GitHub 部分的界面；去掉后不影响 winget 和 scoop。
