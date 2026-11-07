# 项目文档索引

> 本目录承载当前代码和程序的长期文档。`README.md` 和 `AGENTS.md` 只保留入口、摘要和协作约束。

## 一层入口

- [架构索引](./architecture/README.md)
- [单一信息源说明](./architecture/source-of-truth.md)
- [项目结构总览](./architecture/project-structure.md)
- [运行与部署说明](./operations.md)
- [API 与接口说明](./api.md)
- [计划索引](../plans/README.md)
- [外部参考索引](../references/README.md)

## 使用方式

### 新成员或新会话

1. 先看 [项目结构总览](./architecture/project-structure.md)，确认目录职责。
2. 再看 [单一信息源说明](./architecture/source-of-truth.md)，确认同类信息应该维护在哪里。
3. 按任务需要进入具体专题文档、计划或外部参考。

### 需要改代码

1. 先确认目标模块的主维护文档。
2. 改完后只更新对应事实源，其他文档只补入口、摘要或链接。
3. 如果只是临时方案、阶段拆分或验收记录，写入 `plans/`，不要写成长期事实。

## 边界说明

- `README.md`：面向人类的项目总览、快速入口和文档分流。
- `AGENTS.md`：面向 AI 助手的协作入口、约束和项目不变量。
- `docs/`：当前项目长期事实和专题文档。
- `plans/`：计划、设计草案、执行过程和验收记录，不承担当前结构真相。
- `references/`：外部资料索引和借鉴记录，不直接作为当前项目事实源。
