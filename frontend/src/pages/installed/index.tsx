// Installed: the programs of this machine, from winget, scoop, the registry
// and the Store, marked when the catalog has them or their source has a
// newer version; added to the catalog from here, or their source searched.
// They are filtered by three groups that combine: their source, their
// updates and whether the catalog has them.

import { AddTargetChip } from "@/components/add-target";
import { AppIcon } from "@/components/app-icon";
import { Chip, EmptyState, SearchBox, SelectionBar, WideLabel } from "@/components/common";
import type { AddCandidate } from "@/components/dialogs/add-dialog";
import { UninstallButton } from "@/components/dialogs/uninstall-dialog";
import { AddButton, InShelfButton } from "@/components/row-actions";
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
import { UpdateMenu } from "@/components/update-menu";
import { errorText, useInstalled, useUpdates } from "@/lib/api";
import { sourceID, sourceKey } from "@/lib/shelf";
import { formatBytes, formatDateTime, matches } from "@/lib/text";
import {
  allInstalled,
  holdText,
  installedMatches,
  isAllInstalled,
  sortInstalled,
  type InstalledFilter,
  type InstalledSort,
  type ShelfGroup,
  type UpdateGroup,
} from "@/lib/updates";
import { revealRow, useList, usePageKeys } from "@/lib/use-list";
import { cn } from "@/lib/utils";
import { Installed, type InstalledEntry, type InstalledKind, type UpdateItem } from "@/mygo";
import { useActions } from "@/state/actions";
import { useNav } from "@/state/nav";
import { useQuickAdd } from "@/state/quick-add";
import {
  ArrowDownWideNarrowIcon,
  ArrowUpCircleIcon,
  BellOffIcon,
  CheckIcon,
  CircleDashedIcon,
  LayoutDashboardIcon,
  LayersIcon,
  LayoutGridIcon,
  LoaderCircleIcon,
  MonitorCheckIcon,
  PlusIcon,
  PackageXIcon,
  RefreshCwIcon,
  SearchIcon,
  SearchXIcon,
  TriangleAlertIcon,
  ZapIcon,
  type LucideIcon,
} from "lucide-react";
import { useEffect, useMemo, useRef, useState, type MouseEvent } from "react";
import { useDefaultLayout } from "react-resizable-panels";
import { toast } from "sonner";
import { InstalledOverview } from "./overview";

const sortItems: Array<{ value: InstalledSort; label: string }> = [
  { value: "name", label: "名称" },
  { value: "size", label: "占用空间" },
  { value: "date", label: "安装日期" },
];

const sortKey = "installed.sort";

function savedSort(): InstalledSort {
  const v = localStorage.getItem(sortKey);
  return sortItems.some((i) => i.value === v) ? (v as InstalledSort) : "name";
}

const kindItems: Array<{ value: InstalledKind; label: string }> = [
  { value: "winget", label: "winget" },
  { value: "scoop", label: "scoop" },
  { value: "local", label: "本机程序" },
  { value: "store", label: "应用商店" },
];

interface GroupItem<T> {
  value: T;
  label: string;
  icon: LucideIcon;
  tone: string;
}

const updateItems: Array<GroupItem<Exclude<UpdateGroup, "all">>> = [
  { value: "update", label: "可更新", icon: ArrowUpCircleIcon, tone: "text-primary" },
  { value: "auto", label: "自动更新", icon: ZapIcon, tone: "text-primary" },
  { value: "held", label: "已忽略", icon: BellOffIcon, tone: "text-muted-foreground" },
];

const shelfItems: Array<GroupItem<Exclude<ShelfGroup, "all">>> = [
  { value: "inShelf", label: "已在清单", icon: CheckIcon, tone: "text-success" },
  {
    value: "notInShelf",
    label: "未加入清单",
    icon: CircleDashedIcon,
    tone: "text-muted-foreground",
  },
];

