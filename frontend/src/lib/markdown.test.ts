import { describe, expect, it } from "vite-plus/test";
import {
  continueList,
  indentLines,
  insertBlock,
  link,
  noteLength,
  noteSummary,
  taskCount,
  toggleInline,
  toggleLines,
  toggleTask,
  type Edit,
} from "./markdown";

/** at writes an edit as text with its selection in [ ], or | for a cursor. */
function at(text: string): Edit {
  const cursor = text.indexOf("|");
  if (cursor >= 0) return { text: text.replace("|", ""), start: cursor, end: cursor };
  const start = text.indexOf("[[");
  const end = text.indexOf("]]") - 2;
  return { text: text.replace("[[", "").replace("]]", ""), start, end };
}

function show(e: Edit | null): string {
  if (!e) return "null";
  if (e.start === e.end) return e.text.slice(0, e.start) + "|" + e.text.slice(e.start);
  return `${e.text.slice(0, e.start)}[[${e.text.slice(e.start, e.end)}]]${e.text.slice(e.end)}`;
}

describe("toggleInline", () => {
  it("marks the selection and takes the mark away again", () => {
    const bold = toggleInline(at("用 [[winget]] 装"), "bold");
    expect(show(bold)).toBe("用 **[[winget]]** 装");
    expect(show(toggleInline(bold, "bold"))).toBe("用 [[winget]] 装");
    expect(show(toggleInline(at("[[**粗**]]"), "bold"))).toBe("[[粗]]");
  });
  it("selects a hint when nothing is selected", () => {
    expect(show(toggleInline(at("a|b"), "code"))).toBe("a`[[代码]]`b");
    expect(show(toggleInline(at("|"), "strike"))).toBe("~~[[删除线]]~~");
  });
  it("does not take bold for italic", () => {
    expect(show(toggleInline(at("**[[x]]**"), "italic"))).toBe("***[[x]]***");
  });
});

describe("toggleLines", () => {
  it("turns lines into a list and back", () => {
    const list = toggleLines(at("[[一\n二]]"), "bullet");
    expect(list.text).toBe("- 一\n- 二");
    expect(toggleLines({ ...list, start: 0, end: list.text.length }, "bullet").text).toBe("一\n二");
  });
  it("numbers lines and replaces another list mark", () => {
    expect(toggleLines(at("[[- 一\n- 二\n三]]"), "ordered").text).toBe("1. 一\n2. 二\n3. 三");
    expect(toggleLines(at("[[- 一\n1. 二]]"), "task").text).toBe("- [ ] 一\n- [ ] 二");
  });
  it("keeps tasks apart from bullets", () => {
    expect(toggleLines(at("- [ ] 待|办"), "bullet").text).toBe("- 待办");
    expect(toggleLines(at("- [x] 完|成"), "task").text).toBe("完成");
  });
  it("cycles heading levels and keeps the cursor in the line", () => {
    const h2 = toggleLines(at("标|题"), "heading");
    expect(show(h2)).toBe("## 标|题");
    const h3 = toggleLines(h2, "heading");
    expect(show(h3)).toBe("### 标|题");
    expect(show(toggleLines(h3, "heading"))).toBe("标|题");
  });
  it("quotes lines and unquotes them", () => {
    const q = toggleLines(at("[[a\nb]]"), "quote");
    expect(q.text).toBe("> a\n> b");
    expect(toggleLines(q, "quote").text).toBe("a\nb");
  });
  it("leaves the lines around the selection alone", () => {
    expect(toggleLines(at("前\n[[中]]\n后"), "bullet").text).toBe("前\n- 中\n后");
    // A selection that ends at a line's start leaves that line.
    expect(toggleLines({ text: "a\nb", start: 0, end: 2 }, "bullet").text).toBe("- a\nb");
  });
});

describe("link and blocks", () => {
  it("selects the address to type", () => {
    expect(show(link(at("看 [[主页]]")))).toBe("看 [主页]([[https://]])");
    expect(show(link(at("[[https://a.dev]]")))).toBe("[https://a.dev]([[https://a.dev]])");
  });
  it("puts a code block on lines of its own", () => {
    expect(show(insertBlock(at("说明|"), "code"))).toBe("说明\n\n```\n[[代码]]\n```\n");
    expect(show(insertBlock(at("a\n\n[[x = 1]]\n\nb"), "code"))).toBe(
      "a\n\n```\n[[x = 1]]\n```\n\nb",
    );
  });
  it("inserts a rule", () => {
    expect(show(insertBlock(at("上|"), "rule"))).toBe("上\n\n---\n\n|");
  });
});

describe("continueList", () => {
  it("continues bullets, numbers and tasks", () => {
    expect(show(continueList(at("- 一|")))).toBe("- 一\n- |");
    expect(show(continueList(at("  9. 九|")))).toBe("  9. 九\n  10. |");
    expect(show(continueList(at("- [x] 做完|")))).toBe("- [x] 做完\n- [ ] |");
    expect(show(continueList(at("> 引用|")))).toBe("> 引用\n> |");
  });
  it("ends the list on an empty item", () => {
    expect(show(continueList(at("- 一\n- |")))).toBe("- 一\n|");
  });
  it("is ordinary elsewhere", () => {
    expect(continueList(at("普通|"))).toBeNull();
    expect(continueList(at("- [[一]]"))).toBeNull();
  });
});

describe("indentLines", () => {
  it("indents list items and back", () => {
    const e = indentLines(at("- 一\n- 二|"), false);
    expect(show(e)).toBe("- 一\n  - 二|");
    expect(show(indentLines(e!, true))).toBe("- 一\n- 二|");
  });
  it("leaves other lines to Tab", () => {
    expect(indentLines(at("正文|"), false)).toBeNull();
  });
});

describe("tasks", () => {
  const note = "## 安装后\n- [ ] 登录\n- [x] 关自启\n  1. [ ] 子项";
  it("ticks the task at a list item's offset", () => {
    const login = note.indexOf("- [ ] 登录");
    expect(toggleTask(note, login)).toBe(note.replace("- [ ] 登录", "- [x] 登录"));
    expect(toggleTask(note, note.indexOf("- [x]"))).toBe(note.replace("- [x]", "- [ ]"));
    expect(toggleTask(note, note.indexOf("  1."))).toBe(note.replace("1. [ ]", "1. [x]"));
    expect(toggleTask(note, 0)).toBe(note);
  });
  it("counts tasks", () => {
    expect(taskCount(note)).toEqual({ done: 1, total: 3 });
    expect(taskCount(undefined)).toEqual({ done: 0, total: 0 });
  });
});

describe("noteSummary", () => {
  it("is the first line without marks", () => {
    expect(noteSummary("\n## **激活** 方法\n正文")).toBe("激活 方法");
    expect(noteSummary("---\n- [ ] 看 [文档](https://a.dev)")).toBe("看 文档");
    expect(noteSummary("```\ncode")).toBe("code");
    expect(noteSummary("")).toBe("");
    expect(noteSummary("字".repeat(100), 10)).toBe(`${"字".repeat(10)}…`);
  });
});

describe("noteLength", () => {
  it("counts code points, as Go counts runes", () => {
    expect(noteLength("备注")).toBe(2);
    expect(noteLength("a😀b")).toBe(3);
    expect(noteLength("")).toBe(0);
  });
});
