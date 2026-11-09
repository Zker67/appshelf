// Uninstall: the programs of this machine weighed for removal, by the room
// they take and how long ago they were last opened; removed one by one or
// several at once, each previewed first. The removals of this run follow,
// with what they left behind to look for.

import { AppIcon } from "@/components/app-icon";
import { Chip, EmptyState, SearchBox, SelectionBar, WideLabel } from "@/components/common";
import { SettingsFooter, SideGroup, SideRow } from "@/components/side-nav";
import { SourceBadge, SourceIcon } from "@/components/source-badge";
import { Pill } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from "@/components/ui/resizable";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Skeleton } from "@/components/ui/skeleton";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { errorText, useInstalled, useTasks } from "@/lib/api";
import { formatBytes, matches } from "@/lib/text";
import {
  ageText,
  allRemoval,
  isAllRemoval,
  localDate,
  removalMatches,
  removalTasks,
  removing,
  sizeBand,
  sizeShare,
  sortForRemoval,
  totalSize,
  useBand,
  type RemovalFilter,
  type RemovalSort,
  type SizeBand,
  type UseBand,
} from "@/lib/uninstall";
import { revealRow, useList, usePageKeys } from "@/lib/use-list";
import { cn } from "@/lib/utils";
import {
  Installed,
  Tasks,
  type InstalledEntry,
  type InstalledKind,
  type TaskEntry,
  type TaskState,
} from "@/mygo";
import { useActions } from "@/state/actions";
import {
  ActivityIcon,
  AppWindowIcon,
  ArrowDownWideNarrowIcon,
  CalendarClockIcon,
  CheckCircle2Icon,
  CircleDashedIcon,
  CircleHelpIcon,
  CircleSlashIcon,
  ClockIcon,
  FeatherIcon,
  HardDriveIcon,
  HistoryIcon,
  HourglassIcon,
  LayoutGridIcon,
  LoaderCircleIcon,
  PackageIcon,
  PackageXIcon,
  RefreshCwIcon,
  RotateCcwIcon,
  SearchCheckIcon,
  SearchXIcon,
  TerminalIcon,
  XCircleIcon,
  type LucideIcon,
} from "lucide-react";
import { useEffect, useMemo, useRef, useState, type MouseEvent, type ReactNode } from "react";
import { useDefaultLayout } from "react-resizable-panels";
import { toast } from "sonner";
import { UninstallDetail } from "./detail";

const sortItems: Array<{ value: RemovalSort; label: string }> = [
  { value: "size", label: "占用空间" },
  { value: "unused", label: "久未打开" },
  { value: "recent", label: "最近打开" },
  { value: "name", label: "名称" },
];

const sortKey = "uninstall.sort";

function savedSort(): RemovalSort {
  const v = localStorage.getItem(sortKey);
  return sortItems.some((i) => i.value === v) ? (v as RemovalSort) : "size";
}

interface Band<T> {
  value: T;
  label: string;
  icon: LucideIcon;
  tone: string;
}

const sizeItems: Array<Band<SizeBand>> = [
  { value: "huge", label: "1 GB 以上", icon: HardDriveIcon, tone: "text-warning" },
  { value: "large", label: "100 MB – 1 GB", icon: PackageIcon, tone: "text-primary" },
  { value: "small", label: "100 MB 以下", icon: FeatherIcon, tone: "text-success" },
  { value: "unknown", label: "大小未知", icon: CircleHelpIcon, tone: "text-muted-foreground" },
];

const useItems: Array<Band<UseBand>> = [
  { value: "month", label: "一个月内打开过", icon: ActivityIcon, tone: "text-success" },
  { value: "months", label: "1–6 个月没打开", icon: CalendarClockIcon, tone: "text-primary" },
  { value: "stale", label: "半年以上没打开", icon: HourglassIcon, tone: "text-warning" },
  { value: "never", label: "没有打开记录", icon: CircleDashedIcon, tone: "text-muted-foreground" },
];

const kindItems: Array<{ value: InstalledKind; label: string }> = [
  { value: "winget", label: "winget" },
  { value: "scoop", label: "scoop" },
  { value: "local", label: "本机程序" },
  { value: "store", label: "应用商店" },
];

type View = "list" | "history";

