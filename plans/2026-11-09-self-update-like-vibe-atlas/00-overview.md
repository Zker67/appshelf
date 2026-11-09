# 自身更新改成 Vibe Atlas 的方式

状态：`[in-progress]`

## 目标

用户要求 AppShelf 自身的检查更新和更新方式与 Vibe Atlas（`Zker67/vibe-atlas` 的 `updates.go`、`update-toast.tsx`）一致：

- 检查方式可以设置：启动时检查（默认）、定时检查（每小时 / 每 6 小时 / 每天）、不自动检查。
- 发现新版本时窗口右下角提示「发现新版本 x.y.z」，带「去更新」「稍后」。
- 「去更新」在后台下载、校验签名并替换，提示里显示进度，完成后自动重新打开；没有单独的更新窗口。
- 失败时提示原因，可「重试」或「去 GitHub」下载安装包。
- 设置里的说明文字收进悬停提示，不占版面。

## 做法

| 位置 | 内容 |
|---|---|
| `internal/api/selfupdate.go` | 状态机（idle / checking / available / downloading / restarting / failed）、`watchSelf` 按设置定时检查、`About.SelfUpdate` / `CheckForUpdates` / `InstallUpdate`、`selfUpdate` 事件、失败原因的中文说明 |
| `internal/api/settings.go`、`updates.go`、`dto_updates.go` | `selfCheck`、`selfEvery` 两项设置，取代原来 updater 插件的“每天检查”“自动下载” |
| `main.go` | 去掉 MyGo 的 updater 插件；`SelfCheck` 用 `mygo.Updater.Check`，`Relaunch` 用 `mygo.App.Relaunch`；取文件仍走 `selfupdate.go` 的 `githubFeed` |
| `frontend/src/lib/self-update.ts`、`components/self-update-toast.tsx` | 状态、「稍后」、右下角提示（独立的 Toaster，不与其他提示混在一起） |
| `components/dialogs/settings-dialog.tsx` | 「关于」：版本、状态、检查更新 / 去更新 / 重试、所有版本、自动检查新版本；「更新」页去掉“软件图书馆自身”；各行说明改为 `Hint` 悬停显示 |
| `pages/updates/index.tsx` | 「软件图书馆有新版本」卡片改为就地「去更新」并显示进度 |

## 验收标准

- `go vet`、`go tool mygo vet`、`CGO_ENABLED=0 go test ./...`、前端 `check` 和 `test`、`go tool mygo build` 通过；`selfupdate_test.go` 覆盖检查、进度、失败重试、自动重新打开、检查方式设置和启动时 / 不自动的调度。
- 发布下一版后，用户从这一版更新：右下角提示、点「去更新」后的进度与自动重新打开是否顺畅。

## 说明

- 已装的 0.5.0 及更早版本仍用旧的更新窗口升级到这一版；之后的升级才是新方式。
- 回滚：恢复 `main.go` 的 updater 插件和 Hooks，删除 `internal/api/selfupdate.go` 与前端两个新文件即可；设置文件里多出的 `selfCheck`、`selfEvery` 会被旧版本忽略。
