// Words and filters of updates: how a package is updated, why an update is
// held back, and the groups the installed programs are filtered by.

import type { InstalledEntry, InstalledKind, UpdateHold, UpdateMode } from "@/mygo";

export const modeLabels: Record<UpdateMode, string> = {
  notify: "提醒我",
  auto: "自动更新",
  ignore: "忽略",
};

/** formatDay writes a date as "11月14日". Invalid or missing dates are "". */
export function formatDay(value: string | Date | undefined | null): string {
  if (!value) return "";
  const d = typeof value === "string" ? new Date(value) : value;
  if (Number.isNaN(d.getTime()) || d.getFullYear() < 2000) return "";
  return `${d.getMonth() + 1}月${d.getDate()}日`;
}

/** holdText says why an update is held back, as "已跳过 1.9". */
export function holdText(
  hold: UpdateHold | undefined,
  opts: { skip?: string; until?: string; mode?: UpdateMode } = {},
): string {
  switch (hold) {
    case "ignore":
      return "已忽略更新";
    case "skip":
      return opts.skip ? `已跳过 ${opts.skip}` : "已跳过这个版本";
    case "snooze": {
      const day = formatDay(opts.until);
      return day ? `暂缓到 ${day}` : "暂缓提醒";
    }
  }
  return opts.mode === "ignore" ? "已忽略更新" : "";
}

/** The update groups of the installed programs. */
export type UpdateGroup = "all" | "update" | "held" | "auto";
/** The catalog groups of the installed programs. */
export type ShelfGroup = "all" | "inShelf" | "notInShelf";

/**
 * InstalledFilter is what the installed programs are filtered by: their
 * sources (none selected is every source), an update group and a catalog
 * group. The groups combine: "可更新" and "未加入清单" are the programs
 * with an update that the catalog does not have.
 */
export interface InstalledFilter {
  kinds: ReadonlySet<InstalledKind>;
  update: UpdateGroup;
  shelf: ShelfGroup;
}

export const allInstalled: InstalledFilter = { kinds: new Set(), update: "all", shelf: "all" };

/** The kinds of the programs no source AppShelf uses knows. */
export const noSourceKinds: ReadonlySet<InstalledKind> = new Set<InstalledKind>(["local", "store"]);

/** inUpdateGroup reports whether a program is in an update group. */
export function inUpdateGroup(e: InstalledEntry, g: UpdateGroup): boolean {
  switch (g) {
    case "update":
      return !!e.update && !e.hold;
    case "held":
      return (!!e.update && !!e.hold) || e.mode === "ignore";
    case "auto":
      return e.mode === "auto";
  }
  return true;
}

/** inShelfGroup reports whether a program is in a catalog group. */
export function inShelfGroup(e: InstalledEntry, g: ShelfGroup): boolean {
  switch (g) {
    case "inShelf":
      return e.inShelf.length > 0;
    case "notInShelf":
      return e.inShelf.length === 0;
  }
  return true;
}

/** installedMatches reports whether a program passes the filter's groups. */
export function installedMatches(e: InstalledEntry, f: InstalledFilter): boolean {
  return (
    (f.kinds.size === 0 || f.kinds.has(e.kind)) &&
    inUpdateGroup(e, f.update) &&
    inShelfGroup(e, f.shelf)
  );
}

/** isAllInstalled reports whether the filter lets every program through. */
export function isAllInstalled(f: InstalledFilter): boolean {
  return f.kinds.size === 0 && f.update === "all" && f.shelf === "all";
}
