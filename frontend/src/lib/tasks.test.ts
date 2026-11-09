import { describe, expect, it } from "vite-plus/test";
import { formatElapsed, lineKind, queueProgress, taskProgress } from "./tasks";
import { formatBytes } from "./text";

describe("formatBytes", () => {
  it("writes sizes in the unit that fits", () => {
    expect(formatBytes(0)).toBe("0 B");
    expect(formatBytes(512)).toBe("512 B");
    expect(formatBytes(1536)).toBe("1.5 KB");
    expect(formatBytes(12.3 * 1024 * 1024)).toBe("12.3 MB");
    expect(formatBytes(150 * 1024 * 1024)).toBe("150 MB");
    expect(formatBytes(-1)).toBe("0 B");
  });
});

describe("formatElapsed", () => {
  it("writes minutes and seconds, hours when long", () => {
    expect(formatElapsed(0)).toBe("0:00");
    expect(formatElapsed(42.9)).toBe("0:42");
    expect(formatElapsed(62)).toBe("1:02");
    expect(formatElapsed(3723)).toBe("1:02:03");
  });
});

describe("taskProgress", () => {
  it("has a percent only with a known size", () => {
    expect(taskProgress({ done: 50, total: 200 })).toEqual({ percent: 25, text: "50 B / 200 B" });
    expect(taskProgress({ done: 2048, total: 0 })).toEqual({
      percent: null,
      text: "已下载 2.0 KB",
    });
    expect(taskProgress({})).toEqual({ percent: null, text: "" });
    expect(taskProgress({ done: 300, total: 200 }).percent).toBe(100);
  });

  it("takes the source's percentage without bytes, never making one up", () => {
    expect(taskProgress({ percent: 42.4 })).toEqual({ percent: 42.4, text: "42%" });
    expect(taskProgress({ percent: 0 })).toEqual({ percent: null, text: "" });
    expect(taskProgress({ percent: 130 }).percent).toBe(100);
    // Bytes with a size come first.
    expect(taskProgress({ done: 50, total: 200, percent: 90 }).percent).toBe(25);
  });
});

describe("lineKind", () => {
  it("tells errors and AppShelf's lines", () => {
    expect(lineKind("[AppShelf] 之后的状态：已安装 2")).toBe("appshelf");
    expect(lineKind("[AppShelf] 安装失败：winget 退出码 0x8A150011")).toBe("error");
    expect(lineKind("安装程序哈希不匹配，0x8a150011")).toBe("error");
    expect(lineKind("Installer failed with exit code: 1603")).toBe("error");
    expect(lineKind("正在下载 https://example.com/a.exe")).toBe("plain");
  });
});

describe("queueProgress", () => {
  it("counts the tasks that ended", () => {
    expect(
      queueProgress([
        { state: "succeeded" },
        { state: "failed" },
        { state: "running" },
        { state: "waiting" },
        { state: "skipped" },
      ]),
    ).toEqual({ done: 3, total: 5, failed: 1, active: true });
    expect(queueProgress([])).toEqual({ done: 0, total: 0, failed: 0, active: false });
  });
});
