// The page follows the appearance Go gives the window: the system's, or
// the light or dark one chosen (About.setTheme), through
// prefers-color-scheme.

import { useSyncExternalStore } from "react";

const query = "(prefers-color-scheme: dark)";

function media(): MediaQueryList | null {
  return typeof window === "undefined" ? null : window.matchMedia(query);
}

/** syncTheme puts the dark class on the page while it is dark. */
export function syncTheme(): void {
  const mq = media();
  if (!mq) return;
  const apply = () => document.documentElement.classList.toggle("dark", mq.matches);
  apply();
  mq.addEventListener("change", apply);
}

function subscribe(onChange: () => void): () => void {
  const mq = media();
  mq?.addEventListener("change", onChange);
  return () => mq?.removeEventListener("change", onChange);
}

/** useDark reports whether the page is dark. */
export function useDark(): boolean {
  return useSyncExternalStore(
    subscribe,
    () => media()?.matches ?? false,
    () => false,
  );
}
