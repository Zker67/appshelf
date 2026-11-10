import { describe, expect, it } from "vite-plus/test";
import { formatElapsed, lineKind, newWaits, queueProgress, taskProgress } from "./tasks";
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

  it("shows the speed with the bytes", () => {
    expect(taskProgress({ done: 12 << 20, total: 50 << 20, speed: 3.2 * (1 << 20) }).text).toBe(
      "12.0 MB / 50.0 MB · 3.2 MB/s",
    );
    expect(taskProgress({ done: 2048, speed: 1024 }).text).toBe("已下载 2.0 KB · 1.0 KB/s");
    expect(taskProgress({ done: 50, total: 200, speed: 0 }).text).toBe("50 B / 200 B");
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

  it("does not count a removal the user canceled as failed", () => {
    expect(queueProgress([{ state: "failed", canceled: true }, { state: "failed" }])).toEqual({
      done: 2,
      total: 2,
      failed: 1,
      active: false,
    });
  });
});

describe("newWaits", () => {
  it("tells each run waiting for the user once", () => {
    const told = new Set<string>();
    const waiting = { id: 1, started: "a", state: "running", wait: "user" } as const;
    const working = { id: 2, started: "a", state: "running", wait: "working" } as const;
    expect(newWaits([waiting, working], told)).toEqual([waiting]);
    expect(newWaits([waiting, working], told)).toEqual([]);
    // A retry is another run.
    expect(newWaits([{ ...waiting, started: "b" }], told)).toHaveLength(1);
    expect(newWaits([{ ...waiting, id: 3, state: "failed" }], told)).toEqual([]);
  });
});
