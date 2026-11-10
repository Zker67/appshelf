// Discover: one search over winget and scoop as you type, GitHub when
// asked; what is already in the catalog or installed is marked. Adding is a
// click; adding by hand is for what no search finds.

import { AddTargetChip } from "@/components/add-target";
import { AppIcon } from "@/components/app-icon";
import { CopyButton, EmptyState, SearchBox, SelectionBar, WideLabel } from "@/components/common";
import type { AddCandidate } from "@/components/dialogs/add-dialog";
import { DetailPanel } from "@/components/detail-panel";
import { PackageInfo, SourceLink } from "@/components/package-info";
import { AddButton, InShelfButton } from "@/components/row-actions";
import { SettingsFooter, SideGroup, SideRow } from "@/components/side-nav";
import { GitHubMark, SourceBadge, SourceIcon } from "@/components/source-badge";
import { Pill } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from "@/components/ui/resizable";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Skeleton } from "@/components/ui/skeleton";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { errorText, useShelf, useSources } from "@/lib/api";
import { sourceKey, sourceLabel, type SourceKind } from "@/lib/shelf";
import { fold, formatDateTime, formatStars } from "@/lib/text";
import { pairTwins } from "@/lib/twins";
import { revealRow, useList, usePageKeys } from "@/lib/use-list";
import { cn } from "@/lib/utils";
import { Discover, type PackageRef, type SearchBatch, type SearchHit, type Source } from "@/mygo";
import { useActions } from "@/state/actions";
import { useNav, type FindSource } from "@/state/nav";
import { useQuickAdd } from "@/state/quick-add";
import { useQueryClient } from "@tanstack/react-query";
import {
  ArchiveIcon,
  ArrowRightIcon,
  CheckIcon,
  DownloadIcon,
  LayersIcon,
  LoaderCircleIcon,
  MergeIcon,
  MonitorCheckIcon,
  PencilLineIcon,
  PlusIcon,
  SearchIcon,
  SearchXIcon,
  StarIcon,
  TriangleAlertIcon,
  XIcon,
} from "lucide-react";
import { Channel } from "mygo-runtime";
import { useEffect, useMemo, useRef, useState, type MouseEvent } from "react";
import { useDefaultLayout } from "react-resizable-panels";
import { toast } from "sonner";

const kinds: SourceKind[] = ["winget", "scoop", "github"];

interface SourceResult {
  state: "idle" | "loading" | "done" | "error" | "unavailable";
  hits: SearchHit[];
  error?: string;
}

type Results = Record<SourceKind, SourceResult>;

const idle: SourceResult = { state: "idle", hits: [] };
const idleResults: Results = { winget: idle, scoop: idle, github: idle };

const examples = ["Visual Studio Code", "7-Zip", "PowerToys", "Everything", "Obsidian", "Git"];

/** searchable: two letters at least, or a Chinese character. */
function searchable(q: string): boolean {
  return fold(q).length >= 2 || /\p{Script=Han}/u.test(q);
}

const keyOf = (h: SearchHit) => `${h.type}:${h.id}`;

/** sourceOf is the source of the catalog a search hit is. */
export function sourceOf(h: SearchHit): Source {
  switch (h.type) {
    case "scoop": {
      const [bucket, ...name] = h.id.split("/");
      return { type: "scoop", bucket: bucket ?? "", name: name.join("/") };
    }
    case "github":
      return { type: "github", repo: h.id };
    default:
      return { type: "winget", id: h.id };
  }
}

/**
 * pickOf is the source chosen of a winget row with a scoop twin: as the user
 * chose, else the one in the catalog, else winget.
 */
function pickOf(h: SearchHit, twin: SearchHit, picks: Record<string, SourceKind>): SourceKind {
  return picks[keyOf(h)] ?? (twin.inShelf && !h.inShelf ? "scoop" : "winget");
}

