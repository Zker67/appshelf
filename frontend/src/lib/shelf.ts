// What the catalog's apps are shown and filtered by.

import type { AppStatus, ShelfApp, Source } from "@/mygo";
import { matches } from "./text";

export type SourceKind = "winget" | "scoop" | "github";

export const sourceLabels: Record<SourceKind, string> = {
  winget: "winget",
  scoop: "scoop",
  github: "GitHub",
};

/** sourceLabel names a source type. */
export function sourceLabel(type: string): string {
  return sourceLabels[type as SourceKind] ?? type;
}

/** sourceID is a source's exact identity: an ID, bucket/name or owner/name. */
export function sourceID(s: Source): string {
  switch (s.type) {
    case "winget":
      return s.id ?? "";
    case "scoop":
      return `${s.bucket ?? ""}/${s.name ?? ""}`;
    case "github":
      return s.repo ?? "";
  }
  return "";
}

/** iconPath is a source's icon on Go's icon scheme, as Go makes it. */
export function iconPath(s: Source): string {
  const esc = encodeURIComponent;
  switch (s.type) {
    case "winget":
      return s.id ? `/w/${esc(s.id)}` : "";
    case "scoop":
      return s.name ? `/s/${esc(s.name)}` : "";
    case "github": {
      const [owner, repo] = (s.repo ?? "").split("/");
      return owner && repo ? `/g/${esc(owner)}/${esc(repo)}` : "";
    }
  }
  return "";
}

/** sourceKey identifies what a source installs, ignoring case, as in Go. */
export function sourceKey(s: Source): string {
  switch (s.type) {
    case "winget":
      return `winget:${(s.id ?? "").toLowerCase()}`;
    case "scoop":
      return `scoop:${(s.bucket ?? "").toLowerCase()}/${(s.name ?? "").toLowerCase()}`;
    case "github":
      return `github:${(s.repo ?? "").toLowerCase()}`;
  }
  return `${s.type}:`;
}

export type Tone = "primary" | "success" | "warning" | "muted" | "danger";

export interface StatusView {
  label: string;
  tone: Tone;
  /** detail is a version change, or the note of a state. */
  detail?: string;
}

/** statusView words an app's state for the list. */
export function statusView(status: AppStatus | null, checking: boolean): StatusView {
  if (!status) return { label: checking ? "读取中" : "未读取", tone: "muted" };
  switch (status.state) {
    case "updateAvailable":
      return {
        label: "可更新",
        tone: "primary",
        detail: `${status.version || "?"} → ${status.latest ?? ""}`,
      };
    case "installed":
      return status.external
        ? { label: "已安装（来源外）", tone: "success", detail: status.note }
        : { label: "已安装", tone: "success", detail: status.version || undefined };
    case "notInstalled":
      return { label: "未安装", tone: "muted" };
    case "unavailable":
      return { label: "不可用", tone: "danger", detail: status.note };
    default:
      return { label: "无法确认", tone: "warning", detail: status.note };
  }
}

export type StatusFilter = "all" | "update" | "installed" | "notInstalled" | "problem";

/** statusGroup puts an app in one of the status filters. */
export function statusGroup(app: ShelfApp): Exclude<StatusFilter, "all"> | "pending" {
  const s = app.status;
  if (!s) return "pending";
  switch (s.state) {
    case "updateAvailable":
      return "update";
    case "installed":
      return "installed";
    case "notInstalled":
      return "notInstalled";
  }
  return "problem";
}

export interface ShelfFilter {
  /** categories selected; "" is the apps without a category. */
  categories: ReadonlySet<string>;
  tags: ReadonlySet<string>;
  status: StatusFilter;
  query: string;
}

export const emptyFilter: ShelfFilter = {
  categories: new Set(),
  tags: new Set(),
  status: "all",
  query: "",
};

/** hasGroupFilter reports whether categories or tags are selected. */
export function hasGroupFilter(f: ShelfFilter): boolean {
  return f.categories.size > 0 || f.tags.size > 0;
}

/** isFiltered reports whether anything is filtered out. */
export function isFiltered(f: ShelfFilter): boolean {
  return hasGroupFilter(f) || f.status !== "all" || f.query.trim() !== "";
}

/**
 * inGroups reports whether an app is in the categories or tags selected:
 * their union, so selecting "工具" and "常用" shows the apps of either.
 * Nothing selected is every app.
 */
export function inGroups(app: ShelfApp, f: ShelfFilter): boolean {
  if (!hasGroupFilter(f)) return true;
  const cat = app.app.categoryId ?? "";
  if (f.categories.has(cat)) return true;
  return (app.app.tagIds ?? []).some((t) => f.tags.has(t));
}

/** filterApps applies the groups, the state and the query. */
export function filterApps(apps: ShelfApp[], f: ShelfFilter): ShelfApp[] {
  return apps.filter((a) => {
    if (!inGroups(a, f)) return false;
    if (f.status !== "all" && statusGroup(a) !== f.status) return false;
    return matches(f.query, [a.app.name, sourceID(a.app.source)]);
  });
}

export type SortKey = "added" | "name" | "status";

const statusOrder: Record<ReturnType<typeof statusGroup>, number> = {
  update: 0,
  notInstalled: 1,
  installed: 2,
  problem: 3,
  pending: 4,
};

/** sortApps orders apps: as added, by name, or updates first. */
export function sortApps(apps: ShelfApp[], key: SortKey): ShelfApp[] {
  if (key === "added") return apps;
  const byName = (a: ShelfApp, b: ShelfApp) => a.app.name.localeCompare(b.app.name, "zh-CN");
  const out = [...apps];
  if (key === "name") return out.sort(byName);
  return out.sort(
    (a, b) => statusOrder[statusGroup(a)] - statusOrder[statusGroup(b)] || byName(a, b),
  );
}

/** countStatus counts the apps of each state filter. */
export function countStatus(apps: ShelfApp[]): Record<StatusFilter, number> {
  const out: Record<StatusFilter, number> = {
    all: apps.length,
    update: 0,
    installed: 0,
    notInstalled: 0,
    problem: 0,
  };
  for (const a of apps) {
    const g = statusGroup(a);
    if (g !== "pending") out[g]++;
  }
  return out;
}
