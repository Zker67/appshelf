import { describe, expect, it } from "vite-plus/test";
import type { ShelfApp } from "@/mygo";
import { prune, selectRange, toggle } from "./selection";
import {
  countStatus,
  emptyFilter,
  filterApps,
  inGroups,
  isFiltered,
  sortApps,
  sourceID,
  sourceKey,
  statusView,
  type ShelfFilter,
} from "./shelf";
import { fold, formatDateTime, formatStars, hue, initial, matches } from "./text";

function app(
  id: string,
  name: string,
  extra: Partial<ShelfApp["app"]> = {},
  status: ShelfApp["status"] = null,
): ShelfApp {
  return {
    app: { id, name, source: { type: "winget", id: `${name}.${name}` }, ...extra },
    status,
    checking: false,
    pending: false,
    icon: "",
  };
}

const apps = [
  app(
    "a1",
    "Git",
    { categoryId: "c_dev", tagIds: ["t_daily"] },
    { state: "installed", version: "2.4" },
  ),
  app(
    "a2",
    "微信",
    { categoryId: "c_chat" },
    { state: "updateAvailable", version: "3.9", latest: "4.0" },
  ),
  app("a3", "7-Zip", { tagIds: ["t_daily"] }, { state: "notInstalled" }),
  app("a4", "Obsidian", {}, { state: "unknown", note: "离线" }),
  app("a5", "Visual Studio Code", { categoryId: "c_dev" }, null),
];

const filter = (f: Partial<ShelfFilter>): ShelfFilter => ({ ...emptyFilter, ...f });
const ids = (list: ShelfApp[]) => list.map((a) => a.app.id);

describe("text", () => {
  it("folds case, spaces, punctuation and full-width forms", () => {
    expect(fold("Visual Studio Code")).toBe("visualstudiocode");
    expect(fold("visual-studio_code")).toBe("visualstudiocode");
    expect(fold("ＶＳ　Ｃｏｄｅ")).toBe("vscode");
    expect(fold("微信 （WeChat）")).toBe("微信wechat");
  });
  it("matches every word loosely", () => {
    expect(matches("", ["anything"])).toBe(true);
    expect(matches("studio code", ["Visual Studio Code"])).toBe(true);
    expect(matches("VSCODE", ["x", "Microsoft.VS-Code"])).toBe(true);
    expect(matches("code vim", ["Visual Studio Code"])).toBe(false);
    expect(matches("微信", [undefined, "微信"])).toBe(true);
  });
  it("writes times with the date and 24 hours", () => {
    expect(formatDateTime(new Date(2026, 10, 7, 14, 5))).toBe("2026-11-07 14:05");
    expect(formatDateTime(new Date(2026, 0, 2, 9, 3))).toBe("2026-01-02 09:03");
    expect(formatDateTime("")).toBe("");
    expect(formatDateTime("0001-01-01T00:00:00Z")).toBe("");
  });
  it("draws fallback icons from names", () => {
    expect(initial("  7-Zip")).toBe("7");
    expect(initial("obsidian")).toBe("O");
    expect(initial("微信")).toBe("微");
    expect(initial("(beta) tool")).toBe("B");
    expect(hue("Git")).toBe(hue("Git"));
    expect(hue("Git")).toBeGreaterThanOrEqual(190);
    expect(formatStars(52310)).toBe("5.2 万");
    expect(formatStars(980)).toBe("980");
  });
});

describe("shelf", () => {
  it("unites the categories and tags selected", () => {
    expect(ids(filterApps(apps, emptyFilter))).toEqual(["a1", "a2", "a3", "a4", "a5"]);
    expect(ids(filterApps(apps, filter({ categories: new Set(["c_dev"]) })))).toEqual(["a1", "a5"]);
    // A category or a tag: the union.
    expect(
      ids(
        filterApps(apps, filter({ categories: new Set(["c_chat"]), tags: new Set(["t_daily"]) })),
      ),
    ).toEqual(["a1", "a2", "a3"]);
    // "" is the apps without a category.
    expect(ids(filterApps(apps, filter({ categories: new Set([""]) })))).toEqual(["a3", "a4"]);
    expect(inGroups(apps[0]!, filter({ tags: new Set(["t_none"]) }))).toBe(false);
  });
  it("filters by state and query within the groups", () => {
    expect(ids(filterApps(apps, filter({ status: "update" })))).toEqual(["a2"]);
    expect(ids(filterApps(apps, filter({ status: "problem" })))).toEqual(["a4"]);
    expect(ids(filterApps(apps, filter({ query: "studio" })))).toEqual(["a5"]);
    expect(ids(filterApps(apps, filter({ query: "git.git" })))).toEqual(["a1"]);
    expect(
      ids(filterApps(apps, filter({ categories: new Set(["c_dev"]), query: "code" }))),
    ).toEqual(["a5"]);
    expect(isFiltered(emptyFilter)).toBe(false);
    expect(isFiltered(filter({ query: " " }))).toBe(false);
    expect(isFiltered(filter({ status: "installed" }))).toBe(true);
  });
  it("counts and sorts by state", () => {
    expect(countStatus(apps)).toEqual({
      all: 5,
      update: 1,
      installed: 1,
      notInstalled: 1,
      problem: 1,
    });
    expect(ids(sortApps(apps, "status"))).toEqual(["a2", "a3", "a1", "a4", "a5"]);
    expect(ids(sortApps(apps, "added"))).toEqual(ids(apps));
    expect(sortApps(apps, "name")[0]!.app.name).toBe("7-Zip");
  });
  it("words states", () => {
    expect(statusView(null, true).label).toBe("读取中");
    expect(statusView({ state: "updateAvailable", version: "1", latest: "2" }, false)).toEqual({
      label: "可更新",
      tone: "primary",
      detail: "1 → 2",
    });
    expect(
      statusView({ state: "installed", external: true, note: "注册表：Git" }, false).label,
    ).toBe("已安装（来源外）");
    expect(statusView({ state: "unknown", note: "离线" }, false)).toEqual({
      label: "无法确认",
      tone: "warning",
      detail: "离线",
    });
  });
  it("names sources as Go does", () => {
    expect(sourceID({ type: "scoop", bucket: "main", name: "7zip" })).toBe("main/7zip");
    expect(sourceKey({ type: "winget", id: "Git.Git" })).toBe("winget:git.git");
    expect(sourceKey({ type: "github", repo: "Owner/Repo" })).toBe("github:owner/repo");
  });
});

describe("selection", () => {
  const order = ["a", "b", "c", "d", "e"];
  it("toggles one", () => {
    expect([...toggle(new Set(["a"]), "b")]).toEqual(["a", "b"]);
    expect([...toggle(new Set(["a"]), "a")]).toEqual([]);
    expect([...toggle(new Set(["a"]), "a", true)]).toEqual(["a"]);
  });
  it("selects a range either way", () => {
    expect([...selectRange(order, new Set(), "b", "d", true)].sort()).toEqual(["b", "c", "d"]);
    expect([...selectRange(order, new Set(), "d", "b", true)].sort()).toEqual(["b", "c", "d"]);
    expect([...selectRange(order, new Set(order), "b", "d", false)].sort()).toEqual(["a", "e"]);
    expect([...selectRange(order, new Set(), null, "c", true)]).toEqual(["c"]);
    expect([...selectRange(order, new Set(), "gone", "c", true)]).toEqual(["c"]);
  });
  it("keeps only what is listed", () => {
    const sel = new Set(["a", "x"]);
    expect([...prune(sel, order)]).toEqual(["a"]);
    const same = new Set(["a", "b"]);
    expect(prune(same, order)).toBe(same);
  });
});
