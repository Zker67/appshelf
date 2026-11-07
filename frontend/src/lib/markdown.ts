// Editing a note's Markdown as text: what the toolbar and the keys do to the
// text and its selection, ticking a task, and a note's first line.

/** maxNoteLen is how long a note can be, in characters, as in Go. */
export const maxNoteLen = 20000;

/**
 * noteLength counts a note's characters as Go does: code points, so an
 * emoji made of two UTF-16 units is one.
 */
export function noteLength(note: string): number {
  let n = 0;
  for (let i = 0; i < note.length; i++) {
    const c = note.charCodeAt(i);
    if (c < 0xdc00 || c > 0xdfff) n++;
  }
  return n;
}

/** Edit is a text with its selection, as a textarea has them. */
export interface Edit {
  text: string;
  start: number;
  end: number;
}

/** InlineMark is how a span of text is marked. */
export type InlineMark = "bold" | "italic" | "strike" | "code";

const inlineMarks: Record<InlineMark, { mark: string; hint: string }> = {
  bold: { mark: "**", hint: "粗体" },
  italic: { mark: "*", hint: "斜体" },
  strike: { mark: "~~", hint: "删除线" },
  code: { mark: "`", hint: "代码" },
};

/**
 * toggleInline marks the selection, or a hint selected when nothing is, or
 * takes the mark away when the selection already has it around it.
 */
export function toggleInline(e: Edit, kind: InlineMark): Edit {
  const { mark, hint } = inlineMarks[kind];
  const { text, start, end } = e;
  const n = mark.length;
  const before = text.slice(start - n, start);
  const after = text.slice(end, end + n);
  // Italic's "*" is half of bold's "**": "**x**" is not italic.
  const single =
    kind !== "italic" ||
    (text.slice(start - 2, start) !== "**" && text.slice(end, end + 2) !== "**");
  if (before === mark && after === mark && single) {
    return {
      text: text.slice(0, start - n) + text.slice(start, end) + text.slice(end + n),
      start: start - n,
      end: end - n,
    };
  }
  const chosen = text.slice(start, end);
  if (chosen.length > n * 2 && chosen.startsWith(mark) && chosen.endsWith(mark)) {
    const inner = chosen.slice(n, -n);
    return {
      text: text.slice(0, start) + inner + text.slice(end),
      start,
      end: start + inner.length,
    };
  }
  const body = chosen || hint;
  return {
    text: text.slice(0, start) + mark + body + mark + text.slice(end),
    start: start + n,
    end: start + n + body.length,
  };
}

/** LineMark is how a line starts. */
export type LineMark = "heading" | "bullet" | "ordered" | "task" | "quote";

const listPrefix = /^(\s*)([-*+]|\d+[.)])\s+(\[[ xX]\]\s+)?/;
const prefixes: Record<LineMark, RegExp> = {
  heading: /^#{1,6}\s+/,
  bullet: /^(\s*)[-*+]\s+(?!\[[ xX]\]\s)/,
  ordered: /^(\s*)\d+[.)]\s+/,
  task: /^(\s*)[-*+]\s+\[[ xX]\]\s+/,
  // The empty group is the indent the others keep, as "$1".
  quote: /^()>\s?/,
};

/** lineRange widens a selection to the whole lines it touches. */
function lineRange(text: string, start: number, end: number): [number, number] {
  const from = text.lastIndexOf("\n", start - 1) + 1;
  // A selection ending at a line's start does not take that line.
  const stop = end > start && text[end - 1] === "\n" ? end - 1 : end;
  const nl = text.indexOf("\n", stop);
  return [from, nl === -1 ? text.length : nl];
}

/**
 * toggleLines starts the selected lines with a mark, or takes it away when
 * all of them have it. A list mark replaces another; a heading goes one
 * level deeper each time, back to none after ###.
 */
