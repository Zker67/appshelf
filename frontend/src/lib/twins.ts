// One app found in both winget and scoop: shown as one result, with the
// source to add chosen on it. Only names that are the same, folded, pair;
// the source added is still the exact package chosen.

import { fold } from "@/lib/text";

interface Named {
  type: string;
  id: string;
  name: string;
}

const keyOf = (h: Named) => `${h.type}:${h.id}`;

/**
 * pairTwins pairs each winget hit with the scoop hit of the same name, by
 * the winget hit's key. A hit pairs once; a name of several hits in one
 * source pairs its first.
 */
export function pairTwins<T extends Named>(winget: T[], scoop: T[]): Map<string, T> {
  const byName = new Map<string, T>();
  for (const s of scoop) {
    const n = fold(s.name);
    if (n && !byName.has(n)) byName.set(n, s);
  }
  const out = new Map<string, T>();
  for (const w of winget) {
    const n = fold(w.name);
    const s = byName.get(n);
    if (!s) continue;
    out.set(keyOf(w), s);
    byName.delete(n);
  }
  return out;
}
