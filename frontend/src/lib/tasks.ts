// What the tasks show: sizes, progress, the lines of their logs that tell
// what went wrong, and how far the queue has come.

import type { TaskEntry } from "@/mygo";
import { formatBytes } from "./text";

/** formatElapsed writes a duration in seconds as 0:42 or 1:02:03. */
export function formatElapsed(seconds: number): string {
  const s = Math.max(0, Math.floor(seconds));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const pad = (x: number) => String(x).padStart(2, "0");
  return h > 0 ? `${h}:${pad(m)}:${pad(s % 60)}` : `${m}:${pad(s % 60)}`;
}

/** TaskProgress is what a running task's bar shows. */
export interface TaskProgress {
  /**
   * percent is 0–100 when the size or the source's percentage is known,
   * else null: the bar flows rather than make a number up.
   */
  percent: number | null;
  /**
   * text is how far it has come, as "12 MB / 50 MB · 3.2 MB/s" or "45%",
   * or "".
   */
  text: string;
}

export function taskProgress(
  t: Pick<TaskEntry, "done" | "total" | "percent" | "speed">,
): TaskProgress {
  const done = t.done ?? 0;
  const total = t.total ?? 0;
  const speed = (t.speed ?? 0) > 0 ? ` · ${formatBytes(t.speed ?? 0)}/s` : "";
  if (total > 0) {
    const percent = Math.min(100, Math.max(0, (done / total) * 100));
    return { percent, text: `${formatBytes(done)} / ${formatBytes(total)}${speed}` };
  }
  const reported = t.percent ?? 0;
  if (reported > 0) {
    const percent = Math.min(100, reported);
    return { percent, text: `${Math.round(percent)}%` };
  }
  return { percent: null, text: done > 0 ? `已下载 ${formatBytes(done)}${speed}` : "" };
}

/** LineKind is how a line of a log is shown. */
export type LineKind = "appshelf" | "error" | "plain";

const errorPattern =
  /(error|fail(ed|ure)?|exception|denied|not found|cannot|unable|0x8a15[0-9a-f]{4}|退出码|失败|错误|出错|无法|拒绝|找不到)/i;

/** lineKind tells AppShelf's own lines and those that look like errors. */
export function lineKind(line: string): LineKind {
  if (line.startsWith("[AppShelf]")) return errorPattern.test(line) ? "error" : "appshelf";
  return errorPattern.test(line) ? "error" : "plain";
}

/** QueueProgress is how far the tasks listed have come. */
export interface QueueProgress {
  done: number;
  total: number;
  failed: number;
  /** active means a task waits or runs. */
  active: boolean;
}

export function queueProgress(items: readonly Pick<TaskEntry, "state">[]): QueueProgress {
  let done = 0;
  let failed = 0;
  let active = false;
  for (const t of items) {
    if (t.state === "waiting" || t.state === "running") active = true;
    else done++;
    if (t.state === "failed") failed++;
  }
  return { done, total: items.length, failed, active };
}
