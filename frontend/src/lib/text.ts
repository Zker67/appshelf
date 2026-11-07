// Loose text matching and display helpers shared by the pages.

/**
 * fold reduces text for loose matching: full-width characters as their
 * ordinary forms, lower case, without spaces and punctuation, so that
 * "Visual Studio Code", "visual-studio_code" and "ＶＳ Ｃｏｄｅ" compare
 * by their letters only.
 */
export function fold(text: string): string {
  return text
    .normalize("NFKC")
    .toLowerCase()
    .replace(/[\s\p{P}\p{S}]+/gu, "");
}

/**
 * matches reports whether every word of the query appears in one of the
 * fields, compared folded. An empty query matches everything.
 */
export function matches(query: string, fields: Array<string | undefined | null>): boolean {
  const words = query
    .normalize("NFKC")
    .split(/\s+/)
    .map(fold)
    .filter((w) => w !== "");
  if (words.length === 0) return true;
  const folded = fields.filter((f): f is string => !!f).map(fold);
  return words.every((w) => folded.some((f) => f.includes(w)));
}

/** pad2 pads a number to two digits. */
function pad2(n: number): string {
  return n < 10 ? `0${n}` : `${n}`;
}

/**
 * formatDateTime writes a time as "2026-11-07 14:05": the date, then the
 * time on 24 hours. Invalid or missing times are "".
 */
export function formatDateTime(value: string | Date | undefined | null): string {
  if (!value) return "";
  const d = typeof value === "string" ? new Date(value) : value;
  if (Number.isNaN(d.getTime()) || d.getFullYear() < 2000) return "";
  return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())} ${pad2(d.getHours())}:${pad2(d.getMinutes())}`;
}

/** formatClock writes the hour and minute of a time, as "14:05". */
export function formatClock(value: string | Date | undefined | null): string {
  if (!value) return "";
  const d = typeof value === "string" ? new Date(value) : value;
  if (Number.isNaN(d.getTime())) return "";
  return `${pad2(d.getHours())}:${pad2(d.getMinutes())}`;
}

/** formatCount writes a count of things with its unit, as "12 个软件". */
export function formatCount(n: number, unit = "个软件"): string {
  return `${n} ${unit}`;
}

/** formatStars writes a star count compactly, as "1.2 万". */
export function formatStars(n: number): string {
  if (n >= 10000) return `${(n / 10000).toFixed(n >= 100000 ? 0 : 1)} 万`;
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`;
  return `${n}`;
}

/**
 * initial is the letter an icon falls back to: the first letter or digit
 * of a name, upper case, or its first Chinese character.
 */
export function initial(name: string): string {
  const trimmed = name.trim();
  for (const ch of trimmed) {
    if (/[\p{L}\p{N}]/u.test(ch)) return ch.toUpperCase();
  }
  return trimmed.slice(0, 1).toUpperCase() || "?";
}

/** hue picks a stable hue for a name, for its fallback icon. */
export function hue(name: string): number {
  let h = 0;
  for (const ch of name) h = (h * 31 + (ch.codePointAt(0) ?? 0)) >>> 0;
  // Blues and their neighbours, with the app's colors.
  return 190 + (h % 110);
}