export function UninstallPage() {
  const { data, isPending, isError, error } = useInstalled();
  const { data: tasks } = useTasks();
  const actions = useActions();
  const [now] = useState(() => new Date());
  const [view, setView] = useState<View>("list");
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<RemovalFilter>(allRemoval);
  const [sort, setSortState] = useState<RemovalSort>(savedSort);
  const setSort = (v: string) => {
    const next = sortItems.find((i) => i.value === v)?.value ?? "size";
    setSortState(next);
    localStorage.setItem(sortKey, next);
  };
  const searchRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);

  const entries = useMemo(() => data?.entries ?? [], [data]);
  const shown = useMemo(
    () =>
      sortForRemoval(
        entries.filter(
          (e) => removalMatches(e, filter, now) && matches(query, [e.name, e.publisher]),
        ),
        sort,
      ),
    [entries, filter, query, sort, now],
  );
  const largest = useMemo(() => Math.max(0, ...entries.map((e) => e.size ?? 0)), [entries]);
  const busy = useMemo(() => removing(tasks?.items ?? []), [tasks]);
  const history = useMemo(() => removalTasks(tasks?.items ?? []), [tasks]);

  const order = useMemo(() => shown.map((e) => e.key), [shown]);
  const byKey = useMemo(() => new Map(entries.map((e) => [e.key, e])), [entries]);
  const list = useList(order, false);
  const focused = view === "list" && list.focus ? byKey.get(list.focus) : undefined;
  const selected = [...list.selected]
    .map((k) => byKey.get(k))
    .filter((e): e is InstalledEntry => !!e && !busy.has(e.key));
  useEffect(() => revealRow(listRef, list.focus), [list.focus]);

  const countWith = (patch: Partial<RemovalFilter>) =>
    entries.filter((e) => removalMatches(e, { ...filter, ...patch }, now)).length;
  /** choose turns a row of a group on, or off when it is on, showing the list. */
  const choose = (patch: Partial<RemovalFilter>, active: boolean) => {
    const off: Partial<RemovalFilter> = {};
    if (patch.kinds) off.kinds = new Set();
    if (patch.size) off.size = "all";
    if (patch.used) off.used = "all";
    setFilter((f) => ({ ...f, ...(active ? off : patch) }));
    setView("list");
  };
  const uninstall = (es: InstalledEntry[]) => {
    const keys = es.filter((e) => !busy.has(e.key)).map((e) => e.key);
    if (keys.length > 0) actions.previewUninstall(keys);
  };

  usePageKeys({
    focusSearch: () => searchRef.current?.focus(),
    selectAll: () => {
      if (view === "list") list.selectAll(true);
    },
    escape: () => {
      if (list.selected.size > 0) {
        list.selectAll(false);
        return true;
      }
      return false;
    },
    move: (d) => {
      if (view === "list") list.move(d);
    },
    space: () => {
      if (view === "list" && list.focus)
        list.check(list.focus, !list.selected.has(list.focus), false);
    },
    enter: () => {
      const e = view === "list" && list.focus ? byKey.get(list.focus) : undefined;
      if (!e || busy.has(e.key)) return false;
      uninstall([e]);
      return true;
    },
  });

  const refresh = () =>
    Installed.refresh().catch((err: unknown) =>
      toast.error("刷新失败", { description: errorText(err) }),
    );
  const layout = useDefaultLayout({
    id: "uninstall",
    storage: localStorage,
    panelIds: focused ? ["side", "list", "detail"] : ["side", "list"],
  });
  const loading = isPending || (!data?.loaded && entries.length === 0);
  const shownSize = totalSize(shown);
  const chosenSize = totalSize(selected);

  return (
    <ResizablePanelGroup
      orientation="horizontal"
      defaultLayout={layout.defaultLayout}
      onLayoutChanged={layout.onLayoutChanged}
    >
      <ResizablePanel
        id="side"
        defaultSize={232}
        minSize={196}
        maxSize={340}
        groupResizeBehavior="preserve-pixel-size"
      >
        <div className="glass-panel flex h-full min-h-0 flex-col">
          <ScrollArea className="min-h-0 flex-1">
            <div className="flex flex-col gap-4 p-2.5">
              <div className="flex flex-col gap-0.5">
                <SideRow
                  icon={LayoutGridIcon}
                  label="全部程序"
                  count={entries.length}
                  active={view === "list" && isAllRemoval(filter)}
                  onClick={() => {
                    setFilter(allRemoval);
                    setView("list");
                  }}
                />
                <SideRow
                  icon={HistoryIcon}
                  iconClass="text-muted-foreground"
                  label="最近卸载"
                  count={history.length}
                  active={view === "history"}
                  onClick={() => setView("history")}
                />
              </div>
              <SideGroup title="占用空间">
                {sizeItems
                  .filter((s) => s.value !== "unknown" || countWith({ size: "unknown" }) > 0)
                  .map((s) => {
                    const active = view === "list" && filter.size === s.value;
                    return (
                      <SideRow
                        key={s.value}
                        icon={s.icon}
                        iconClass={s.tone}
                        label={s.label}
                        count={countWith({ size: s.value })}
                        active={active}
                        onClick={() => choose({ size: s.value }, active)}
                      />
                    );
                  })}
              </SideGroup>
              <SideGroup title="上次打开">
                {useItems.map((u) => {
                  const active = view === "list" && filter.used === u.value;
                  return (
                    <SideRow
                      key={u.value}
                      icon={u.icon}
                      iconClass={u.tone}
                      label={u.label}
                      count={countWith({ used: u.value })}
                      active={active}
                      onClick={() => choose({ used: u.value }, active)}
                    />
                  );
                })}
                {data?.loaded && !data.partial && !data.usageTracked && (
                  <p className="px-2.5 pt-1 text-[11px] leading-relaxed text-muted-foreground">
                    Windows 没有记录程序的打开。可在“设置 › 隐私和安全性 › 常规”里打开“允许 Windows
                    跟踪应用启动”
                  </p>
                )}
              </SideGroup>
              <SideGroup title="来源">
                {kindItems
                  .filter(
                    (k) => entries.some((e) => e.kind === k.value) || filter.kinds.has(k.value),
                  )
                  .map((k) => {
                    const active = view === "list" && filter.kinds.has(k.value);
                    return (
                      <SideRow
                        key={k.value}
                        icon={({ className }) => (
                          <SourceIcon type={k.value} className={cn("size-4", className)} />
                        )}
                        label={k.label}
                        count={countWith({ kinds: new Set([k.value]) })}
                        active={active}
                        onClick={() =>
                          choose({ kinds: new Set([k.value]) }, active && filter.kinds.size === 1)
                        }
                      />
                    );
                  })}
              </SideGroup>
            </div>
          </ScrollArea>
          <SettingsFooter />
        </div>
      </ResizablePanel>
      <ResizableHandle />
      <ResizablePanel id="list" minSize={520}>
        <div className="glass-panel glass-strong @container flex h-full min-h-0 flex-col">
          <div className="flex h-[52px] shrink-0 items-center gap-2 border-b px-3">
            <div className="flex min-w-0 items-center gap-2">
              {view === "list" ? (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <span>
                      <Chip icon={HardDriveIcon}>
                        {shown.length} 个 · {formatBytes(shownSize)}
                      </Chip>
                    </span>
                  </TooltipTrigger>
                  <TooltipContent className="max-w-72">
                    注册表里登记的占用空间合计，没有登记大小的程序不计入
                  </TooltipContent>
                </Tooltip>
              ) : (
                <Chip icon={PackageXIcon}>{history.length} 个</Chip>
              )}
              {data?.partial && view === "list" && (
                <Chip tone="primary" className="anim-fade">
                  <LoaderCircleIcon className="size-3.5 animate-spin" />
                  <WideLabel>正在补充来源</WideLabel>
                </Chip>
              )}
            </div>
            <SearchBox
              value={query}
              onChange={(q) => {
                setQuery(q);
                if (q.trim()) setView("list");
              }}
              placeholder="搜索名称或发布者"
              inputRef={searchRef}
              className="mx-auto w-full max-w-sm min-w-40"
            />
            {view === "list" && (
              <DropdownMenu>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <DropdownMenuTrigger asChild>
                      <Button variant="outline" size="sm" className="shrink-0" aria-label="排序">
                        <ArrowDownWideNarrowIcon />
                        <WideLabel>{sortItems.find((i) => i.value === sort)?.label}</WideLabel>
                      </Button>
                    </DropdownMenuTrigger>
                  </TooltipTrigger>
                  <TooltipContent>排序</TooltipContent>
                </Tooltip>
                <DropdownMenuContent align="end" className="w-36">
                  <DropdownMenuLabel>排序</DropdownMenuLabel>
                  <DropdownMenuRadioGroup value={sort} onValueChange={setSort}>
                    {sortItems.map((i) => (
                      <DropdownMenuRadioItem key={i.value} value={i.value}>
                        {i.label}
                      </DropdownMenuRadioItem>
                    ))}
                  </DropdownMenuRadioGroup>
                </DropdownMenuContent>
              </DropdownMenu>
            )}
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="outline"
                  size="sm"
                  className="shrink-0"
                  aria-label="重新读取"
                  disabled={data?.loading ?? true}
                  onClick={() => void refresh()}
                >
                  <RefreshCwIcon className={data?.loading ? "animate-spin" : undefined} />
                  <WideLabel>{data?.loading ? "读取中" : "重新读取"}</WideLabel>
                </Button>
              </TooltipTrigger>
              <TooltipContent>重新读取本机已装的程序</TooltipContent>
            </Tooltip>
          </div>
          {view === "history" ? (
            history.length === 0 ? (
              <EmptyState
                icon={HistoryIcon}
                title="这次还没有卸载过软件"
                hint="卸载的软件会列在这里，卸载成功后可以查找它留下的残留"
              >
                <Button variant="outline" onClick={() => setView("list")}>
                  <LayoutGridIcon />
                  查看全部程序
                </Button>
              </EmptyState>
            ) : (
              <ScrollArea className="min-h-0 flex-1">
                <div className="list-stagger anim-fade pb-20">
                  {history.map((t) => (
                    <HistoryRow key={t.id} task={t} />
                  ))}
                </div>
              </ScrollArea>
            )
          ) : isError ? (
            <EmptyState icon={SearchXIcon} title="读取失败" hint={errorText(error)}>
              <Button variant="outline" onClick={() => void refresh()}>
                <RefreshCwIcon />
                重新读取
              </Button>
            </EmptyState>
          ) : loading ? (
            <ListSkeleton />
          ) : shown.length === 0 ? (
            <EmptyState
              icon={SearchXIcon}
              title={query.trim() ? `没有找到「${query.trim()}」` : "没有符合条件的程序"}
            >
              <Button
                variant="outline"
                onClick={() => {
                  setQuery("");
                  setFilter(allRemoval);
                }}
              >
                清除筛选
              </Button>
            </EmptyState>
          ) : (
            <ScrollArea className="min-h-0 flex-1" viewportRef={listRef}>
              <div className="list-stagger anim-fade pb-20">
                {shown.map((e) => (
                  <ProgramRow
                    key={e.key}
                    entry={e}
                    now={now}
                    largest={largest}
                    task={busy.get(e.key)}
                    checked={list.selected.has(e.key)}
                    focused={list.focus === e.key}
                    selecting={list.selected.size > 0}
                    onClick={(ev) => list.clickRow(e.key, ev)}
                    onCheck={(on, shift) => list.check(e.key, on, shift)}
                    onUninstall={() => uninstall([e])}
                  />
                ))}
              </div>
            </ScrollArea>
          )}
          <SelectionBar
            count={view === "list" ? list.selected.size : 0}
            onClear={() => list.selectAll(false)}
          >
            <Button
              size="sm"
              variant="destructive"
              disabled={selected.length === 0}
              onClick={() => uninstall(selected)}
            >
              <PackageXIcon />
              卸载
              {chosenSize > 0 && (
                <span className="rounded-md bg-destructive/15 px-1.5 text-[11px] leading-[18px] font-semibold tabular-nums">
                  {formatBytes(chosenSize)}
                </span>
              )}
            </Button>
          </SelectionBar>
        </div>
      </ResizablePanel>
      {focused && (
        <>
          <ResizableHandle />
          <ResizablePanel
            id="detail"
            defaultSize={380}
            minSize={320}
            maxSize={600}
            groupResizeBehavior="preserve-pixel-size"
          >
            <UninstallDetail
              entry={focused}
              task={busy.get(focused.key)}
              onClose={list.closeDetails}
              onUninstall={() => uninstall([focused])}
            />
          </ResizablePanel>
        </>
      )}
    </ResizablePanelGroup>
  );
}

