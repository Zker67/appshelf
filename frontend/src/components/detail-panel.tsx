// The details column of a page: the item's icon and name, its actions,
// then what is known of it in tabs, each scrolling on its own. Scrolled
// down, the head folds to a line and keeps the actions in view.

import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Skeleton } from "@/components/ui/skeleton";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { XIcon, type LucideIcon } from "lucide-react";
import { Tabs as TabsPrimitive } from "radix-ui";
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";

/** Unfold puts the head back to its full size, as a tab is changed. */
const Unfold = createContext<() => void>(() => {});

// The icon's box: the icon, 40 pixels, in a 6 pixel frame; folded smaller.
const iconBox = 52;
const iconBoxFolded = 34;

export function DetailPanel({
  icon,
  title,
  subtitle,
  badges,
  actions,
  onClose,
  children,
}: {
  icon: ReactNode;
  title: string;
  /** A line under the title, as the item's ID. */
  subtitle?: ReactNode;
  badges?: ReactNode;
  actions?: ReactNode;
  onClose: () => void;
  /** What is known of the item, as DetailTabs. */
  children: ReactNode;
}) {
  const [folded, setFolded] = useState(false);
  const body = useRef<HTMLDivElement>(null);
  const unfold = useCallback(() => setFolded(false), []);

  // Scroll events do not bubble: the body catches its tabs' as they go
  // down. The head folds only when what remains to scroll is long enough
  // that the room it gives back does not unfold it again.
  useEffect(() => {
    const el = body.current;
    if (!el) return;
    const onScroll = (e: Event) => {
      const v = e.target;
      if (!(v instanceof HTMLElement) || v.dataset.slot !== "scroll-area-viewport") return;
      const top = v.scrollTop;
      setFolded((f) => (f ? top > 4 : top > 32 && v.scrollHeight - v.clientHeight > 120));
    };
    el.addEventListener("scroll", onScroll, { capture: true, passive: true });
    return () => el.removeEventListener("scroll", onScroll, { capture: true });
  }, []);

  const box = folded ? iconBoxFolded : iconBox;
  return (
    <div className="flex h-full min-h-0 flex-col bg-surface">
      <div className="shrink-0 border-b bg-linear-to-b from-primary/[0.08] via-primary/[0.03] to-transparent dark:from-primary/[0.14] dark:via-primary/[0.05]">
        <div
          className={cn(
            "anim-panel flex items-center gap-3.5 px-5 transition-[padding] duration-300 ease-out-soft",
            folded ? "pt-3 pb-2.5" : "pt-5 pb-4",
          )}
        >
          <div
            className="anim-rise relative shrink-0 transition-[width,height] duration-300 ease-out-soft [animation-delay:60ms]"
            style={{ width: box, height: box }}
          >
            <div
              className="absolute top-0 left-0 origin-top-left rounded-[28%] bg-card p-1.5 shadow-soft ring-1 ring-border/70 transition-transform duration-300 ease-out-soft"
              style={{ transform: `scale(${box / iconBox})` }}
            >
              {icon}
            </div>
          </div>
          <div className="min-w-0 flex-1">
            <div
              className={cn(
                "leading-snug font-semibold tracking-tight transition-[font-size] duration-300 ease-out-soft select-text",
                folded ? "truncate text-[15px]" : "text-[17px] break-words",
              )}
              title={folded ? title : undefined}
            >
              {title}
            </div>
            {(subtitle || badges) && (
              <div
                className={cn(
                  "grid transition-[grid-template-rows,opacity] duration-300 ease-out-soft",
                  folded ? "grid-rows-[0fr] opacity-0" : "grid-rows-[1fr]",
                )}
                inert={folded}
              >
                <div className="min-h-0 overflow-hidden">
                  {subtitle && (
                    <div className="mt-0.5 flex min-w-0 items-center gap-1 text-xs text-muted-foreground">
                      {subtitle}
                    </div>
                  )}
                  {badges && (
                    <div className="mt-1.5 flex flex-wrap items-center gap-1.5 pb-px">{badges}</div>
                  )}
                </div>
              </div>
            )}
          </div>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon-sm"
                className={cn(
                  "-mr-2 text-muted-foreground transition-transform hover:rotate-90",
                  !folded && "-mt-1 self-start",
                )}
                onClick={onClose}
                aria-label="关闭详情"
              >
                <XIcon />
              </Button>
            </TooltipTrigger>
            <TooltipContent>关闭详情</TooltipContent>
          </Tooltip>
        </div>
        {actions && (
          <div
            className={cn(
              "anim-panel flex flex-wrap items-center gap-2 px-5 transition-[padding] duration-300 ease-out-soft [animation-delay:50ms]",
              folded ? "pb-3" : "pb-4",
            )}
          >
            {actions}
          </div>
        )}
      </div>
      <Unfold.Provider value={unfold}>
        <div ref={body} className="anim-panel flex min-h-0 flex-1 flex-col [animation-delay:100ms]">
          {children}
        </div>
      </Unfold.Provider>
    </div>
  );
}

