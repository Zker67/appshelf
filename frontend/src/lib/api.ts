// Reading what Go knows, and reading it again when Go says it changed.

import { QueryClient, useQuery } from "@tanstack/react-query";
import { isCallError } from "mygo-runtime";
import { About, Installed, Shelf, Sources, Tasks, Updates, events, type ChangeKind } from "@/mygo";

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      // Go tells when something changed: data is never stale before.
      staleTime: Infinity,
      refetchOnWindowFocus: false,
      retry: 1,
    },
  },
});

/** The query keys of what Go announces changes of. */
export const keys = {
  shelf: ["shelf"],
  tasks: ["tasks"],
  sources: ["sources"],
  installed: ["installed"],
  updates: ["updates"],
  about: ["about"],
} as const satisfies Record<ChangeKind | "about", readonly string[]>;

/** The kinds that change the updates, beside "updates": what they list. */
const changeUpdates: ReadonlySet<ChangeKind> = new Set(["shelf", "tasks", "installed"]);

/** listenForChanges reads again what Go says changed. */
export function listenForChanges(): () => void {
  return events.changed.on((change) => {
    for (const kind of change.kinds) {
      void queryClient.invalidateQueries({ queryKey: [kind] });
    }
    if (change.kinds.some((k) => changeUpdates.has(k)) && !change.kinds.includes("updates")) {
      void queryClient.invalidateQueries({ queryKey: keys.updates, exact: true });
    }
  });
}

export const useShelf = () => useQuery({ queryKey: keys.shelf, queryFn: () => Shelf.snapshot() });
export const useTasks = () => useQuery({ queryKey: keys.tasks, queryFn: () => Tasks.list() });
export const useSources = () =>
  useQuery({ queryKey: keys.sources, queryFn: () => Sources.overview() });
export const useInstalled = () =>
  useQuery({ queryKey: keys.installed, queryFn: () => Installed.list() });
export const useAbout = () => useQuery({ queryKey: keys.about, queryFn: () => About.info() });
export const useUpdates = () => useQuery({ queryKey: keys.updates, queryFn: () => Updates.list() });
export const useUpdateHistory = () =>
  useQuery({ queryKey: [...keys.updates, "history"], queryFn: () => Updates.history() });
export const useUpdateSettings = () =>
  useQuery({ queryKey: [...keys.updates, "settings"], queryFn: () => Updates.settings() });

/** errorText is the message of an error, as Go wrote it. */
export function errorText(err: unknown): string {
  if (isCallError(err)) return err.message;
  if (err instanceof Error) return err.message;
  return String(err);
}

/** iconURL is the address of an icon path of Go's icon scheme. */
export function iconURL(base: string | undefined, path: string): string {
  if (!base || !path) return "";
  return base + path;
}
