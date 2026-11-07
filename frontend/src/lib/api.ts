// Reading what Go knows, and reading it again when Go says it changed.

import { QueryClient, useQuery } from "@tanstack/react-query";
import { isCallError } from "mygo-runtime";
import { About, Installed, Shelf, Sources, Tasks, events, type ChangeKind } from "@/mygo";

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
  about: ["about"],
} as const satisfies Record<ChangeKind | "about", readonly string[]>;

/** listenForChanges reads again what Go says changed. */
export function listenForChanges(): () => void {
  return events.changed.on((change) => {
    for (const kind of change.kinds) {
      void queryClient.invalidateQueries({ queryKey: [kind] });
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
