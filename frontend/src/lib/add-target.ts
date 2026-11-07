// Where apps found or installed are added: a category and tags chosen
// once, kept in the page's storage and shared by the pages, so that adding
// is one click.

import { useShelf } from "@/lib/api";
import type { ShelfSnapshot } from "@/mygo";
import { useMemo, useSyncExternalStore } from "react";

/** AddTarget is the category ("" for none) and the tags apps are added with. */
export interface AddTarget {
  categoryId: string;
  tagIds: string[];
}

export const noTarget: AddTarget = { categoryId: "", tagIds: [] };

const storageKey = "appshelf.discover.addTarget";

/** parseTarget reads a stored target, ignoring what is not one. */
export function parseTarget(raw: string | null): AddTarget {
  try {
    const v: unknown = JSON.parse(raw ?? "null");
    if (!v || typeof v !== "object") return noTarget;
    const r = v as Record<string, unknown>;
    return {
      categoryId: typeof r.categoryId === "string" ? r.categoryId : "",
      tagIds: Array.isArray(r.tagIds)
        ? r.tagIds.filter((t): t is string => typeof t === "string")
        : [],
    };
  } catch {
    return noTarget;
  }
}

/** liveTarget drops a category or tags no longer in the catalog. */
export function liveTarget(
  t: AddTarget,
  shelf: Pick<ShelfSnapshot, "categories" | "tags"> | undefined,
): AddTarget {
  if (!shelf) return t;
  const cats = new Set(shelf.categories.map((c) => c.id));
  const tags = new Set(shelf.tags.map((x) => x.id));
  return {
    categoryId: cats.has(t.categoryId) ? t.categoryId : "",
    tagIds: t.tagIds.filter((id) => tags.has(id)),
  };
}

/** targetNames are the names of a target's category ("" for none) and tags. */
export function targetNames(
  t: AddTarget,
  shelf: Pick<ShelfSnapshot, "categories" | "tags"> | undefined,
): { category: string; tags: string[] } {
  return {
    category: shelf?.categories.find((c) => c.id === t.categoryId)?.name ?? "",
    tags: t.tagIds
      .map((id) => shelf?.tags.find((x) => x.id === id)?.name)
      .filter((n): n is string => !!n),
  };
}

/** describeTarget names a target for a toast: 分类「开发」 · 标签 常用. */
export function describeTarget(
  t: AddTarget,
  shelf: Pick<ShelfSnapshot, "categories" | "tags"> | undefined,
): string {
  const { category, tags } = targetNames(t, shelf);
  return [
    category ? `分类「${category}」` : "不分类",
    ...(tags.length ? [`标签 ${tags.join("、")}`] : []),
  ].join(" · ");
}

// One target for the pages: a page changing it changes it for the others.
let stored: AddTarget | null = null;
const listeners = new Set<() => void>();

function current(): AddTarget {
  stored ??= parseTarget(localStorage.getItem(storageKey));
  return stored;
}

function subscribe(onChange: () => void): () => void {
  listeners.add(onChange);
  return () => listeners.delete(onChange);
}

function setTarget(t: AddTarget): void {
  stored = t;
  try {
    localStorage.setItem(storageKey, JSON.stringify(t));
  } catch {
    // Not kept: it still holds until the window closes.
  }
  for (const l of listeners) l();
}

/** useAddTarget is the target apps are added to, as the catalog has it now. */
export function useAddTarget(): [AddTarget, (t: AddTarget) => void] {
  const { data: shelf } = useShelf();
  const raw = useSyncExternalStore(subscribe, current, () => noTarget);
  const target = useMemo(() => liveTarget(raw, shelf), [raw, shelf]);
  return [target, setTarget];
}