/** DetailTab is a tab of the details: its name, its mark, what it shows. */
export interface DetailTab {
  value: string;
  label: string;
  icon: LucideIcon;
  content: ReactNode;
}

/**
 * DetailTabs shows the details in tabs, the chosen one marked by a pill that
 * slides to it, each scrolling on its own. One tab shows no bar; `pending`
 * tabs still being read hold their place until known.
 */
export function DetailTabs({ tabs, pending = 0 }: { tabs: DetailTab[]; pending?: number }) {
  const unfold = useContext(Unfold);
  const [chosen, setChosen] = useState(tabs[0]?.value ?? "");
  const [dir, setDir] = useState<"left" | "right" | undefined>(undefined);
  const value = tabs.some((t) => t.value === chosen) ? chosen : (tabs[0]?.value ?? "");
  const bar = tabs.length + pending > 1;

  const list = useRef<HTMLDivElement>(null);
  const [pill, setPill] = useState<{ left: number; width: number } | null>(null);
  const keys = tabs.map((t) => t.value).join("|");
  useLayoutEffect(() => {
    const el = list.current;
    if (!el) return;
    const measure = () => {
      const tab = el.querySelector<HTMLElement>(`[data-tab="${CSS.escape(value)}"]`);
      setPill(tab ? { left: tab.offsetLeft, width: tab.offsetWidth } : null);
    };
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, [value, keys, pending]);

  const change = (v: string) => {
    const from = tabs.findIndex((t) => t.value === value);
    const to = tabs.findIndex((t) => t.value === v);
    setDir(to > from ? "right" : "left");
    setChosen(v);
    unfold();
  };

  return (
    <TabsPrimitive.Root
      value={value}
      onValueChange={change}
      className="flex min-h-0 flex-1 flex-col"
    >
      <div
        className={cn(
          "grid shrink-0 transition-[grid-template-rows,opacity] duration-300 ease-out-soft",
          bar ? "grid-rows-[1fr]" : "grid-rows-[0fr] opacity-0",
        )}
      >
        <div className="min-h-0 overflow-hidden">
          <div className="px-5 pt-3 pb-1">
            <TabsPrimitive.List
              ref={list}
              aria-label="详情"
              className="relative flex rounded-xl bg-foreground/[0.045] p-[3px] dark:bg-white/[0.05]"
            >
              {pill && (
                <div
                  className="absolute top-[3px] bottom-[3px] rounded-[9px] bg-card shadow-sm ring-1 ring-border/60 transition-[left,width] duration-400 ease-spring dark:bg-white/[0.09] dark:ring-white/[0.06]"
                  style={{ left: pill.left, width: pill.width }}
                />
              )}
              {tabs.map(({ value: v, label, icon: Icon }) => (
                <TabsPrimitive.Trigger
                  key={v}
                  value={v}
                  data-tab={v}
                  className="group/tab relative flex h-7 min-w-0 flex-1 items-center justify-center gap-1.5 rounded-[9px] px-2 text-[13px] font-medium text-muted-foreground transition-colors outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/50 data-[state=active]:text-foreground"
                >
                  <Icon className="size-3.5 shrink-0 transition-[color,transform] duration-300 group-hover/tab:scale-110 group-data-[state=active]/tab:text-primary" />
                  <span className="truncate">{label}</span>
                </TabsPrimitive.Trigger>
              ))}
              {Array.from({ length: pending }, (_, i) => (
                <div key={i} className="flex h-7 flex-1 items-center justify-center">
                  <Skeleton className="h-3 w-12" />
                </div>
              ))}
            </TabsPrimitive.List>
          </div>
        </div>
      </div>
      {tabs.map(({ value: v, content }) => (
        <TabsPrimitive.Content
          key={v}
          value={v}
          className="flex min-h-0 flex-1 flex-col outline-none data-[state=inactive]:hidden"
        >
          <ScrollArea className="min-h-0 flex-1">
            <div className={cn("page-in px-5 pb-5", bar ? "pt-3" : "pt-4")} data-dir={dir}>
              {content}
            </div>
          </ScrollArea>
        </TabsPrimitive.Content>
      ))}
    </TabsPrimitive.Root>
  );
}
