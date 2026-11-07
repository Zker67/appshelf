import logo from "@/assets/logo.png";
import { DiscoverIcon, InstalledIcon, ShelfIcon } from "@/components/nav-icons";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { errorText, keys, queryClient, useAbout, useShelf, useTasks } from "@/lib/api";
import { cn } from "@/lib/utils";
import { About, type ThemeSource } from "@/mygo";
import { useActions } from "@/state/actions";
import { useNav, type Page } from "@/state/nav";
import {
  CircleHelpIcon,
  ListChecksIcon,
  LoaderCircleIcon,
  MonitorIcon,
  MoonIcon,
  SettingsIcon,
  SunIcon,
} from "lucide-react";
import { useLayoutEffect, useRef, useState, type ComponentType, type SVGProps } from "react";
import { toast } from "sonner";

const pages: Array<{ page: Page; label: string; icon: ComponentType<SVGProps<SVGSVGElement>> }> = [
  { page: "shelf", label: "我的清单", icon: ShelfIcon },
  { page: "discover", label: "发现", icon: DiscoverIcon },
  { page: "installed", label: "本机已装", icon: InstalledIcon },
];

/**
 * Titlebar is the window's only title bar: the app's name, the pages, and
 * the tools every page shares. Windows draws its buttons at the right.
 */
export function Titlebar() {
  return (
    <header
      className="app-drag relative z-30 flex shrink-0 items-center gap-3 border-b bg-titlebar/90 pl-3 backdrop-blur"
      style={{
        height: "var(--mygo-titlebar-height, 44px)",
        paddingRight: "calc(var(--mygo-titlebar-inset-right, 0px) + 8px)",
      }}
    >
      <div className="flex shrink-0 items-center gap-2 pr-2">
        <img src={logo} alt="" className="size-[22px] drop-shadow-sm" draggable={false} />
        <span className="text-[13px] font-semibold tracking-[0.08em]">软件商城</span>
      </div>
      <NavTabs />
      <div className="flex-1" />
      <Tools />
    </header>
  );
}

function NavTabs() {
  const nav = useNav();
  const { data: shelf } = useShelf();
  const updates = shelf?.apps.filter((a) => a.status?.state === "updateAvailable").length ?? 0;
  const refs = useRef<Record<string, HTMLButtonElement | null>>({});
  const [pill, setPill] = useState<{ left: number; width: number } | null>(null);
  useLayoutEffect(() => {
    const el = refs.current[nav.page];
    if (el) setPill({ left: el.offsetLeft, width: el.offsetWidth });
  }, [nav.page, updates]);
  return (
    <nav className="app-no-drag relative flex items-center gap-0.5 rounded-xl bg-foreground/[0.045] p-[3px] dark:bg-white/[0.05]">
      {pill && (
        <div
          className="absolute top-[3px] bottom-[3px] rounded-[9px] bg-card shadow-sm ring-1 ring-border/70 transition-[left,width] duration-300 ease-[var(--ease-out-soft)]"
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
                ? "text-foreground"
                : "text-muted-foreground hover:bg-foreground/[0.04] hover:text-foreground",
            )}
          >
            <Icon className={cn("size-[18px] transition-colors", active && "text-primary")} />
            {label}
            {page === "shelf" && updates > 0 && (
              <span className="ml-0.5 rounded-full bg-primary px-1.5 text-[10px] leading-4 font-semibold text-primary-foreground tabular-nums">
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

function Tools() {
  const actions = useActions();
  const { data: tasks } = useTasks();
  const { data: about } = useAbout();
  const pending =
    tasks?.items.filter((t) => t.state === "waiting" || t.state === "running").length ?? 0;
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
      <Button
        variant="ghost"
        size="sm"
        className="gap-1.5 text-muted-foreground hover:text-foreground"
        onClick={actions.openTasks}
      >
        {tasks?.running ? (
          <LoaderCircleIcon className="animate-spin text-primary" />
        ) : (
          <ListChecksIcon />
        )}
        任务
        {pending > 0 && (
          <span className="rounded-full bg-primary/15 px-1.5 text-[10px] leading-4 font-semibold text-primary tabular-nums">
            {pending}
          </span>
        )}
      </Button>
      <Button
        variant="ghost"
        size="sm"
        className="gap-1.5 text-muted-foreground hover:text-foreground"
        onClick={actions.openHelp}
      >
        <CircleHelpIcon />
        使用说明
      </Button>
      <Button
        variant="ghost"
        size="sm"
        className="gap-1.5 text-muted-foreground hover:text-foreground"
        onClick={() => actions.openSettings()}
      >
        <SettingsIcon />
        设置
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
