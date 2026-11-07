// My shelf: the catalog's apps, filtered by state, category and tag, to
// install and update one or many; their details at the right.

import { Chip, EmptyState, SearchBox, SelectionBar } from "@/components/common";
import { CategoryPicker } from "@/components/pickers";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from "@/components/ui/resizable";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Skeleton } from "@/components/ui/skeleton";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { errorText, useShelf } from "@/lib/api";
import {
  countStatus,
  emptyFilter,
  filterApps,
  hasGroupFilter,
  inGroups,
  isFiltered,
  sortApps,
  type ShelfFilter,
  type SortKey,
} from "@/lib/shelf";
import { formatDateTime } from "@/lib/text";
import { revealRow, useList, usePageKeys } from "@/lib/use-list";
import { Shelf, type ShelfApp } from "@/mygo";
import { useActions } from "@/state/actions";
import { useNav } from "@/state/nav";
import {
  ArrowDownUpIcon,
  ArrowUpCircleIcon,
  DownloadIcon,
  FileDownIcon,
  FolderIcon,
  HashIcon,
  LayersIcon,
  ListFilterIcon,
  LoaderCircleIcon,
  MonitorCheckIcon,
  PackageOpenIcon,
  PencilLineIcon,
  PlusIcon,
  RefreshCwIcon,
  SearchIcon,
  SearchXIcon,
  Trash2Icon,
} from "lucide-react";
import { useDefaultLayout } from "react-resizable-panels";
import { useEffect, useMemo, useRef, useState } from "react";
import { toast } from "sonner";
import { ShelfDetail } from "./detail";
import { assignCategory, deleteApps } from "./ops";
import { ShelfRow, type RowActions } from "./row";
import { useCatalogFiles } from "./catalog-files";
import { ShelfSidebar } from "./sidebar";
import { TagMenu } from "./tag-menu";

const sorts: Array<{ value: SortKey; label: string }> = [
  { value: "added", label: "按加入顺序" },
  { value: "name", label: "按名称" },
  { value: "status", label: "可更新优先" },
];

function savedSort(): SortKey {
  const v = localStorage.getItem("shelf.sort");
  return sorts.some((s) => s.value === v) ? (v as SortKey) : "added";
}