export function InstalledPage() {
  const { data, isPending, isError, error } = useInstalled();
  const actions = useActions();
  const nav = useNav();
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<InstalledFilter>(allInstalled);
  // The page opens on the overview; a filter or a search shows the list.
  const [overview, setOverview] = useState(true);
  const [sort, setSortState] = useState<InstalledSort>(savedSort);
  const setSort = (v: string) => {
    const next = sortItems.find((i) => i.value === v)?.value ?? "name";
    setSortState(next);
    localStorage.setItem(sortKey, next);
  };
  const searchRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);

  const entries = useMemo(() => data?.entries ?? [], [data]);
  const shown = useMemo(
    () =>
      sortInstalled(
        entries.filter(
          (e) =>
            installedMatches(e, filter) &&
            matches(query, [e.name, e.publisher, e.source ? sourceID(e.source) : ""]),
        ),
        sort,
      ),
    [entries, filter, query, sort],
  );
  // What the updates know of the packages with an update, as whether they
  // can update on their own.
  const { data: updates } = useUpdates();
  const updateOf = useMemo(
    () => new Map<string, UpdateItem>((updates?.items ?? []).map((i) => [i.key, i])),
    [updates],
  );
  const order = useMemo(() => shown.map((e) => e.key), [shown]);
  const byKey = useMemo(() => new Map(entries.map((e) => [e.key, e])), [entries]);
  const list = useList(order, false);
  const selected = [...list.selected]
    .map((k) => byKey.get(k))
    .filter((e): e is InstalledEntry => !!e);
  const addable = selected.filter((e) => e.source && e.inShelf.length === 0);
  const updatable = selected.filter((e) => e.source && !!e.update);
  useEffect(() => revealRow(listRef, list.focus), [list.focus]);

  // A row counts what the list would show with it chosen, the other groups
  // as they are.
  const countWith = (patch: Partial<InstalledFilter>) =>
    entries.filter((e) => installedMatches(e, { ...filter, ...patch })).length;
  const loading = !data?.loaded;

  const openList = (f: Partial<InstalledFilter>) => {
    setFilter({ ...allInstalled, ...f });
    setOverview(false);
  };
  /** choose turns a row of a group on, or off when active, showing the list. */
  const choose = (patch: Partial<InstalledFilter>, active: boolean) => {
    const off: Partial<InstalledFilter> = {};
    if (patch.kinds) off.kinds = new Set();
    if (patch.update) off.update = "all";
    if (patch.shelf) off.shelf = "all";
    setFilter((f) => ({ ...f, ...(active ? off : patch) }));
    setOverview(false);
  };
  const search = (q: string) => {
    setQuery(q);
    if (q.trim()) setOverview(false);
  };

  usePageKeys({
    focusSearch: () => searchRef.current?.focus(),
    selectAll: () => {
      if (!overview) list.selectAll(true);
    },
    escape: () => {
      if (list.selected.size > 0) {
        list.selectAll(false);
        return true;
      }
      return false;
    },
    move: (d) => {
      if (!overview) list.move(d);
    },
    space: () => {
      if (!overview && list.focus) list.check(list.focus, !list.selected.has(list.focus), false);
    },
    enter: () => {
      const e = !overview && list.focus ? byKey.get(list.focus) : undefined;
      if (!e) return false;
      if (e.inShelf.length > 0) nav.showInShelf(e.inShelf);
      else if (e.source) add([e]);
      else return false;
      return true;
    },
  });

  const quick = useQuickAdd((k) => (byKey.get(k)?.inShelf.length ?? 0) > 0);
  const candidate = (e: InstalledEntry): AddCandidate | null =>
    e.source ? { name: e.name, source: e.source, icon: e.icon } : null;
  /** add adds the programs with a source, not in the catalog, to the target chosen. */
  const add = (es: InstalledEntry[]) => {
    const items = es.flatMap((e) => {
      const cand = e.inShelf.length === 0 ? candidate(e) : null;
      return cand ? [{ key: e.key, cand }] : [];
    });
    void quick.add(items, false);
  };
  const findSource = (e: InstalledEntry) =>
    nav.search(e.name, { find: { name: e.name, rule: e.rule } });
  const refresh = () =>
    Installed.refresh().catch((err: unknown) =>
      toast.error("刷新失败", { description: errorText(err) }),
    );
  const layout = useDefaultLayout({
    id: "installed",
    storage: localStorage,
    panelIds: ["side", "list"],
  });
  const at = formatDateTime(data?.at);

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
                  icon={LayoutDashboardIcon}
                  label="概览"
                  active={overview}
                  onClick={() => setOverview(true)}
                />
                <SideRow
                  icon={LayoutGridIcon}
                  label="全部程序"
                  count={entries.length}
                  active={!overview && isAllInstalled(filter)}
                  onClick={() => openList({})}
                />
              </div>
              <SideGroup title="来源">
                {kindItems
                  .filter(
                    (k) => entries.some((e) => e.kind === k.value) || filter.kinds.has(k.value),
                  )
                  .map((k) => {
                    const active = !overview && filter.kinds.has(k.value);
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
              <SideGroup title="更新">
                {updateItems.map((u) => {
                  const active = !overview && filter.update === u.value;
                  return (
                    <SideRow
                      key={u.value}
                      icon={u.icon}
                      iconClass={u.tone}
                      label={u.label}
                      count={countWith({ update: u.value })}
                      active={active}
                      onClick={() => choose({ update: u.value }, active)}
                    />
                  );
                })}
              </SideGroup>
              <SideGroup title="清单">
                {shelfItems.map((g) => {
                  const active = !overview && filter.shelf === g.value;
                  return (
                    <SideRow
                      key={g.value}
                      icon={g.icon}
                      iconClass={g.tone}
                      label={g.label}
                      count={countWith({ shelf: g.value })}
                      active={active}
                      onClick={() => choose({ shelf: g.value }, active)}
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
              <span className="min-w-0 truncate text-sm font-semibold">本机已装</span>
              <Chip icon={LayersIcon}>{overview ? entries.length : shown.length} 个</Chip>
              {data?.partial && (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <span>
                      <Chip tone="primary" className="anim-fade">
                        <LoaderCircleIcon className="size-3.5 animate-spin" />
                        <WideLabel>正在补充来源</WideLabel>
                      </Chip>
                    </span>
                  </TooltipTrigger>
                  <TooltipContent className="max-w-80">
                    先列出了注册表里的程序；winget 的来源与更新、scoop 和应用商店应用读完后补上
                  </TooltipContent>
                </Tooltip>
              )}
              {(data?.notes.length ?? 0) > 0 && (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <span>
                      <Chip
                        icon={TriangleAlertIcon}
                        className="border-warning/30 bg-warning/10 text-warning"
                      >
                        {data?.notes.length} 条提示
                      </Chip>
                    </span>
                  </TooltipTrigger>
                  <TooltipContent className="max-w-96">
                    <ul className="flex flex-col gap-1">
                      {data?.notes.map((n) => (
                        <li key={n}>{n}</li>
                      ))}
                    </ul>
                  </TooltipContent>
                </Tooltip>
              )}
            </div>
            <SearchBox
              value={query}
              onChange={search}
              placeholder="搜索名称或发布者"
              inputRef={searchRef}
              className="mx-auto w-full max-w-sm min-w-40"
            />
            {!overview && (
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
            <AddTargetChip />
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
              <TooltipContent>
                {at ? `重新读取 · 读取于 ${at}` : "正在读取本机已装的程序"}
              </TooltipContent>
            </Tooltip>
          </div>
          {isError ? (
            <EmptyState icon={SearchXIcon} title="读取失败" hint={errorText(error)}>
              <Button variant="outline" onClick={() => void refresh()}>
                <RefreshCwIcon />
                重新读取
              </Button>
            </EmptyState>
          ) : isPending || (loading && entries.length === 0) ? (
            <ListSkeleton />
          ) : overview ? (
            <InstalledOverview
              entries={entries}
              at={at}
              onOpen={openList}
              onUpdate={actions.previewInstalledUpdates}
              onAdd={add}
            />
          ) : shown.length === 0 ? (
            <EmptyState
              icon={SearchXIcon}
              title={query.trim() ? `没有找到「${query.trim()}」` : "没有符合条件的程序"}
            >
              <Button
                variant="outline"
                onClick={() => {
                  setQuery("");
                  setFilter(allInstalled);
                }}
              >
                清除筛选
              </Button>
              {query.trim() && (
                <Button onClick={() => nav.search(query.trim())}>
                  <SearchIcon />
                  在发现中搜索
                </Button>
              )}
            </EmptyState>
          ) : (
            <ScrollArea className="min-h-0 flex-1" viewportRef={listRef}>
              <div className="list-stagger anim-fade pb-20">
                {shown.map((e) => (
                  <EntryRow
                    key={e.key}
                    entry={e}
                    checked={list.selected.has(e.key)}
                    focused={list.focus === e.key}
                    selecting={list.selected.size > 0}
                    onClick={(ev) => list.clickRow(e.key, ev)}
                    onCheck={(on, shift) => list.check(e.key, on, shift)}
                    adding={quick.adding.has(e.key)}
                    addHint={quick.hint}
                    onAdd={() => add([e])}
                    onFind={() => findSource(e)}
                    onShow={() => nav.showInShelf(e.inShelf)}
                    onUpdate={() => actions.previewInstalledUpdates([e.key])}
                    onUninstall={() => actions.previewUninstall([e.key])}
                    update={e.source ? updateOf.get(sourceKey(e.source)) : undefined}
                  />
                ))}
              </div>
            </ScrollArea>
          )}
          <SelectionBar
            count={overview ? 0 : selected.length}
            onClear={() => list.selectAll(false)}
          >
            <Button
              size="sm"
              disabled={updatable.length === 0}
              onClick={() => actions.previewInstalledUpdates(updatable.map((e) => e.key))}
            >
              <ArrowUpCircleIcon />
              更新{updatable.length > 0 && ` ${updatable.length}`}
            </Button>
            <Button size="sm" disabled={addable.length === 0} onClick={() => add(selected)}>
              <PlusIcon />
              加入清单{addable.length > 0 && ` ${addable.length}`}
            </Button>
            <Button
              size="sm"
              variant="outline"
              className="hover:border-destructive/40 hover:text-destructive"
              disabled={selected.length === 0}
              onClick={() => actions.previewUninstall(selected.map((e) => e.key))}
            >
              <PackageXIcon />
              卸载{selected.length > 0 && ` ${selected.length}`}
            </Button>
          </SelectionBar>
        </div>
      </ResizablePanel>
    </ResizablePanelGroup>
  );
}

function EntryRow({
  entry: e,
  checked,
  focused,
  selecting,
  adding,
  addHint,
  onClick,
  onCheck,
  onAdd,
  onFind,
  onShow,
  onUpdate,
  onUninstall,
  update,
}: {
  entry: InstalledEntry;
  checked: boolean;
  focused: boolean;
  selecting: boolean;
  adding: boolean;
  addHint: string;
  onClick: (ev: MouseEvent) => void;
  onCheck: (on: boolean, shift: boolean) => void;
  onAdd: () => void;
  onFind: () => void;
  onShow: () => void;
  onUpdate: () => void;
  onUninstall: () => void;
  /** update is what the updates know of its package. */
  update?: UpdateItem;
}) {
  const inShelf = e.inShelf.length > 0;
  const held = !!e.update && !!e.hold;
  const hasMode = !!e.mode && e.mode !== "notify";
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
          {e.source && (
            <span className="max-w-[50%] shrink-0 truncate font-mono">{sourceID(e.source)}</span>
          )}
          {e.publisher && <span className="min-w-0 truncate">{e.publisher}</span>}
        </div>
      </div>
      {(e.size || e.installed) && (
        <div className="hidden w-[84px] shrink-0 flex-col items-end gap-0.5 text-[11px] leading-tight text-muted-foreground tabular-nums @[760px]:flex">
          {e.size ? <span>{formatBytes(e.size)}</span> : null}
          {e.installed && <span>{e.installed}</span>}
        </div>
      )}
      <div className="flex shrink-0 items-center gap-1">
        {held && (
          <Pill tone="muted">
            <BellOffIcon />
            {holdText(e.hold, { skip: update?.skip, until: update?.until })}
          </Pill>
        )}
        {e.update && !held && (
          <Tooltip>
            <TooltipTrigger asChild>
              <span>
                <Pill tone="primary">
                  <ArrowUpCircleIcon />
                  可更新 {e.update}
                </Pill>
              </span>
            </TooltipTrigger>
            <TooltipContent>
              {e.source ? "可直接更新，无需先加入清单" : "需要可用的更新来源"}
            </TooltipContent>
          </Tooltip>
        )}
      </div>
      {e.source && (e.update || hasMode) && (
        <UpdateMenu
          target={{
            key: sourceKey(e.source),
            name: e.name,
            mode: e.mode ?? "notify",
            hold: e.hold,
            latest: e.update,
            canAuto: update?.canAuto,
            autoNote: update?.autoNote,
          }}
        />
      )}
      {e.update && e.source && !held && (
        <Button
          size="xs"
          variant="outline"
          className="transition-colors hover:border-primary/40 hover:text-primary"
          onClick={(ev) => {
            ev.stopPropagation();
            onUpdate();
          }}
        >
          <ArrowUpCircleIcon />
          更新
        </Button>
      )}
      <UninstallButton
        onClick={onUninstall}
        className="opacity-0 transition-opacity group-hover/row:opacity-100 focus-visible:opacity-100"
      />
      <div className="flex w-[104px] shrink-0 justify-end">
        {inShelf ? (
          <InShelfButton onClick={onShow} />
        ) : e.source ? (
          <AddButton
            label="加入清单"
            adding={adding}
            hint={e.source.type === "github" ? "确认检测规则后加入" : addHint}
            onClick={onAdd}
          />
        ) : (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                size="xs"
                variant="ghost"
                className="text-muted-foreground group-hover/row:text-foreground"
                onClick={(ev) => {
                  ev.stopPropagation();
                  onFind();
                }}
              >
                <MonitorCheckIcon />
                查找来源
              </Button>
            </TooltipTrigger>
            <TooltipContent>到发现页搜索它的来源，加入时带上检测规则</TooltipContent>
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
          <Skeleton className="h-6 w-16 rounded-md" />
        </div>
      ))}
    </div>
  );
}
