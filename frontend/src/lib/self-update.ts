// AppShelf's own update, as Vibe Atlas has it: Go checks in the background
// and tells each change; a version found shows at the bottom right, and
// 去更新 downloads it in place, AppShelf starting again in it.

import { useQuery } from "@tanstack/react-query";
import { useSyncExternalStore } from "react";
import { toast } from "sonner";
import { errorText, queryClient } from "@/lib/api";
import { About, events, type SelfState } from "@/mygo";

export const selfKey = ["self"] as const;

/** The releases on GitHub: the installers of every version. */
export const RELEASES_URL = "https://github.com/Zker67/appshelf/releases";

/** useSelfUpdate is the state of AppShelf's own update, as Go tells it. */
export const useSelfUpdate = () =>
  useQuery({ queryKey: selfKey, queryFn: () => About.selfUpdate() });

/** listenForSelfUpdate keeps the state as Go sends each change. */
export function listenForSelfUpdate(): () => void {
  return events.selfUpdate.on((s: SelfState) => queryClient.setQueryData(selfKey, s));
}

// The version put off with 稍后, not offered again while AppShelf runs.
let later = "";
const listeners = new Set<() => void>();
const tell = () => listeners.forEach((l) => l());

/** useLater is the version put off, or "". */
export function useLater(): string {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    () => later,
  );
}

/** putOff puts the version found off until AppShelf starts again. */
export function putOff(version: string): void {
  later = version;
  tell();
}

/**
 * checkSelf checks for a newer AppShelf now, as the user asked: a version
 * found shows again even if put off. It returns why it failed, or "".
 */
export async function checkSelf(): Promise<string> {
  later = "";
  tell();
  try {
    queryClient.setQueryData(selfKey, await About.checkForUpdates());
    return "";
  } catch (err) {
    return errorText(err);
  }
}

/** installSelf downloads the newer AppShelf; it starts again in it. */
export function installSelf(): void {
  later = "";
  tell();
  About.installUpdate().catch((err: unknown) =>
    toast.error("无法更新", { description: errorText(err) }),
  );
}

/** openReleases opens the releases on GitHub, to download an installer by hand. */
export function openReleases(latest = false): void {
  About.openURL(latest ? `${RELEASES_URL}/latest` : RELEASES_URL).catch((err: unknown) =>
    toast.error("无法打开链接", { description: errorText(err) }),
  );
}