export function DiscoverPage() {
  const nav = useNav();
  const actions = useActions();
  const queryClient = useQueryClient();
  const { data: sources } = useSources();
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<Results>(idleResults);
  const [searched, setSearched] = useState("");
  const [only, setOnly] = useState<SourceKind | null>(null);
  const [find, setFind] = useState<FindSource | null>(null);
  /** picks: the source chosen of a result in winget and scoop, by its row. */
  const [picks, setPicks] = useState<Record<string, SourceKind>>({});
  const gen = useRef(0);
  const searchRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);

  const setSource = (my: number, kind: SourceKind, r: SourceResult) => {
    if (my === gen.current) setResults((all) => ({ ...all, [kind]: r }));
  };

  const searchGitHub = (q: string, my = gen.current) => {
    const text = q.trim();
    if (!searchable(text)) return;
    setSource(my, "github", { state: "loading", hits: results.github.hits });
    Discover.searchGitHub(text)
      .then((res) => setSource(my, "github", { state: "done", hits: res.hits }))
      .catch((err: unknown) =>
        setSource(my, "github", { state: "error", hits: [], error: errorText(err) }),
      )
      .finally(() => void queryClient.invalidateQueries({ queryKey: ["sources"] }));
  };

  const search = (q: string, withGitHub: boolean) => {
    const my = ++gen.current;
    const text = q.trim();
    setSearched(text);
    if (!searchable(text)) {
      setResults(idleResults);
      return;
    }
    setResults({
      winget: { state: "loading", hits: [] },
      scoop: { state: "loading", hits: [] },
      github: withGitHub ? { state: "loading", hits: [] } : idle,
    });
    const channel = new Channel<SearchBatch>((batch) => {
      const kind = batch.type as SourceKind;
      if (batch.unavailable)
        setSource(my, kind, { state: "unavailable", hits: [], error: batch.error });
      else if (batch.error) setSource(my, kind, { state: "error", hits: [], error: batch.error });
      else setSource(my, kind, { state: "done", hits: batch.hits });
    });
    Discover.search(text, channel)
      .catch((err: unknown) => {
        if (my === gen.current) toast.error("搜索失败", { description: errorText(err) });
      })
      .finally(() => {
        // A source that sent nothing found nothing.
        if (my !== gen.current) return;
        setResults((all) => {
          const out = { ...all };
          for (const k of ["winget", "scoop"] as const)
            if (out[k].state === "loading") out[k] = { state: "done", hits: [] };
          return out;
        });
      });
    if (withGitHub) searchGitHub(text, my);
  };

  // Ready the searches as the page shows, for the first one to be quick.
  useEffect(() => {
    void Discover.warm();
  }, []);

  // Typing searches after a pause.
  useEffect(() => {
    if (query.trim() === searched) return;
    const t = window.setTimeout(() => search(query, false), 250);
    return () => window.clearTimeout(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [query]);

  // Another page asked for a search. The page's effects run again each
  // time it shows, so a request is done once.
  const intent = nav.discoverIntent;
  const handled = useRef(0);
  useEffect(() => {
    if (!intent || intent.seq === handled.current) return;
    handled.current = intent.seq;
    if (intent.query.trim() || intent.find) {
      setQuery(intent.query);
      setFind(intent.find ?? null);
      setOnly(null);
      search(intent.query, intent.github ?? false);
    }
    window.setTimeout(() => searchRef.current?.focus(), 50);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [intent?.seq]);

  // Whether a hit is in the catalog follows the catalog, as apps are added
  // or deleted after the search.
  const { data: shelf } = useShelf();
  const view = useMemo<Results>(() => {
    if (!shelf) return results;
    const keys = new Map(shelf.apps.map((a) => [sourceKey(a.app.source), a.app.id]));
    const fresh = (r: SourceResult): SourceResult => ({
      ...r,
      hits: r.hits.map((h) => ({ ...h, inShelf: keys.get(sourceKey(sourceOf(h))) })),
    });
    return {
      winget: fresh(results.winget),
      scoop: fresh(results.scoop),
      github: fresh(results.github),
    };
  }, [results, shelf]);
  // With all sources shown, an app in winget and scoop is one row, of the
  // winget key; the source to add is chosen on it.
  const merged = only === null;
  const twins = useMemo(() => pairTwins(view.winget.hits, view.scoop.hits), [view]);
  const sections = useMemo(() => {
    const paired = new Set([...twins.values()].map(keyOf));
    return kinds
      .filter((k) => (only ? only === k : true))
      .map((k) => ({
        kind: k,
        hits:
          merged && k === "scoop"
            ? view.scoop.hits.filter((h) => !paired.has(keyOf(h)))
            : view[k].hits,
      }));
  }, [merged, only, twins, view]);
  /** twinPick is how a row with a twin chooses its source, by the row's key. */
  const twinPick = (key: string): TwinPick | undefined => {
    const row = merged ? view.winget.hits.find((h) => keyOf(h) === key) : undefined;
    const twin = row && twins.get(key);
    if (!row || !twin) return undefined;
    return {
      pick: pickOf(row, twin, picks),
      onPick: (k) => setPicks((p) => ({ ...p, [key]: k })),
    };
  };
  const rows = useMemo(() => sections.flatMap((s) => s.hits), [sections]);
  const byKey = useMemo(
    () =>
      new Map(
        rows.map((h) => {
          const twin = merged ? twins.get(keyOf(h)) : undefined;
          return [keyOf(h), twin && pickOf(h, twin, picks) === "scoop" ? twin : h] as const;
        }),
      ),
    [rows, merged, twins, picks],
  );
  const allCount = kinds.reduce((n, k) => n + view[k].hits.length, 0) - twins.size;
  const order = useMemo(() => rows.map(keyOf), [rows]);
  const list = useList(order, false);
  const focused = list.focus ? byKey.get(list.focus) : undefined;
  const selected = [...list.selected].map((k) => byKey.get(k)).filter((h): h is SearchHit => !!h);
  const addable = selected.filter((h) => !h.inShelf);
  useEffect(() => revealRow(listRef, list.focus), [list.focus]);

  const quick = useQuickAdd((k) => !!byKey.get(k)?.inShelf);
  const candidate = (h: SearchHit): AddCandidate => ({
    name: h.name,
    source: sourceOf(h),
    icon: h.icon,
    ...(find ? { detect: { displayName: find.rule }, from: find.name } : {}),
  });
  /** add adds the rows not in the catalog, as each shows its source. */
  const add = (keys: string[], install: boolean) => {
    const items = keys.flatMap((key) => {
      const h = byKey.get(key);
      return h && !h.inShelf ? [{ key, cand: candidate(h) }] : [];
    });
    void quick.add(items, install);
  };

  usePageKeys({
    focusSearch: () => searchRef.current?.select(),
    selectAll: () => list.selectAll(true),
    escape: () => {
      if (list.selected.size > 0) {
        list.selectAll(false);
        return true;
      }
      if (list.focus) {
        list.closeDetails();
        return true;
      }
      return false;
    },
    move: (d) => list.move(d),
    space: () => {
      if (list.focus) list.check(list.focus, !list.selected.has(list.focus), false);
    },
    enter: () => {
      if (!list.focus || !focused) return false;
      if (focused.inShelf) nav.showInShelf([focused.inShelf]);
      else add([list.focus], false);
      return true;
    },
  });

  const loading = kinds.some((k) => results[k].state === "loading");
  const anyDone = kinds.some((k) => results[k].state !== "idle");
  const githubRate = sources?.githubSearchRate;
  const layout = useDefaultLayout({
    id: "discover",
    storage: localStorage,
    panelIds: focused ? ["side", "list", "detail"] : ["side", "list"],
  });

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
              <SideRow
                icon={LayersIcon}
                label="全部结果"
                count={allCount}
                active={only === null}
                onClick={() => setOnly(null)}
              />
              <SideGroup title="来源">
                {kinds.map((k) => (
                  <SourceRow
                    key={k}
                    kind={k}
                    result={results[k]}
                    active={only === k}
                    onClick={() => setOnly(only === k ? null : k)}
                  />
                ))}
              </SideGroup>
            </div>
          </ScrollArea>
          <SettingsFooter />
        </div>
      </ResizablePanel>
      <ResizableHandle />
      <ResizablePanel id="list" minSize={520}>
        <div className="glass-panel glass-strong @container flex h-full min-h-0 flex-col">
          <div className="flex h-[60px] shrink-0 items-center gap-2.5 border-b px-3">
            <AddTargetChip />
            <SearchBox
              value={query}
              onChange={setQuery}
              onEnter={() => search(query, true)}
              placeholder="搜索软件，回车同时搜索 GitHub"
              inputRef={searchRef}
              size="lg"
              busy={loading}
              className="mx-auto w-full max-w-md min-w-48"
            />
            <div className="flex shrink-0 items-center gap-1.5">
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="outline"
                    size="sm"
                    aria-label="搜 GitHub"
                    disabled={!searchable(query) || results.github.state === "loading"}
                    onClick={() => searchGitHub(query)}
                  >
                    <GitHubMark className="size-3.5" />
                    <WideLabel from="xl">搜 GitHub</WideLabel>
                  </Button>
                </TooltipTrigger>
                <TooltipContent>
                  {githubRate?.known
                    ? `GitHub 搜索剩余 ${githubRate.remaining} / ${githubRate.limit} 次${formatDateTime(githubRate.reset) ? `，${formatDateTime(githubRate.reset)} 重置` : ""}`
                    : "未设置 token 时每分钟最多 10 次"}
                </TooltipContent>
              </Tooltip>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    size="sm"
                    aria-label="手动添加"
                    onClick={() => actions.edit(undefined, { name: query.trim() })}
                  >
                    <PencilLineIcon />
                    <WideLabel>手动添加</WideLabel>
                  </Button>
                </TooltipTrigger>
                <TooltipContent>手动添加</TooltipContent>
              </Tooltip>
            </div>
          </div>
          {find && (
            <div className="anim-fade flex shrink-0 items-center gap-2.5 border-b bg-primary/[0.07] px-4 py-2 text-sm">
              <MonitorCheckIcon className="size-4 shrink-0 text-primary" />
              <span className="min-w-0 truncate">
                正在为本机的「<span className="font-medium">{find.name}</span>
                」查找来源，加入时带上它的检测规则
              </span>
              <Button variant="ghost" size="xs" className="ml-auto" onClick={() => setFind(null)}>
                <XIcon />
                取消
              </Button>
            </div>
          )}
          {!anyDone ? (
            <EmptyState
              icon={SearchIcon}
              title="搜索软件"
              hint="winget 和 scoop 一起搜索，搜不到再搜 GitHub"
            >
              {examples.map((e) => (
                <Button
                  key={e}
                  variant="outline"
                  size="sm"
                  className="rounded-full"
                  onClick={() => {
                    setQuery(e);
                    search(e, false);
                  }}
                >
                  {e}
                </Button>
              ))}
            </EmptyState>
          ) : rows.length === 0 && loading ? (
            <ResultSkeleton />
          ) : rows.length === 0 ? (
            <EmptyState icon={SearchXIcon} title={`没有找到「${searched}」`}>
              {results.github.state === "idle" && (
                <Button onClick={() => searchGitHub(searched)}>
                  <GitHubMark className="size-3.5" />
                  搜 GitHub
                </Button>
              )}
              <Button variant="outline" onClick={() => actions.edit(undefined, { name: searched })}>
                <PencilLineIcon />
                手动添加
              </Button>
            </EmptyState>
          ) : (
            <ScrollArea className="min-h-0 flex-1" viewportRef={listRef}>
              <div className="list-stagger pb-20">
                {sections.map(({ kind: k, hits }) =>
                  hits.length === 0 ? null : (
                    <section key={k}>
                      <div className="sticky top-0 z-10 flex h-8 items-center gap-2 border-b bg-sticky px-4 text-xs font-medium text-muted-foreground backdrop-blur-md">
                        <SourceIcon type={k} />
                        {sourceLabel(k)}
                        <span className="tabular-nums">{hits.length}</span>
                        {merged && k === "scoop" && twins.size > 0 && (
                          <span className="flex items-center gap-1 font-normal">
                            <MergeIcon className="size-3" />
                            另有 {twins.size} 个与 winget 同名，已合并
                          </span>
                        )}
                        {results[k].state === "loading" && (
                          <span className="text-primary">更新中</span>
                        )}
                      </div>
                      {hits.map((row) => {
                        const key = keyOf(row);
                        const h = byKey.get(key) ?? row;
                        return (
                          <HitRow
                            key={key}
                            rowKey={key}
                            hit={h}
                            twin={twinPick(key)}
                            checked={list.selected.has(key)}
                            focused={list.focus === key}
                            selecting={list.selected.size > 0}
                            onClick={(e) => list.clickRow(key, e)}
                            onCheck={(on, shift) => list.check(key, on, shift)}
                            adding={quick.adding.has(key)}
                            addHint={quick.hint}
                            onAdd={() => add([key], false)}
                            onShow={() => h.inShelf && nav.showInShelf([h.inShelf])}
                          />
                        );
                      })}
                    </section>
                  ),
                )}
                <div className="flex items-center justify-center gap-2 py-5 text-sm text-muted-foreground">
                  查不到？
                  {results.github.state === "idle" && (
                    <Button variant="outline" size="sm" onClick={() => searchGitHub(searched)}>
                      <GitHubMark className="size-3.5" />
                      搜 GitHub
                    </Button>
                  )}
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => actions.edit(undefined, { name: searched })}
                  >
                    <PencilLineIcon />
                    手动添加
                  </Button>
                </div>
              </div>
            </ScrollArea>
          )}
          <SelectionBar count={selected.length} onClear={() => list.selectAll(false)}>
            <Button
              size="sm"
              disabled={addable.length === 0}
              onClick={() => add([...list.selected], false)}
            >
              <PlusIcon />
              加入清单
            </Button>
            <Button
              size="sm"
              variant="secondary"
              disabled={addable.length === 0}
              onClick={() => add([...list.selected], true)}
            >
              <DownloadIcon />
              加入并安装
            </Button>
          </SelectionBar>
        </div>
      </ResizablePanel>
      {focused && list.focus && (
        <>
          <ResizableHandle />
          <ResizablePanel
            id="detail"
            defaultSize={380}
            minSize={300}
            maxSize={600}
            groupResizeBehavior="preserve-pixel-size"
          >
            <HitDetail
              hit={focused}
              twin={twinPick(list.focus)}
              onClose={list.closeDetails}
              adding={quick.adding.has(list.focus)}
              addHint={quick.hint}
              onAdd={(install) => list.focus && add([list.focus], install)}
              onShow={() => focused.inShelf && nav.showInShelf([focused.inShelf])}
            />
          </ResizablePanel>
        </>
      )}
    </ResizablePanelGroup>
  );
}

