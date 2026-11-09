# internal/tasklog

一个任务（安装、更新、卸载）的日志：原始输出照原样写进日志文件，同时整理成行留在内存里，供任务面板实时显示；来源报告的进度也记在这里。

## 用法

| 方法 | 作用 |
|---|---|
| `New(file, changed)` | 写到 `file`（可为 nil）；每次有新输出或进度时调用 `changed` |
| `Write(p)` | 原样写进文件；按行整理：`\r` 表示这一行重新绘制（`\r\n` 仍是换行），去掉 ANSI 转义和控制字符，制表符换成空格，去掉行尾空格，丢掉只有 spinner 或进度条字符的行，连续空行只留一个；超过 16 KiB 没有换行的输出切成一行 |
| `Lines(from)` | 从第 `from` 行起的行、实际起点 `Start`（更早的行被丢弃时大于 `from`）、下次读取的游标 `Next`，以及还在刷新的当前行 `Live` |
| `ReportProgress(p)` / `Progress()` | 实现 `source.ProgressReporter`：来源用 `source.ReportProgress(log, p)` 报告阶段和下载字节 |
| `Close()` | 结束当前行并关闭文件；之后的输出只留在内存 |

内存里最多保留最后 2000 行（`MaxLines`），完整内容在日志文件里。

## 谁用

`internal/queue` 为每个要执行的任务创建一个 `Log`，交给来源写输出；`Queue.Log(id, from)` 和 `Queue.Items()` 读出行和进度，经 `internal/api` 的 `Tasks.Log`、`Tasks.List` 给前端。

## 测试

`tasklog_test.go`：`\r` 覆盖（含分几次写入）、CRLF、ANSI、spinner、空行合并、长行切分、环形截断与游标、关闭后的写入、进度。