export function ShelfPage() {
  const { data: shelf, isPending, isError, error } = useShelf();
  const actions = useActions();
  const nav = useNav();
  const [filter, setFilter] = useState<ShelfFilter>(emptyFilter);
  const [sort, setSort] = useState<SortKey>(savedSort);
  const searchRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const files = useCatalogFiles();

  const apps = useMemo(() => shelf?.apps ?? [], [shelf]);
  const categories = useMemo(
    () => new Map((shelf?.categories ?? []).map((c) => [c.id, c])),
    [shelf],
  );
  const tags = useMemo(() => new Map((shelf?.tags ?? []).map((t) => [t.id, t])), [shelf]);
  const byId = useMemo(() => new Map(apps.map((a) => [a.app.id, a])), [apps]);
  const counts = useMemo(
    () => countStatus(apps.filter((a) => inGroups(a, filter))),
    [apps, filter],
  );
  const shown = useMemo(() => sortApps(filterApps(apps, filter), sort), [apps, filter, sort]);
  const order = useMemo(() => shown.map((a) => a.app.id), [shown]);
  const list = useList(order, true);
  const focused = list.focus ? byId.get(list.focus) : undefined;
  const selected = useMemo(
    () => [...list.selected].map((id) => byId.get(id)).filter((a): a is ShelfApp => !!a),
    [list.selected, byId],
  );

  // Another page asked to show apps: everything is shown, they are opened.
  // The page's effects run again each time it shows, so a request is done
  // once.
  const focusSeq = nav.shelfFocus?.seq;
  const handled = useRef(0);
  useEffect(() => {
    const ids = nav.shelfFocus?.ids ?? [];
    if (!focusSeq || focusSeq === handled.current || ids.length === 0) return;
    handled.current = focusSeq;
    setFilter(emptyFilter);
    list.setSelected(ids.length > 1 ? ids : []);
    if (ids[0]) list.open(ids[0]);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [focusSeq]);
  useEffect(() => revealRow(listRef, list.focus), [list.focus]);

  usePageKeys({
    focusSearch: () => searchRef.current?.focus(),
    selectAll: () => list.selectAll(true),
    escape: () => {
      if (list.selected.size > 0) {
        list.selectAll(false);
        return true;
      }
      return false;
    },
    move: (d) => list.move(d),
    space: () => {
      if (list.focus) list.check(list.focus, !list.selected.has(list.focus), false);
    },
  });

  const remove = (targets: ShelfApp[]) => {
    void deleteApps(targets.map((t) => ({ id: t.app.id, name: t.app.name })));
  };
  const act: RowActions = {
    open: list.open,
    clickRow: list.clickRow,
    check: list.check,
    targets: (id) =>
      list.selected.has(id) ? selected : [byId.get(id)].filter((a): a is ShelfApp => !!a),
    install: (ids) => actions.preview(ids, "install"),
    upgrade: (ids) => actions.preview(ids, "upgrade"),
    edit: (app) => actions.edit(app),
    note: (item) => actions.editNote(item),
    remove,
  };

  const refresh = () =>
    Shelf.refresh().catch((err: unknown) =>
      toast.error("检查更新失败", { description: errorText(err) }),
    );
  const layout = useDefaultLayout({
    id: "shelf",
    storage: localStorage,
    panelIds: focused ? ["side", "list", "detail"] : ["side", "list"],
  });
  const allChecked = shown.length > 0 && list.selected.size === shown.length;
  const selecting = list.selected.size > 0;
  const updatable = selected.filter((a) => a.status?.state === "updateAvailable").length;
  const installable = selected.filter((a) => a.status?.state === "notInstalled").length;
  const names = new Map([...categories.values()].map((c) => [c.id, c.name]));

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
        maxSize={360}
        groupResizeBehavior="preserve-pixel-size"
      >
        <ShelfSidebar
          shelf={shelf}
          filter={filter}
          onFilter={setFilter}
          counts={counts}
          files={files}
        />
      </ResizablePanel>
      <ResizableHandle />
      <ResizablePanel id="list" minSize={460}>
        <div className="@container relative flex h-full min-h-0 flex-col bg-background">
          <div className="flex h-[52px] shrink-0 items-center gap-2 border-b px-3">
            <div className="flex min-w-0 shrink-0 items-center gap-2">
              <span className="max-w-60 truncate text-sm font-semibold">
                {title(filter, categories, tags)}
              </span>
              {hasGroupFilter(filter) && filter.status !== "all" && (
                <Chip icon={ListFilterIcon}>{statusTitles[filter.status]}</Chip>
              )}
              <Chip icon={LayersIcon}>{shown.length} 个</Chip>
              {shelf?.refreshing && (
                <Chip icon={LoaderCircleIcon} tone="primary" className="[&>svg]:animate-spin">
                  检查中
                </Chip>
              )}
            </div>
            <SearchBox
              value={filter.query}
              onChange={(query) => setFilter((f) => ({ ...f, query }))}
              placeholder="搜索清单"
              inputRef={searchRef}
              className="mx-auto w-full max-w-sm min-w-40"
            />
            <div className="flex shrink-0 items-center gap-1.5">
              <DropdownMenu>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <DropdownMenuTrigger asChild>
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        className="text-muted-foreground"
                        aria-label="排序"
                      >
                        <ArrowDownUpIcon />
                      </Button>
                    </DropdownMenuTrigger>
                  </TooltipTrigger>
                  <TooltipContent>
                    排序：{sorts.find((s) => s.value === sort)?.label}
                  </TooltipContent>
                </Tooltip>
                <DropdownMenuContent align="end" className="w-40">
                  <DropdownMenuRadioGroup
                    value={sort}
                    onValueChange={(v) => {
                      setSort(v as SortKey);
                      localStorage.setItem("shelf.sort", v);
                    }}
                  >
                    {sorts.map((s) => (
                      <DropdownMenuRadioItem key={s.value} value={s.value}>
                        {s.label}
                      </DropdownMenuRadioItem>
                    ))}
                  </DropdownMenuRadioGroup>
                </DropdownMenuContent>
              </DropdownMenu>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="outline" size="sm">
                    <PlusIcon />
                    添加软件
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-44">
                  <DropdownMenuItem onSelect={() => nav.search("")}>
                    <SearchIcon />
                    从发现搜索
                  </DropdownMenuItem>
                  <DropdownMenuItem onSelect={() => nav.go("installed")}>
                    <MonitorCheckIcon />
                    从本机已装添加
                  </DropdownMenuItem>
                  <DropdownMenuItem onSelect={() => actions.edit()}>
                    <PencilLineIcon />
                    手动添加
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    size="sm"
                    onClick={() => void refresh()}
                    disabled={shelf?.refreshing ?? true}
                  >
                    <RefreshCwIcon className={shelf?.refreshing ? "animate-spin" : undefined} />
                    检查更新
                  </Button>
                </TooltipTrigger>
                <TooltipContent>
                  {formatDateTime(shelf?.lastRefresh)
                    ? `上次检查：${formatDateTime(shelf?.lastRefresh)}`
                    : "还没有检查过"}
                </TooltipContent>
              </Tooltip>
            </div>
          </div>
          {isPending ? (
            <ListSkeleton />
          ) : isError ? (
            <EmptyState icon={SearchXIcon} title="清单读取失败" hint={errorText(error)} />
          ) : apps.length === 0 ? (
            <EmptyState icon={PackageOpenIcon} title="清单还是空的">
              <Button onClick={() => nav.search("")}>
                <SearchIcon />
                去发现添加
              </Button>
              <Button variant="outline" onClick={() => nav.go("installed")}>
                <MonitorCheckIcon />
                从本机已装添加
              </Button>
              <Button variant="outline" onClick={files.importFile} disabled={files.busy !== null}>
                <FileDownIcon />
                导入清单
              </Button>
              <Button variant="ghost" onClick={() => actions.edit()}>
                <PencilLineIcon />
                手动添加
              </Button>
            </EmptyState>
          ) : shown.length === 0 ? (
            <EmptyState icon={ListFilterIcon} title="没有符合条件的软件">
              <Button variant="outline" onClick={() => setFilter(emptyFilter)}>
                清除筛选
              </Button>
              {filter.query.trim() && (
                <Button onClick={() => nav.search(filter.query.trim())}>
                  <SearchIcon />
                  在发现中搜索「{filter.query.trim()}」
                </Button>
              )}
            </EmptyState>
          ) : (
            <>
              <div className="flex h-9 shrink-0 items-center gap-3 border-b bg-muted/30 px-3 text-xs text-muted-foreground">
                <Checkbox
                  checked={allChecked ? true : selecting ? "indeterminate" : false}
                  onCheckedChange={() => list.selectAll(!allChecked)}
                  aria-label="全选"
                />
                <span>{selecting ? `已选 ${list.selected.size} / ${shown.length}` : "全选"}</span>
                {isFiltered(filter) && (
                  <button
                    type="button"
                    className="ml-1 text-primary hover:underline"
                    onClick={() => setFilter(emptyFilter)}
                  >
                    清除筛选
                  </button>
                )}
                <span className="ml-auto w-[118px] text-right">状态</span>
                <span className="w-[64px]" />
              </div>
              <ScrollArea className="min-h-0 flex-1" viewportRef={listRef}>
                <div className="list-stagger anim-fade pb-20">
                  {shown.map((a) => (
                    <ShelfRow
                      key={a.app.id}
                      item={a}
                      categories={categories}
                      tags={tags}
                      checked={list.selected.has(a.app.id)}
                      focused={list.focus === a.app.id}
                      selecting={selecting}
                      act={act}
                    />
                  ))}
                  <button
                    type="button"
                    onClick={() => nav.search("")}
                    className="flex h-12 w-full items-center gap-2 px-4 text-sm text-muted-foreground transition-colors hover:bg-foreground/[0.035] hover:text-foreground"
                  >
                    <PlusIcon className="size-4" />
                    添加软件
                  </button>
                </div>
              </ScrollArea>
            </>
          )}
          <SelectionBar count={selected.length} onClear={() => list.selectAll(false)}>
            <Button
              size="sm"
              disabled={installable === 0}
              onClick={() =>
                actions.preview(
                  selected.map((a) => a.app.id),
                  "install",
                )
              }
            >
              <DownloadIcon />
              安装{installable > 0 && ` ${installable}`}
            </Button>
            <Button
              size="sm"
              variant="secondary"
              disabled={updatable === 0}
              onClick={() =>
                actions.preview(
                  selected.map((a) => a.app.id),
                  "upgrade",
                )
              }
            >
              <ArrowUpCircleIcon />
              更新{updatable > 0 && ` ${updatable}`}
            </Button>
            <CategoryPicker
              value={{ id: "__none" }}
              onChange={(c) =>
                void assignCategory(
                  selected.map((a) => ({ id: a.app.id, name: a.app.name })),
                  c,
                  names,
                )
              }
            >
              <Button size="sm" variant="ghost">
                <FolderIcon />
                设分类
              </Button>
            </CategoryPicker>
            <TagMenu apps={selected.map((a) => a.app)}>
              <Button size="sm" variant="ghost">
                <HashIcon />
                标签
              </Button>
            </TagMenu>
            <Button
              size="sm"
              variant="ghost"
              className="text-destructive hover:text-destructive"
              onClick={() => remove(selected)}
            >
              <Trash2Icon />
              删除
            </Button>
          </SelectionBar>
          {files.confirm}
        </div>
      </ResizablePanel>
      {focused && (
        <>
          <ResizableHandle />
          <ResizablePanel
            id="detail"
            defaultSize={360}
            minSize={300}
            maxSize={560}
            groupResizeBehavior="preserve-pixel-size"
          >
            <ShelfDetail
              item={focused}
              categories={categories}
              tags={tags}
              onClose={list.closeDetails}
              onInstall={() => actions.preview([focused.app.id], "install")}
              onUpgrade={() => actions.preview([focused.app.id], "upgrade")}
              onEdit={() => actions.edit(focused.app)}
              onDelete={() => remove([focused])}
            />
          </ResizablePanel>
        </>
      )}
    </ResizablePanelGroup>
  );
}

