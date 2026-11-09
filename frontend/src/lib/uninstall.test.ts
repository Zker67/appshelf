import { describe, expect, it } from "vite-plus/test";
import type { InstalledEntry, TaskEntry } from "@/mygo";
import {
  ageText,
  allRemoval,
  daysSince,
  localDate,
  removalMatches,
  removalTasks,
  removing,
  sizeBand,
  sizeShare,
  sortForRemoval,
  totalSize,
  useBand,
} from "./uninstall";

const MB = 1024 * 1024;
const now = new Date(2026, 10, 9, 15, 30);

function entry(key: string, extra: Partial<InstalledEntry> = {}): InstalledEntry {
  return { key, name: key, kind: "local", source: null, inShelf: [], icon: "", rule: "", ...extra };
}

describe("size and last-opened bands", () => {
  it("puts sizes in their band", () => {
    expect(sizeBand(undefined)).toBe("unknown");
    expect(sizeBand(0)).toBe("unknown");
    expect(sizeBand(5 * MB)).toBe("small");
    expect(sizeBand(100 * MB)).toBe("large");
    expect(sizeBand(1024 * MB)).toBe("huge");
  });

  it("counts days from a registry date", () => {
    expect(daysSince("2026-11-09", now)).toBe(0);
    expect(daysSince("2026-11-06", now)).toBe(3);
    expect(daysSince("", now)).toBeNull();
    expect(daysSince("20261106", now)).toBeNull();
  });

  it("counts calendar days from a time", () => {
    expect(daysSince(new Date(2026, 10, 9, 0, 5).toISOString(), now)).toBe(0);
    expect(daysSince(new Date(2026, 10, 8, 23, 55).toISOString(), now)).toBe(1);
    expect(daysSince("not a time", now)).toBeNull();
    expect(localDate(new Date(2026, 10, 6, 23, 30).toISOString())).toBe("2026-11-06");
    expect(localDate(undefined)).toBe("");
  });

  it("puts dates in their band and says how long ago", () => {
    expect(useBand("2026-10-20T08:00:00Z", now)).toBe("month");
    expect(useBand("2026-07-01T08:00:00Z", now)).toBe("months");
    expect(useBand("2026-03-01T08:00:00Z", now)).toBe("stale");
    expect(useBand(undefined, now)).toBe("never");
    expect(ageText("2026-11-09", now)).toBe("今天");
    expect(ageText("2026-11-08", now)).toBe("昨天");
    expect(ageText("2026-11-01", now)).toBe("8 天前");
    expect(ageText("2026-08-01", now)).toBe("3 个月前");
    expect(ageText("2023-11-01", now)).toBe("3 年前");
    expect(ageText(undefined, now)).toBe("");
  });
});

describe("removalMatches", () => {
  const big = entry("big", { kind: "winget", size: 2048 * MB, lastUsed: "2025-01-01T08:00:00Z" });
  const fresh = entry("fresh", { size: 3 * MB, lastUsed: "2026-11-01T08:00:00Z" });
  it("combines the groups", () => {
    expect(removalMatches(big, allRemoval, now)).toBe(true);
    expect(removalMatches(big, { ...allRemoval, size: "huge", used: "stale" }, now)).toBe(true);
    expect(removalMatches(big, { ...allRemoval, size: "huge", used: "month" }, now)).toBe(false);
    expect(removalMatches(entry("x"), { ...allRemoval, used: "never" }, now)).toBe(true);
    expect(removalMatches(fresh, { ...allRemoval, kinds: new Set(["winget"]) }, now)).toBe(false);
    expect(removalMatches(entry("x"), { ...allRemoval, size: "unknown" }, now)).toBe(true);
  });
});

describe("sortForRemoval", () => {
  const list = [
    entry("a", { size: 10, lastUsed: "2026-01-02T08:00:00Z" }),
    entry("b"),
    entry("c", { size: 300, lastUsed: "2025-05-05T08:00:00+08:00" }),
    entry("d", { size: 10, lastUsed: "2026-03-01T08:00:00Z" }),
  ];
  const keys = (by: Parameters<typeof sortForRemoval>[1]) =>
    sortForRemoval(list, by).map((e) => e.key);
  it("puts the largest, longest unused or latest opened first, the unknown last", () => {
    expect(keys("size")).toEqual(["c", "a", "d", "b"]);
    expect(keys("unused")).toEqual(["c", "a", "d", "b"]);
    expect(keys("recent")).toEqual(["d", "a", "c", "b"]);
    expect(keys("name")).toEqual(["a", "b", "c", "d"]);
  });

  it("adds up and scales the sizes", () => {
    expect(totalSize(list)).toBe(320);
    expect(sizeShare(300, 300)).toBe(1);
    expect(sizeShare(75, 300)).toBe(0.5);
    expect(sizeShare(0, 300)).toBe(0);
    expect(sizeShare(1, 1e9)).toBe(0.03);
  });
});

describe("removal tasks", () => {
  const task = (id: number, appId: string, state: TaskEntry["state"], action = "uninstall") =>
    ({ id, appId, name: appId, icon: "", action, state, hasLog: false }) as TaskEntry;
  const items = [
    task(1, "uninstall:arp:a", "succeeded"),
    task(2, "winget.x", "running", "install"),
    task(3, "uninstall:winget:b", "running"),
    task(4, "uninstall:arp:c", "waiting"),
  ];
  it("lists the removals, latest first, and those not done by key", () => {
    expect(removalTasks(items).map((t) => t.id)).toEqual([4, 3, 1]);
    expect([...removing(items).keys()]).toEqual(["winget:b", "arp:c"]);
  });
});