const sizeTone: Record<SizeBand, string> = {
  huge: "bg-warning/70",
  large: "bg-primary/60",
  small: "bg-success/60",
  unknown: "bg-transparent",
};

function ProgramRow({
  entry: e,
  now,
  largest,
  task,
  checked,
  focused,
  selecting,
  onClick,
  onCheck,
  onUninstall,
}: {
  entry: InstalledEntry;
  now: Date;
  largest: number;
  /** task is its removal, waiting or running. */
  task?: TaskEntry;
  checked: boolean;
  focused: boolean;
  selecting: boolean;
  onClick: (ev: MouseEvent) => void;
  onCheck: (on: boolean, shift: boolean) => void;
  onUninstall: () => void;
}) {
  const band = sizeBand(e.size);
  const used = ageText(e.lastUsed, now);
  const stale = useBand(e.lastUsed, now) === "stale";
  return (
    <div
      data-row={e.key}
      onClick={onClick}
      data-checked={checked}
      className={cn(
        "row-lazy list-row group/row relative flex h-[60px] items-center gap-3 px-3 select-none",
        focused && "row-focus",
      )}
    >
      <Checkbox
        checked={checked}
        disabled={!!task}
        aria-label={`选择 ${e.name}`}
        onClick={(ev) => {
          ev.preventDefault();
          ev.stopPropagation();
          onCheck(!checked, ev.shiftKey);
        }}
        className={cn(
          "transition-opacity",
          checked || selecting
            ? "opacity-100"
            : "opacity-0 group-hover/row:opacity-100 focus-visible:opacity-100",
        )}
      />
      <AppIcon path={e.icon} name={e.name} size={36} />
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 items-baseline gap-2">
          <span className="truncate text-sm font-medium select-text">{e.name}</span>
          {e.version && (
            <span className="shrink-0 text-xs text-muted-foreground tabular-nums">{e.version}</span>
          )}
        </div>
        <div className="mt-1 flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground">
          <SourceBadge type={e.kind} />
          {e.publisher && <span className="min-w-0 truncate">{e.publisher}</span>}
          {e.inShelf.length > 0 && (
            <span className="shrink-0 rounded bg-success/10 px-1.5 text-[11px] leading-[18px] text-success">
              在清单里
            </span>
          )}
        </div>
      </div>
      <div className="flex w-[118px] shrink-0 flex-col items-end gap-1">
        <span
          className={cn(
            "text-sm tabular-nums",
            band === "unknown" ? "text-muted-foreground/60" : "font-medium",
          )}
        >
          {band === "unknown" ? "大小未知" : formatBytes(e.size ?? 0)}
        </span>
        <span className="h-1 w-full overflow-hidden rounded-full bg-track">
          <span
            className={cn(
              "block h-full rounded-full transition-[width] duration-500",
              sizeTone[band],
            )}
            style={{ width: `${sizeShare(e.size, largest) * 100}%` }}
          />
        </span>
      </div>
      <div className="hidden w-[92px] shrink-0 flex-col items-end gap-0.5 @[640px]:flex">
        {e.lastUsed ? (
          <>
            <span className={cn("text-xs tabular-nums", stale && "text-warning")}>{used}打开</span>
            <span className="text-[11px] text-muted-foreground tabular-nums">
              {localDate(e.lastUsed)}
            </span>
          </>
        ) : (
          <span className="text-xs text-muted-foreground/60">没有打开记录</span>
        )}
      </div>
      <div className="flex w-[84px] shrink-0 justify-end">
        {task ? (
          <Pill tone={task.state === "running" ? "primary" : "muted"}>
            {task.state === "running" ? (
              <LoaderCircleIcon className="animate-spin" />
            ) : (
              <ClockIcon />
            )}
            {task.state === "running" ? "卸载中" : "等待卸载"}
          </Pill>
        ) : (
          <Button
            size="xs"
            variant="outline"
            className="transition-colors hover:border-destructive/40 hover:text-destructive"
            onClick={(ev) => {
              ev.stopPropagation();
              onUninstall();
            }}
          >
            <PackageXIcon />
            卸载
          </Button>
        )}
      </div>
    </div>
  );
}

