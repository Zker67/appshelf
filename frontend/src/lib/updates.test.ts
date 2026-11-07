import { describe, expect, it } from "vite-plus/test";
import type { InstalledEntry, ShelfApp } from "@/mygo";
import { statusGroup, statusView } from "./shelf";
import {
  allInstalled,
  formatDay,
  holdText,
  installedMatches,
  isAllInstalled,
  noSourceKinds,
  type InstalledFilter,
} from "./updates";

function entry(key: string, extra: Partial<InstalledEntry> = {}): InstalledEntry {
  return {
    key,
    name: key,
    kind: "winget",
    source: null,
    inShelf: [],
    icon: "",
    rule: "",
    ...extra,
  };
}

const git = entry("winget:git.git", { update: "2.56", inShelf: ["a_git"], mode: "notify" });
const code = entry("winget:code", { update: "1.99", mode: "auto" });
const zip = entry("scoop:7zip", { kind: "scoop", update: "25.0", hold: "skip" });
const chat = entry("winget:chat", { mode: "ignore" });
const tool = entry("arp:tool", { kind: "local" });
const store = entry("msix:photos", { kind: "store", inShelf: ["a_photos"] });
const all = [git, code, zip, chat, tool, store];

const keys = (f: InstalledFilter) => all.filter((e) => installedMatches(e, f)).map((e) => e.key);

describe("installed filters", () => {
  it("lets everything through by default", () => {
    expect(keys(allInstalled)).toHaveLength(all.length);
    expect(isAllInstalled(allInstalled)).toBe(true);
  });
  it("splits updates from held ones", () => {
    expect(keys({ ...allInstalled, update: "update" })).toEqual([git.key, code.key]);
    expect(keys({ ...allInstalled, update: "held" })).toEqual([zip.key, chat.key]);
    expect(keys({ ...allInstalled, update: "auto" })).toEqual([code.key]);
  });
  it("combines the update and catalog groups", () => {
    expect(keys({ ...allInstalled, update: "update", shelf: "notInShelf" })).toEqual([code.key]);
    expect(keys({ ...allInstalled, update: "update", shelf: "inShelf" })).toEqual([git.key]);
  });
  it("selects several sources, as those of no source", () => {
    const f = { ...allInstalled, kinds: noSourceKinds };
    expect(keys(f)).toEqual([tool.key, store.key]);
    expect(keys({ ...f, shelf: "inShelf" })).toEqual([store.key]);
    expect(isAllInstalled(f)).toBe(false);
  });
});

describe("hold words", () => {
  it("says why an update waits", () => {
    expect(holdText("skip", { skip: "1.9" })).toBe("已跳过 1.9");
    expect(holdText("snooze", { until: "2026-11-14T08:00:00+08:00" })).toBe("暂缓到 11月14日");
    expect(holdText("ignore")).toBe("已忽略更新");
    expect(holdText(undefined, { mode: "ignore" })).toBe("已忽略更新");
    expect(holdText("")).toBe("");
    expect(formatDay("nonsense")).toBe("");
  });
});

describe("held updates in the catalog", () => {
  it("count as installed, worded as held", () => {
    const held: ShelfApp = {
      app: { id: "a_git", name: "Git", source: { type: "winget", id: "Git.Git" } },
      status: { state: "updateAvailable", version: "2.55", latest: "2.56" },
      checking: false,
      pending: false,
      icon: "",
      hold: "skip",
    };
    expect(statusGroup(held)).toBe("installed");
    expect(statusGroup({ ...held, hold: undefined })).toBe("update");
    expect(statusView(held.status, false, held.hold)).toEqual({
      label: "已跳过 2.56",
      tone: "muted",
      detail: "2.55",
    });
  });
});
