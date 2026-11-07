# `.agents/` — AI 项目规则

本目录存放项目级 AI 行为规则。规则用于描述当前项目的长期约束，不替代宿主工具的全局提示词。

## 目录结构

```text
.agents/
├── rules/
│   └── dev.md
└── skills/
    └── mygo-maintenance/   # mygo init 生成，升级 MyGo 时使用
```

## 索引

- [开发规则](./rules/dev.md)
- [MyGo 维护 Skill](./skills/mygo-maintenance/SKILL.md)
- [项目协作入口](../AGENTS.md)

## 规则文件格式

```markdown
---
trigger: always_on
description: "规则的一句话描述"
---

# 规则标题

规则内容...
```

## trigger 取值

| 取值 | 行为 |
|---|---|
| `always_on` | 每次会话自动加载，作为持续约束 |
| `manual` | 仅在用户显式触发时激活 |

建议按主题拆分规则，保持每个文件职责单一。