/** SourceRow filters the results to one source, and tells how its search went. */
function SourceRow({
  kind,
  result,
  active,
  onClick,
}: {
  kind: SourceKind;
  result: SourceResult;
  active: boolean;
  onClick: () => void;
}) {
  const label = sourceLabel(kind);
  const problem = result.state === "error" || result.state === "unavailable";
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <div className="flex flex-col">
          <SideRow
            icon={({ className }) => <SourceIcon type={kind} className={cn("size-4", className)} />}
            label={label}
            count={result.hits.length}
            trailing={
              result.state === "loading" ? (
                <span className="size-3 animate-spin rounded-full border-[1.5px] border-current border-t-transparent" />
              ) : problem ? (
                <TriangleAlertIcon className="size-3.5 text-warning" />
              ) : undefined
            }
            active={active}
            onClick={onClick}
          />
        </div>
      </TooltipTrigger>
      <TooltipContent side="right" className="max-w-80">
        {problem
          ? `${label}：${result.error ?? "不可用"}`
          : result.state === "idle" && kind === "github"
            ? "GitHub 只在点「搜 GitHub」或回车时搜索"
            : `${label} ${result.hits.length} 个结果`}
      </TooltipContent>
    </Tooltip>
  );
}

/** TwinPick chooses the source of a result found in winget and scoop. */
interface TwinPick {
  pick: SourceKind;
  onPick: (k: SourceKind) => void;
}

