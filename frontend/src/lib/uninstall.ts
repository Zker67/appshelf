// What the uninstall page weighs a program by: how much room it takes and
// how long ago it was installed, not whether it has an update.

import type { InstalledEntry, InstalledKind, TaskEntry } from "@/mygo";

const MB = 1024 * 1024;
const GB = 1024 * MB;

/** SizeBand is how much room a program takes. */
export type SizeBand = "huge" | "large" | "small" | "unknown";

/** sizeBand puts a size in bytes in its band: 1 GB and more, 100 MB to 1 GB, less. */
export function sizeBand(size: number | undefined): SizeBand {
  if (!size || size <= 0) return "unknown";
  if (size >= GB) return "huge";
  if (size >= 100 * MB) return "large";
  return "small";
}

/** AgeBand is how long ago a program was installed. */
export type AgeBand = "recent" | "year" | "old" | "unknown";

/**
 * daysSince counts the whole days from a date written YYYY-MM-DD to now,
 * null when the date is missing or not one.
 */
export function daysSince(date: string | undefined, now: Date): number | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(date ?? "");
  if (!m) return null;
  const d = new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]));
  if (Number.isNaN(d.getTime())) return null;
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  return Math.max(0, Math.round((today.getTime() - d.getTime()) / 86400000));
}

/** ageBand puts an install date in its band: 30 days, a year, longer. */
export function ageBand(date: string | undefined, now: Date): AgeBand {
  const days = daysSince(date, now);
  if (days === null) return "unknown";
  if (days <= 30) return "recent";
  if (days <= 365) return "year";
  return "old";
}

/** ageText says how long ago a date was, as "3 天前" or "2 年前". */
export function ageText(date: string | undefined, now: Date): string {
  const days = daysSince(date, now);
  if (days === null) return "";
  if (days === 0) return "今天";
  if (days === 1) return "昨天";
  if (days < 30) return `${days} 天前`;
  if (days < 365) return `${Math.floor(days / 30)} 个月前`;
  return `${Math.floor(days / 365)} 年前`;
}

/** RemovalFilter is what the uninstall page shows; its groups combine. */
export interface RemovalFilter {
  kinds: ReadonlySet<InstalledKind>;
  size: SizeBand | "all";
  age: AgeBand | "all";
}

export const allRemoval: RemovalFilter = { kinds: new Set(), size: "all", age: "all" };

export function isAllRemoval(f: RemovalFilter): boolean {
  return f.kinds.size === 0 && f.size === "all" && f.age === "all";
}

/** removalMatches reports whether a program passes every group of the filter. */
export function removalMatches(e: InstalledEntry, f: RemovalFilter, now: Date): boolean {
  if (f.kinds.size > 0 && !f.kinds.has(e.kind)) return false;
  if (f.size !== "all" && sizeBand(e.size) !== f.size) return false;
  if (f.age !== "all" && ageBand(e.installed, now) !== f.age) return false;
  return true;
}

/** RemovalSort is the order of the uninstall page's list. */
export type RemovalSort = "size" | "oldest" | "newest" | "name";

/**
 * sortForRemoval orders programs largest first, oldest or newest first, or
 * by name as Go lists them; those of unknown size or date go last, by name.
 */
export function sortForRemoval(
  entries: readonly InstalledEntry[],
  by: RemovalSort,
): InstalledEntry[] {
  if (by === "name") return [...entries];
  const ranked = entries.map((e, i) => ({ e, i }));
  ranked.sort((a, b) => {
    const known = (e: InstalledEntry) => (by === "size" ? !!e.size && e.size > 0 : !!e.installed);
    const ka = known(a.e);
    const kb = known(b.e);
    if (ka !== kb) return ka ? -1 : 1;
    if (!ka) return a.i - b.i;
    let d = 0;
    if (by === "size") d = (b.e.size ?? 0) - (a.e.size ?? 0);
    else {
      const x = a.e.installed ?? "";
      const y = b.e.installed ?? "";
      d = x === y ? 0 : (x < y ? -1 : 1) * (by === "oldest" ? 1 : -1);
    }
    return d || a.i - b.i;
  });
  return ranked.map(({ e }) => e);
}

/** totalSize adds up the known sizes. */
export function totalSize(entries: readonly InstalledEntry[]): number {
  return entries.reduce((sum, e) => sum + (e.size && e.size > 0 ? e.size : 0), 0);
}

/**
 * sizeShare is how full a program's size bar is, against the largest: on
 * a square-root scale, so that small programs still show.
 */
export function sizeShare(size: number | undefined, largest: number): number {
  if (!size || size <= 0 || largest <= 0) return 0;
  return Math.min(1, Math.max(0.03, Math.sqrt(size / largest)));
}

/** removalTasks are the queue's removals, the latest first. */
export function removalTasks(items: readonly TaskEntry[]): TaskEntry[] {
  return items.filter((t) => t.action === "uninstall").reverse();
}

/** removing lists the keys of the installed list waiting or being removed. */
export function removing(items: readonly TaskEntry[]): Map<string, TaskEntry> {
  const out = new Map<string, TaskEntry>();
  for (const t of items) {
    if (t.action !== "uninstall" || (t.state !== "waiting" && t.state !== "running")) continue;
    out.set(t.appId.replace(/^uninstall:/, ""), t);
  }
  return out;
}
