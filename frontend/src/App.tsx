// The window: the title bar with the pages, and the page shown. A page
// stays mounted once shown, keeping its search, filters and scroll.

import { SourceSetupPrompt } from "@/components/source-setup";
import { Titlebar } from "@/components/titlebar";
import { listenForChanges, useShelf } from "@/lib/api";
import { events } from "@/mygo";
import { DiscoverPage } from "@/pages/discover";
import { InstalledPage } from "@/pages/installed";
import { ShelfPage } from "@/pages/shelf";
import { UpdatesPage } from "@/pages/updates";
import { ActionsProvider } from "@/state/actions";
import { NavProvider, useNav, type Page } from "@/state/nav";
import { Activity, useEffect, useRef, useState, type ReactNode } from "react";
import { toast } from "sonner";

export default function App() {
  useEffect(() => listenForChanges(), []);
  return (
    <NavProvider>
      <ActionsProvider>
        <div className="ice-fog" aria-hidden>
          <span className="fog-1" />
          <span className="fog-2" />
          <span className="fog-3" />
        </div>
        <div className="flex h-full flex-col">
          <Titlebar />
          <Pages />
        </div>
        <OpeningNotes />
        <SourceSetupPrompt />
        <Navigation />
      </ActionsProvider>
    </NavProvider>
  );
}

/** The pages in the order of their tabs, which says the side a page enters from. */
const order: Page[] = ["installed", "shelf", "discover", "updates"];

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
      <PageSlot page="updates" dir={dir}>
        <UpdatesPage />
      </PageSlot>
    </main>
  );
}

/**
 * PageSlot shows a page while it is chosen. A page mounts the first time it
 * is chosen, so that what it reads and its icons wait until then. Its panels
 * float on the fog, a gap from the window's edges.
 */
function PageSlot({ page, dir, children }: { page: Page; dir: Direction; children: ReactNode }) {
  const nav = useNav();
  const [visited, setVisited] = useState(nav.page === page);
  if (nav.page === page && !visited) setVisited(true);
  if (!visited) return null;
  return (
    <Activity mode={nav.page === page ? "visible" : "hidden"}>
      <div className="page-in absolute inset-0 px-2 pb-2" data-dir={dir}>
        {children}
      </div>
    </Activity>
  );
}

const pages: ReadonlySet<string> = new Set<Page>(order);

/** Navigation shows the page Go asks for, as after a click on a notification. */
function Navigation() {
  const nav = useNav();
  const go = nav.go;
  useEffect(
    () =>
      events.navigate.on((page) => {
        if (pages.has(page)) go(page as Page);
      }),
    [go],
  );
  return null;
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