/** TwinSwitch is the choice of winget or scoop, for a result in both. */
function TwinSwitch({ twin, className }: { twin: TwinPick; className?: string }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          role="radiogroup"
          aria-label="选择来源"
          className={cn("seg-track inline-flex shrink-0 rounded-md p-0.5", className)}
          onClick={(e) => e.stopPropagation()}
        >
          {(["winget", "scoop"] as const).map((k) => (
            <button
              key={k}
              type="button"
              role="radio"
              aria-checked={twin.pick === k}
              onClick={() => twin.onPick(k)}
              className={cn(
                "flex h-5 items-center gap-1 rounded-[5px] px-1.5 text-[11px] font-medium transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring/50",
                twin.pick === k
                  ? "seg-thumb text-foreground"
                  : "text-muted-foreground hover:text-foreground",
              )}
            >
              <SourceIcon type={k} className="size-3" />
              {sourceLabel(k)}
            </button>
          ))}
        </span>
      </TooltipTrigger>
      <TooltipContent>winget 和 scoop 都有，选择从哪里安装</TooltipContent>
    </Tooltip>
  );
}

function HitRow({
  rowKey,
  hit: h,
  twin,
  checked,
  focused,
  selecting,
  adding,
  addHint,
  onClick,
  onCheck,
  onAdd,
  onShow,
}: {
  rowKey: string;
  hit: SearchHit;
  twin?: TwinPick;
  checked: boolean;
  focused: boolean;
  selecting: boolean;
  adding: boolean;
  addHint: string;
  onClick: (e: MouseEvent) => void;
  onCheck: (on: boolean, shift: boolean) => void;
  onAdd: () => void;
  onShow: () => void;
}) {
  return (
    <div
      data-row={rowKey}
      onClick={onClick}
      data-checked={checked}
      className={cn(
        "row-lazy list-row group/row relative flex h-[60px] items-center gap-3 px-3 select-none",
        focused && "row-focus",
      )}
    >
      <Checkbox
        checked={checked}
        aria-label={`选择 ${h.name}`}
        onClick={(e) => {
          e.preventDefault();
          e.stopPropagation();
          onCheck(!checked, e.shiftKey);
        }}
        className={cn(
          "transition-opacity",
          checked || selecting
            ? "opacity-100"
            : "opacity-0 group-hover/row:opacity-100 focus-visible:opacity-100",
        )}
      />
      <AppIcon path={h.icon} name={h.name} size={36} />
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 items-baseline gap-2">
          <span className="truncate text-sm font-medium">{h.name}</span>
          {h.version && (
            <span className="shrink-0 text-xs text-muted-foreground tabular-nums">{h.version}</span>
          )}
        </div>
        <div className="mt-1 flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
          {twin && <TwinSwitch twin={twin} />}
          <span className="max-w-[45%] shrink-0 truncate font-mono">{h.id}</span>
          {h.summary && <span className="min-w-0 truncate">{h.summary}</span>}
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-1">
        {h.stars !== undefined && h.stars > 0 && (
          <Pill tone="muted">
            <StarIcon />
            {formatStars(h.stars)}
          </Pill>
        )}
        {h.archived && (
          <Pill tone="warning">
            <ArchiveIcon />
            已归档
          </Pill>
        )}
        {h.installed && (
          <Pill tone="success">
            <MonitorCheckIcon />
            本机已装
          </Pill>
        )}
      </div>
      <div className="flex w-[104px] shrink-0 justify-end">
        {h.inShelf ? (
          <InShelfButton onClick={onShow} />
        ) : (
          <AddButton
            adding={adding}
            hint={h.type === "github" ? "确认检测规则后加入" : addHint}
            onClick={onAdd}
          />
        )}
      </div>
    </div>
  );
}

