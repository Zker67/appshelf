// The window: the title bar with the pages, and the page shown. A page
// stays mounted once shown, keeping its search, filters and scroll.

import { Titlebar } from "@/components/titlebar";
import { listenForChanges, useShelf } from "@/lib/api";
import { DiscoverPage } from "@/pages/discover";
import { InstalledPage } from "@/pages/installed";
import { ShelfPage } from "@/pages/shelf";
import { ActionsProvider } from "@/state/actions";
import { NavProvider, useNav, type Page } from "@/state/nav";
import { Activity, useEffect, useRef, useState, type ReactNode } from "react";
import { toast } from "sonner";

export default function App() {
  useEffect(() => listenForChanges(), []);
  return (
    <NavProvider>
      <ActionsProvider>
        <div className="flex h-full flex-col">
          <Titlebar />
          <main className="relative min-h-0 flex-1">
            <PageSlot page="shelf">
              <ShelfPage />
            </PageSlot>
            <PageSlot page="discover">
              <DiscoverPage />
            </PageSlot>
            <PageSlot page="installed">
              <InstalledPage />
            </PageSlot>
          </main>
        </div>
        <OpeningNotes />
      </ActionsProvider>
    </NavProvider>
  );
}

/**
 * PageSlot shows a page while it is chosen. A page mounts the first time it
 * is chosen, so that what it reads and its icons wait until then.
 */
function PageSlot({ page, children }: { page: Page; children: ReactNode }) {
  const nav = useNav();
  const [visited, setVisited] = useState(nav.page === page);
  if (nav.page === page && !visited) setVisited(true);
  if (!visited) return null;
  return (
    <Activity mode={nav.page === page ? "visible" : "hidden"}>
      <div className="anim-fade absolute inset-0">{children}</div>
    </Activity>
  );
}

/** OpeningNotes says once what went wrong opening, as a file not read. */
function OpeningNotes() {
  const { data } = useShelf();
  const said = useRef(new Set<string>());
  useEffect(() => {
    for (const note of data?.notes ?? []) {
      if (said.current.has(note)) continue;
      said.current.add(note);
      toast.warning(note, { duration: 15000 });
    }
  }, [data?.notes]);
  return null;
}