const statusTitles = {
  all: "",
  update: "可更新",
  installed: "已安装",
  notInstalled: "未安装",
  problem: "无法确认",
} as const;

/** title names the groups the list shows, or its state, or all. */
function title(
  f: ShelfFilter,
  categories: Map<string, { name: string }>,
  tags: Map<string, { name: string }>,
): string {
  const parts = [
    ...[...f.categories].map((id) => (id === "" ? "未分类" : (categories.get(id)?.name ?? ""))),
    ...[...f.tags].map((id) => `#${tags.get(id)?.name ?? ""}`),
  ].filter(Boolean);
  if (parts.length === 0) return statusTitles[f.status] || "全部软件";
  return parts.length > 2
    ? `${parts.slice(0, 2).join("、")} 等 ${parts.length} 项`
    : parts.join("、");
}

function ListSkeleton() {
  return (
    <div className="flex flex-1 flex-col">
      {Array.from({ length: 10 }, (_, i) => (
        <div
          key={i}
          className="flex h-[60px] items-center gap-3 border-b border-border/60 px-3"
          style={{ opacity: 1 - i * 0.08 }}
        >
          <Skeleton className="size-4 rounded" />
          <Skeleton className="size-9 rounded-[26%]" />
          <div className="flex flex-1 flex-col gap-1.5">
            <Skeleton className="h-3.5 w-48" />
            <Skeleton className="h-3 w-72" />
          </div>
          <Skeleton className="h-5 w-16 rounded-full" />
        </div>
      ))}
    </div>
  );
}
