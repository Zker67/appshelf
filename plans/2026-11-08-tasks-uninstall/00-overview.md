# 任务进度、软件内日志、读取现代化、卸载与残留清理

## 背景

用户参考 [Bulk Crap Uninstaller](https://github.com/BCUninstaller/Bulk-Crap-Uninstaller)（BCU，commit `30da609`，Apache-2.0），希望 AppShelf 也能卸载软件、本机已装的读取更现代；同时安装要有进度、日志直接在软件内显示，界面随之优化。分析结论：借鉴 BCU 的判断规则（卸载器识别、静默参数、执行后确认、残留打分），不借鉴它的架构和代码，也不引入 .NET；见 [references/external-repos/BCUninstaller__Bulk-Crap-Uninstaller](../../references/external-repos/BCUninstaller__Bulk-Crap-Uninstaller/README.md)。

此前计划把“卸载”列为非目标；本计划由用户明确提出，取代那条限制。

## 阶段

| 阶段 | 文档 | 内容 | 依赖 |
|---|---|---|---|
| 1 | [01-progress-and-logs.md](./01-progress-and-logs.md) | 任务的阶段与进度、软件内实时日志、整体进度、任务面板改版 | — |
| 2 | [02-modern-inventory.md](./02-modern-inventory.md) | 注册表读全字段、先显示注册表结果再补来源、监听注册表与 scoop 目录自动刷新、大小与安装日期 | — |
| 3 | [03-uninstall.md](./03-uninstall.md) | 按来源卸载：winget / scoop / 应用商店 / 注册表卸载命令；预览、队列、执行后确认、交互式重试 | 1、2 |
| 4 | [04-leftovers.md](./04-leftovers.md) | 卸载后查找残留：打分与理由、逐项确认、注册表先备份、文件进回收站 | 3 |

## 总体不变量

- 沿用现有不变量：清单与本机状态分离、逐个执行、执行前后复查、复用来源自身的能力、前端只做展示。
- **卸载不让软件从清单里消失**；卸载只对用户在预览里确认过的程序执行，目标只由 Go 端从自己读到的本机列表里解析，前端不能传入命令或路径。
- **卸载的完成以复查为准**：来源里查不到、注册表项消失，才算成功；卸载程序的退出码只作参考。
- **残留清理永远逐项确认**：只列出、不自动删除；注册表先导出备份，文件和目录移到回收站；受保护的位置（系统目录、已知文件夹本身、其他已装软件的目录、AppShelf 自己）从不列出。
- 进度只显示真实读到的：读不到百分比时显示阶段和已用时间，不编造进度。
- 真实卸载会改动本机：集成测试按环境变量开启，需用户授权并指定测试软件；本计划实现时不运行真实卸载。

## 验收

- 每阶段：`gofmt -l .`、`go vet ./...`、`go tool mygo vet`、`CGO_ENABLED=0 go test ./...`；`go tool mygo generate` 后 `bun run --cwd frontend check`、`bun run --cwd frontend test`；最后 `go tool mygo build`。
- 用户在真实窗口验收界面：任务进度与日志、本机已装分步显示与自动刷新、卸载预览与执行、残留预览与清理。

## 状态

见 [plans/README.md](../README.md) 的索引；实施结果写在 [09-implementation-report.md](./09-implementation-report.md)。