/** hitRef is what to read a hit's details by, in its header and below. */
function hitRef(h: SearchHit): PackageRef {
  return {
    type: h.type,
    id: h.id,
    ...(h.type === "winget" && h.version ? { version: h.version } : {}),
  };
}

function HitDetail({
  hit: h,
  twin,
  adding,
  addHint,
  onClose,
  onAdd,
  onShow,
}: {
  hit: SearchHit;
  twin?: TwinPick;
  adding: boolean;
  addHint: string;
  onClose: () => void;
  onAdd: (install: boolean) => void;
  onShow: () => void;
}) {
  return (
    <DetailPanel
      key={keyOf(h)}
      onClose={onClose}
      icon={<AppIcon path={h.icon} name={h.name} size={40} />}
      title={h.name}
      subtitle={
        <>
          <span className="truncate font-mono select-text">{h.id}</span>
          <CopyButton text={h.id} what="ID" />
          <SourceLink pkgRef={hitRef(h)} />
        </>
      }
      badges={
        <>
          {twin ? <TwinSwitch twin={twin} /> : <SourceBadge type={h.type} />}
          {h.installed && (
            <Pill tone="success">
              <MonitorCheckIcon />
              本机已装
            </Pill>
          )}
          {h.inShelf && (
            <Pill tone="primary">
              <CheckIcon />
              已在清单
            </Pill>
          )}
        </>
      }
      actions={
        h.inShelf ? (
          <Button size="sm" variant="outline" onClick={onShow}>
            <ArrowRightIcon />
            在清单中查看
          </Button>
        ) : (
          <>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button size="sm" disabled={adding} onClick={() => onAdd(false)}>
                  {adding ? <LoaderCircleIcon className="animate-spin" /> : <PlusIcon />}
                  加入清单
                </Button>
              </TooltipTrigger>
              <TooltipContent>
                {h.type === "github" ? "确认检测规则后加入" : addHint}
              </TooltipContent>
            </Tooltip>
            <Button size="sm" variant="outline" disabled={adding} onClick={() => onAdd(true)}>
              <DownloadIcon />
              加入并安装
            </Button>
          </>
        )
      }
    >
      <PackageInfo pkgRef={hitRef(h)} />
    </DetailPanel>
  );
}

function ResultSkeleton() {
  return (
    <div className="flex flex-1 flex-col">
      <div className="flex h-8 items-center border-b px-4">
        <Skeleton className="h-3 w-20" />
      </div>
      {Array.from({ length: 8 }, (_, i) => (
        <div
          key={i}
          className="list-row pointer-events-none relative flex h-[60px] items-center gap-3 px-3"
          style={{ opacity: 1 - i * 0.1 }}
        >
          <Skeleton className="size-4 rounded" />
          <Skeleton className="size-9 rounded-[26%]" />
          <div className="flex flex-1 flex-col gap-1.5">
            <Skeleton className="h-3.5 w-44" />
            <Skeleton className="h-3 w-80" />
          </div>
          <Skeleton className="h-6 w-14 rounded-md" />
        </div>
      ))}
    </div>
  );
}
