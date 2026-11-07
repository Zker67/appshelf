// Which page shows, and what one page asks of another: searching for a
// program's source, showing apps in the catalog.

import { createContext, use, useCallback, useMemo, useState, type ReactNode } from "react";

export type Page = "shelf" | "discover" | "installed" | "updates";

/** A program installed outside any source, whose source is searched for. */
export interface FindSource {
  /** name is the program's name, as the registry has it. */
  name: string;
  /** rule finds the program: added with the source found, it is not
   * installed again. */
  rule: string;
}

export interface DiscoverIntent {
  query: string;
  find?: FindSource;
  /** github searches GitHub at once. */
  github?: boolean;
  seq: number;
}

interface Nav {
  page: Page;
  go: (page: Page) => void;
  discoverIntent: DiscoverIntent | null;
  /** search opens Discover and searches. */
  search: (query: string, opts?: { find?: FindSource; github?: boolean }) => void;
  shelfFocus: { ids: string[]; seq: number } | null;
  /** showInShelf opens the catalog on apps. */
  showInShelf: (ids: string[]) => void;
}

const NavContext = createContext<Nav | null>(null);

let seq = 0;

export function NavProvider({ children }: { children: ReactNode }) {
  const [page, setPage] = useState<Page>("installed");
  const [discoverIntent, setDiscoverIntent] = useState<DiscoverIntent | null>(null);
  const [shelfFocus, setShelfFocus] = useState<Nav["shelfFocus"]>(null);
  const search = useCallback((query: string, opts?: { find?: FindSource; github?: boolean }) => {
    setDiscoverIntent({ query, find: opts?.find, github: opts?.github, seq: ++seq });
    setPage("discover");
  }, []);
  const showInShelf = useCallback((ids: string[]) => {
    setShelfFocus({ ids, seq: ++seq });
    setPage("shelf");
  }, []);
  const value = useMemo<Nav>(
    () => ({ page, go: setPage, discoverIntent, search, shelfFocus, showInShelf }),
    [page, discoverIntent, search, shelfFocus, showInShelf],
  );
  return <NavContext value={value}>{children}</NavContext>;
}

export function useNav(): Nav {
  const nav = use(NavContext);
  if (!nav) throw new Error("useNav outside NavProvider");
  return nav;
}
