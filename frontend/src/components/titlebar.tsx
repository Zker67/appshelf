import logo from "@/assets/logo.png";
import {
  DiscoverIcon,
  InstalledIcon,
  ShelfIcon,
  UninstallIcon,
  UpdatesIcon,
} from "@/components/nav-icons";
import { SetupActivity } from "@/components/source-setup";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { errorText, keys, queryClient, useAbout, useTasks, useUpdates } from "@/lib/api";
import { cn } from "@/lib/utils";
import { About, type ThemeSource } from "@/mygo";
import { useActions } from "@/state/actions";
import { useNav, type Page } from "@/state/nav";
import { queueProgress } from "@/lib/tasks";
import { ListChecksIcon, MonitorIcon, MoonIcon, SunIcon } from "lucide-react";
import { useLayoutEffect, useRef, useState, type ComponentType, type SVGProps } from "react";
import { toast } from "sonner";

const pages: Array<{ page: Page; label: string; icon: ComponentType<SVGProps<SVGSVGElement>> }> = [
  { page: "installed", label: "本机已装", icon: InstalledIcon },
  { page: "shelf", label: "我的清单", icon: ShelfIcon },
  { page: "discover", label: "发现", icon: DiscoverIcon },
  { page: "updates", label: "更新", icon: UpdatesIcon },
  { page: "uninstall", label: "卸载", icon: UninstallIcon },
];

/**
 * Titlebar is the window's only title bar: the app's icon, the pages, and
 * the tools every page shares. It is clear, over the fog, the panels below
 * it. Windows draws its buttons at the right.
 */
export function Titlebar() {
  return (
    <header
      className="app-drag relative z-30 flex shrink-0 items-center gap-3 pl-3.5"
      style={{
        height: "var(--mygo-titlebar-height, 44px)",
        paddingRight: "calc(var(--mygo-titlebar-inset-right, 0px) + 8px)",
      }}
    >
      <div className="brand flex shrink-0 items-center pr-1">
        <img
          src={logo}
          alt="软件图书馆"
          className="brand-logo size-6 drop-shadow-sm"
          draggable={false}
        />
      </div>
      <NavTabs />
      <div className="flex-1" />
      <Tools />
    </header>
  );
}

function NavTabs() {
  const nav = useNav();
  const { data: list } = useUpdates();
  // What the badge counts: the updates offered, AppShelf's own among them.
  const updates = (list?.count ?? 0) + (list?.self.available ? 1 : 0);
  const refs = useRef<Record<string, HTMLButtonElement | null>>({});
  const [pill, setPill] = useState<{ left: number; width: number } | null>(null);
  useLayoutEffect(() => {
    const el = refs.current[nav.page];
    if (el) setPill({ left: el.offsetLeft, width: el.offsetWidth });
  }, [nav.page, updates]);
  return (
    <nav className="seg-track app-no-drag relative flex items-center gap-0.5 rounded-xl p-[3px]">
      {pill && (
        <div
          className="nav-pill absolute top-[3px] bottom-[3px] rounded-[9px]"
          style={{ left: pill.left, width: pill.width }}
        />
      )}
      {pages.map(({ page, label, icon: Icon }) => {
        const active = nav.page === page;
        return (
          <button
            key={page}
            ref={(el) => {
              refs.current[page] = el;
            }}
            type="button"
            data-active={active}
            onClick={() => nav.go(page)}
            className={cn(
              "nav-tab relative z-10 flex h-[30px] items-center gap-1.5 rounded-[9px] px-3 text-[13px] font-medium transition-colors duration-200 outline-none focus-visible:ring-2 focus-visible:ring-ring/50",
              active
                ? "text-brand-foreground"
                : "text-muted-foreground hover:bg-hover hover:text-foreground",
            )}
          >
            <Icon className="size-[18px] transition-colors" />
            {label}
            {page === "updates" && updates > 0 && (
              <span
                className={cn(
                  "ml-0.5 rounded-full px-1.5 text-[10px] leading-4 font-semibold tabular-nums transition-colors",
                  active
                    ? "bg-brand-foreground/25 text-brand-foreground"
                    : "bg-primary text-primary-foreground",
                )}
              >
                {updates}
              </span>
            )}
          </button>
        );
      })}
    </nav>
  );
}

const themes: Array<{ value: ThemeSource; label: string; icon: typeof SunIcon }> = [
  { value: "system", label: "跟随系统", icon: MonitorIcon },
  { value: "light", label: "浅色", icon: SunIcon },
  { value: "dark", label: "深色", icon: MoonIcon },
];

/** RingTone is how the tasks stand: running, stopped with some waiting, or ended. */
type RingTone = "running" | "stopped" | "done" | "failed";