export function toggleLines(e: Edit, kind: LineMark): Edit {
  const [from, to] = lineRange(e.text, e.start, e.end);
  const lines = e.text.slice(from, to).split("\n");
  const has = lines.every((l) => l.trim() === "" || prefixes[kind].test(l));
  let n = 0;
  const out = lines.map((line) => {
    if (line.trim() === "" && lines.length > 1) return line;
    if (kind === "heading") {
      const level = /^(#{1,6})\s+/.exec(line)?.[1]?.length ?? 0;
      const rest = line.replace(prefixes.heading, "");
      // None and # become ##, ## becomes ###, ### and deeper none.
      const next = level >= 3 ? 0 : level === 2 ? 3 : 2;
      return next === 0 ? rest : `${"#".repeat(next)} ${rest}`;
    }
    if (has) return line.replace(prefixes[kind], "$1");
    if (kind === "quote") return `> ${line}`;
    const indent = /^\s*/.exec(line)?.[0] ?? "";
    const rest = line.slice(indent.length).replace(listPrefix, "");
    n++;
    const mark = kind === "bullet" ? "- " : kind === "ordered" ? `${n}. ` : "- [ ] ";
    return indent + mark + rest;
  });
  const block = out.join("\n");
  const text = e.text.slice(0, from) + block + e.text.slice(to);
  if (lines.length === 1) {
    // One line: the cursor keeps its place in the line's text.
    const moved = block.length - (to - from);
    return { text, start: Math.max(from, e.start + moved), end: Math.max(from, e.end + moved) };
  }
  return { text, start: from, end: from + block.length };
}

/** link makes the selection a link, with its address selected to type. */
export function link(e: Edit): Edit {
  const label = e.text.slice(e.start, e.end) || "链接文字";
  const url = /^https?:\/\/\S+$/.test(label) ? label : "https://";
  const text = e.text.slice(0, e.start) + `[${label}](${url})` + e.text.slice(e.end);
  const urlStart = e.start + label.length + 3;
  return { text, start: urlStart, end: urlStart + url.length };
}

/** insertBlock puts a block, as a code block or a rule, on lines of its own. */
export function insertBlock(e: Edit, kind: "code" | "rule"): Edit {
  const { text, start, end } = e;
  const head = text.slice(0, start);
  const tail = text.slice(end);
  const pre = head === "" || head.endsWith("\n\n") ? "" : head.endsWith("\n") ? "\n" : "\n\n";
  const post =
    tail === "" ? "\n" : tail.startsWith("\n\n") ? "" : tail.startsWith("\n") ? "\n" : "\n\n";
  if (kind === "rule") {
    const body = `${pre}---\n${post}`;
    const at = start + body.length;
    return { text: head + body + tail, start: at, end: at };
  }
  const code = text.slice(start, end) || "代码";
  const body = `${pre}\`\`\`\n${code}\n\`\`\`${post}`;
  const at = start + pre.length + 4;
  return { text: head + body + tail, start: at, end: at + code.length };
}

/**
 * continueList is what Enter does at the end of a list item or a quote: a
 * new item marked the same, numbered on, its task not done. On an empty
 * item it ends the list instead. null when Enter is ordinary there.
 */
export function continueList(e: Edit): Edit | null {
  if (e.start !== e.end) return null;
  const { text, start } = e;
  const from = text.lastIndexOf("\n", start - 1) + 1;
  const line = text.slice(from, start);
  const m = /^(\s*)([-*+]|(\d+)([.)]))(\s+)(\[[ xX]\]\s+)?/.exec(line) ?? /^(>\s?)/.exec(line);
  if (!m) return null;
  if (
    line.slice(m[0].length).trim() === "" &&
    (text.slice(start).split("\n")[0] ?? "").trim() === ""
  ) {
    // An empty item: Enter ends the list.
    return { text: text.slice(0, from) + text.slice(start), start: from, end: from };
  }
  let mark: string;
  if (m[0].startsWith(">")) mark = m[0];
  else if (m[3]) mark = `${m[1]}${Number(m[3]) + 1}${m[4]}${m[5]}`;
  else mark = `${m[1]}${m[2]}${m[5]}${m[6] ? "[ ] " : ""}`;
  const insert = `\n${mark}`;
  const at = start + insert.length;
  return { text: text.slice(0, start) + insert + text.slice(start), start: at, end: at };
}

/**
 * indentLines is Tab on list items: two spaces more, or fewer for
 * Shift+Tab. null when the selection is not on list items, for Tab to
 * leave the box as usual.
 */
export function indentLines(e: Edit, outdent: boolean): Edit | null {
  const [from, to] = lineRange(e.text, e.start, e.end);
  const lines = e.text.slice(from, to).split("\n");
  if (!lines.every((l) => listPrefix.test(l))) return null;
  let first = 0;
  let total = 0;
  const out = lines.map((l, i) => {
    let next = l;
    if (outdent) next = l.replace(/^ {1,2}|^\t/, "");
    else next = `  ${l}`;
    const d = next.length - l.length;
    if (i === 0) first = d;
    total += d;
    return next;
  });
  const text = e.text.slice(0, from) + out.join("\n") + e.text.slice(to);
  return {
    text,
    start: Math.max(from, e.start + first),
    end: Math.max(from, e.end + total),
  };
}

/**
 * toggleTask ticks or unticks the task whose list item starts at offset in
 * the note; the note unchanged when there is none.
 */
export function toggleTask(note: string, offset: number): string {
  const m = /^(\s*(?:[-*+]|\d+[.)])\s+\[)([ xX])\]/.exec(note.slice(offset));
  if (!m) return note;
  const at = offset + (m[1] ?? "").length;
  return note.slice(0, at) + (m[2] === " " ? "x" : " ") + note.slice(at + 1);
}

/** noteSummary is the first line of a note with its marks taken away. */
export function noteSummary(note: string | undefined, max = 80): string {
  for (const raw of (note ?? "").split("\n")) {
    const line = raw
      .replace(/^\s*(#{1,6}\s+|>\s?|[-*+]\s+(\[[ xX]\]\s+)?|\d+[.)]\s+)/, "")
      .replace(/!?\[([^\]]*)\]\([^)]*\)/g, "$1")
      .replace(/(\*\*|__|~~|`|\*|_)/g, "")
      .trim();
    if (line === "" || /^(-{3,}|`{3,}.*)$/.test(raw.trim())) continue;
    return line.length > max ? `${line.slice(0, max)}…` : line;
  }
  return "";
}

/** taskCount counts a note's tasks and those done. */
export function taskCount(note: string | undefined): { done: number; total: number } {
  let done = 0;
  let total = 0;
  for (const line of (note ?? "").split("\n")) {
    const m = /^\s*(?:[-*+]|\d+[.)])\s+\[([ xX])\]\s/.exec(line);
    if (!m) continue;
    total++;
    if (m[1] !== " ") done++;
  }
  return { done, total };
}