const states: Record<
  TaskState,
  { tone: "primary" | "success" | "muted" | "danger"; icon: LucideIcon; label: string }
> = {
  waiting: { tone: "muted", icon: ClockIcon, label: "等待卸载" },
  running: { tone: "primary", icon: LoaderCircleIcon, label: "卸载中" },
  succeeded: { tone: "success", icon: CheckCircle2Icon, label: "已卸载" },
  skipped: { tone: "muted", icon: CircleSlashIcon, label: "已跳过" },
  failed: { tone: "danger", icon: XCircleIcon, label: "失败" },
};

/** HistoryRow is a removal of this run, and what can be done after it. */
function HistoryRow({ task: t }: { task: TaskEntry }) {
  const actions = useActions();
  const st = states[t.state];
  const Icon = st.icon;
  const failed = t.state === "failed";
  // A removal tried again shows in the task panel, as one started.
  const run = async (what: string, fn: () => Promise<unknown>) => {
    try {
      await fn();
      actions.openTasks();
    } catch (err) {
      toast.error(`${what}失败`, { description: errorText(err) });
    }
  };
  let sub: ReactNode = t.message;
  if (t.state === "running") sub = t.stage ?? "正在卸载";
  return (
    <div className="row-lazy list-row group/row relative flex h-[60px] items-center gap-3 px-3">
      <AppIcon path={t.icon} name={t.name} size={36} />
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-medium select-text">{t.name}</div>
        <div
          className={cn(
            "mt-1 truncate text-xs",
            failed ? "text-destructive" : "text-muted-foreground",
          )}
          title={typeof sub === "string" ? sub : undefined}
        >
          {sub || (t.state === "succeeded" ? "已从本机移除" : " ")}
        </div>
      </div>
      <Pill tone={st.tone}>
        <Icon className={t.state === "running" ? "animate-spin" : undefined} />
        {st.label}
      </Pill>
      <div className="flex shrink-0 items-center gap-1">
        {t.leftovers && (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                size="xs"
                variant="outline"
                className="transition-colors hover:border-primary/40 hover:text-primary"
                onClick={() => actions.findLeftovers(t.id, t.name)}
              >
                <SearchCheckIcon />
                查找残留
              </Button>
            </TooltipTrigger>
            <TooltipContent className="max-w-72">
              查找它留下的目录、设置、快捷方式和开机启动项，逐项确认后再清理
            </TooltipContent>
          </Tooltip>
        )}
        {failed && t.interactive && (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                size="xs"
                variant="outline"
                onClick={() => void run("重试", () => Tasks.retryInteractive(t.id))}
              >
                <AppWindowIcon />
                打开卸载程序
              </Button>
            </TooltipTrigger>
            <TooltipContent className="max-w-72">
              不再静默执行，打开它自己的卸载窗口，由你在窗口里完成
            </TooltipContent>
          </Tooltip>
        )}
        {failed && (
          <Button
            size="xs"
            variant="outline"
            onClick={() => void run("重试", () => Tasks.retry(t.id))}
          >
            <RotateCcwIcon />
            重试
          </Button>
        )}
        {t.hasLog && (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                size="icon-xs"
                variant="ghost"
                aria-label="查看日志"
                className="text-muted-foreground hover:text-foreground"
                onClick={actions.openTasks}
              >
                <TerminalIcon />
              </Button>
            </TooltipTrigger>
            <TooltipContent>在任务面板查看日志</TooltipContent>
          </Tooltip>
        )}
      </div>
    </div>
  );
}

function ListSkeleton() {
  return (
    <div className="flex flex-1 flex-col">
      {Array.from({ length: 11 }, (_, i) => (
        <div
          key={i}
          className="list-row pointer-events-none relative flex h-[60px] items-center gap-3 px-3"
          style={{ opacity: 1 - i * 0.075 }}
        >
          <Skeleton className="size-4 rounded" />
          <Skeleton className="size-9 rounded-[26%]" />
          <div className="flex flex-1 flex-col gap-1.5">
            <Skeleton className="h-3.5 w-52" />
            <Skeleton className="h-3 w-64" />
          </div>
          <div className="flex w-[118px] flex-col items-end gap-1.5">
            <Skeleton className="h-3.5 w-14" />
            <Skeleton className="h-1 w-full rounded-full" />
          </div>
          <Skeleton className="h-6 w-16 rounded-md" />
        </div>
      ))}
    </div>
  );
}
