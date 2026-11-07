// Selecting items of a list: one by one, a range with Shift, all.

/** toggle returns the selection with id added or removed. */
export function toggle(selected: ReadonlySet<string>, id: string, on?: boolean): Set<string> {
  const out = new Set(selected);
  const want = on ?? !out.has(id);
  if (want) out.add(id);
  else out.delete(id);
  return out;
}

/**
 * selectRange adds or removes every item between the anchor and the target,
 * both included, in the order shown: Shift and a click. Without an anchor in
 * the order, only the target changes.
 */
export function selectRange(
  order: readonly string[],
  selected: ReadonlySet<string>,
  anchor: string | null,
  target: string,
  on: boolean,
): Set<string> {
  const out = new Set(selected);
  const a = anchor === null ? -1 : order.indexOf(anchor);
  const b = order.indexOf(target);
  if (b < 0) return out;
  const [from, to] = a < 0 ? [b, b] : a < b ? [a, b] : [b, a];
  for (let i = from; i <= to; i++) {
    const id = order[i];
    if (id === undefined) continue;
    if (on) out.add(id);
    else out.delete(id);
  }
  return out;
}

/**
 * prune keeps of a selection what is still listed, so that acting on it
 * never touches what the user cannot see; it returns the same set when
 * nothing left.
 */
export function prune(
  selected: ReadonlySet<string>,
  listed: Iterable<string>,
): ReadonlySet<string> {
  const keep = new Set(listed);
  let changed = false;
  const out = new Set<string>();
  for (const id of selected) {
    if (keep.has(id)) out.add(id);
    else changed = true;
  }
  return changed ? out : selected;
}
