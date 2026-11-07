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
          <Pages />
        </div>
        <OpeningNotes />
      </ActionsProvider>
    </NavProvider>
  );
}

/** The pages in the order of their tabs, which says the side a page enters from. */
const order: Page[] = ["installed", "shelf", "discover"];

type Direction = "none" | "left" | "right";

/** Pages holds the pages; the one chosen slides in from the side of its tab. */
function Pages() {
  const nav = useNav();
  const [shown, setShown] = useState(nav.page);
  const [dir, setDir] = useState<Direction>("none");
  if (shown !== nav.page) {
    setDir(order.indexOf(nav.page) > order.indexOf(shown) ? "right" : "left");
    setShown(nav.page);
  }
  return (
    <main className="relative min-h-0 flex-1">
      <PageSlot page="shelf" dir={dir}>
        <ShelfPage />
      </PageSlot>
      <PageSlot page="discover" dir={dir}>
        <DiscoverPage />
      </PageSlot>
      <PageSlot page="installed" dir={dir}>
        <InstalledPage />
      </PageSlot>
    </main>
  );
}

/**
 * PageSlot shows a page while it is chosen. A page mounts the first time it
 * is chosen, so that what it reads and its icons wait until then.
 */
function PageSlot({ page, dir, children }: { page: Page; dir: Direction; children: ReactNode }) {
  const nav = useNav();
  const [visited, setVisited] = useState(nav.page === page);
  if (nav.page === page && !visited) setVisited(true);
  if (!visited) return null;
  return (
    <Activity mode={nav.page === page ? "visible" : "hidden"}>
      <div className="page-in absolute inset-0" data-dir={dir}>
        {children}
      </div>
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