const ringTones: Record<RingTone, { arc: string; track: string; badge: string }> = {
  running: {
    arc: "stroke-primary",
    track: "stroke-primary/20",
    badge: "bg-primary/15 text-primary",
  },
  stopped: {
    arc: "stroke-warning",
    track: "stroke-warning/25",
    badge: "bg-warning/15 text-warning",
  },
  done: { arc: "stroke-success", track: "stroke-success/20", badge: "bg-success/15 text-success" },
  failed: {
    arc: "stroke-destructive",
    track: "stroke-destructive/20",
    badge: "bg-destructive/12 text-destructive",
  },
};

/**
 * TaskRing shows how far the tasks have come, turning while one runs; once
 * they end it closes on a check, or on a cross when one failed.
 */
function TaskRing({ value, tone }: { value: number; tone: RingTone }) {
  const r = 6.5;
  const c = 2 * Math.PI * r;
  const t = ringTones[tone];
  const spinning = tone === "running";
  return (
    <svg viewBox="0 0 16 16" className="size-4" aria-hidden>
      <g className="origin-center -rotate-90">
        <circle
          cx="8"
          cy="8"
          r={r}
          fill="none"
          strokeWidth="1.75"
          className={cn("transition-[stroke] duration-300", t.track)}
        />
        <circle
          cx="8"
          cy="8"
          r={r}
          fill="none"
          strokeWidth="1.75"
          strokeLinecap="round"
          className={cn("transition-[stroke,stroke-dashoffset] duration-500 ease-out", t.arc)}
          strokeDasharray={c}
          strokeDashoffset={c * (1 - Math.max(value, spinning ? 0.06 : 0))}
        />
        {spinning && (
          <circle
            cx="8"
            cy="8"
            r={r}
            fill="none"
            strokeWidth="1.75"
            strokeLinecap="round"
            className="origin-center animate-spin stroke-primary/60 [animation-duration:1.4s]"
            strokeDasharray={`${c * 0.18} ${c}`}
          />
        )}
      </g>
      {tone === "done" && (
        <path
          d="M5.4 8.2l1.8 1.8 3.5-3.7"
          pathLength={12}
          fill="none"
          strokeWidth="1.75"
          strokeLinecap="round"
          strokeLinejoin="round"
          className="task-mark stroke-success"
        />
      )}
      {tone === "failed" && (
        <path
          d="M6 6l4 4M10 6l-4 4"
          pathLength={12}
          fill="none"
          strokeWidth="1.75"
          strokeLinecap="round"
          className="task-mark stroke-destructive"
        />
      )}
    </svg>
  );
}

function Tools() {
  const actions = useActions();
  const { data: tasks } = useTasks();
  const { data: about } = useAbout();
  const progress = queueProgress(tasks?.items ?? []);
  // The ring stays after the tasks end, until those done are cleared.
  const tone: RingTone | null =
    progress.total === 0
      ? null
      : progress.active
        ? tasks?.running
          ? "running"
          : "stopped"
        : progress.failed > 0
          ? "failed"
          : "done";
  const theme = themes.find((t) => t.value === about?.theme) ?? themes[0]!;
  const ThemeIcon = theme.icon;
  const setTheme = async (value: string) => {
    try {
      await About.setTheme(value as ThemeSource);
      await queryClient.invalidateQueries({ queryKey: keys.about });
    } catch (err) {
      toast.error(errorText(err));
    }
  };
  return (
    <div className="app-no-drag flex items-center gap-0.5">
      <SetupActivity onOpen={() => actions.openSettings("sources")} />
      <Button
        variant="ghost"
        size="sm"
        className="gap-1.5 text-muted-foreground hover:text-foreground"
        data-tasks-trigger=""
        onClick={actions.toggleTasks}
      >
        {tone ? (
          <TaskRing value={progress.total ? progress.done / progress.total : 0} tone={tone} />
        ) : (
          <ListChecksIcon />
        )}
        任务
        {tone && (
          <span
            key={tone}
            className={cn(
              "anim-fade rounded-full px-1.5 text-[10px] leading-4 font-semibold tabular-nums",
              ringTones[tone].badge,
            )}
          >
            {tone === "failed" ? `失败 ${progress.failed}` : `${progress.done}/${progress.total}`}
          </span>
        )}
      </Button>
      <DropdownMenu>
        <Tooltip>
          <TooltipTrigger asChild>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="icon-sm"
                className="text-muted-foreground hover:text-foreground"
                aria-label="外观"
              >
                <ThemeIcon />
              </Button>
            </DropdownMenuTrigger>
          </TooltipTrigger>
          <TooltipContent>外观：{theme.label}</TooltipContent>
        </Tooltip>
        <DropdownMenuContent align="end" className="w-36">
          <DropdownMenuRadioGroup value={theme.value} onValueChange={setTheme}>
            {themes.map(({ value, label, icon: Icon }) => (
              <DropdownMenuRadioItem key={value} value={value}>
                <Icon />
                {label}
              </DropdownMenuRadioItem>
            ))}
          </DropdownMenuRadioGroup>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}
