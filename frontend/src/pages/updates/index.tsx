// Updates: everything AppShelf can update in one list, the catalog's apps
// and the installed winget and scoop packages, each once; AppShelf's own
// update on top. How each is updated is set here: tell me, on its own, or
// ignored; a version skipped or put off. Then what was updated, and how.

import logo from "@/assets/logo.png";
import { AppIcon } from "@/components/app-icon";
import { Chip, EmptyState, SearchBox, SelectionBar, WideLabel } from "@/components/common";
import { SettingsFooter, SideGroup, SideRow } from "@/components/side-nav";
import { SourceBadge } from "@/components/source-badge";
import { Pill } from "@/components/status-badge";
import { UpdateMenu, modeIcons } from "@/components/update-menu";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from "@/components/ui/resizable";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Skeleton } from "@/components/ui/skeleton";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { errorText, useUpdateHistory, useUpdateSettings, useUpdates } from "@/lib/api";
import { sourceID } from "@/lib/shelf";
import { formatClock, formatDateTime, matches } from "@/lib/text";
import { holdText } from "@/lib/updates";
import { revealRow, useList, usePageKeys } from "@/lib/use-list";
import { cn } from "@/lib/utils";
import {
  About,
  Updates,
  type CheckInterval,
  type UpdateItem,
  type UpdateRecord,
  type UpdateRule,
} from "@/mygo";
import { useActions } from "@/state/actions";
import {
  ArrowUpCircleIcon,
  BellOffIcon,
  CheckCircle2Icon,
  CirclePlayIcon,
  Clock3Icon,
  DownloadCloudIcon,
  HistoryIcon,
  LoaderCircleIcon,
  RefreshCwIcon,
  RotateCcwIcon,
  SearchXIcon,
  SparklesIcon,
  Trash2Icon,
  TriangleAlertIcon,
  XCircleIcon,
  ZapIcon,
  type LucideIcon,
} from "lucide-react";
import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type MouseEvent,
  type ReactNode,
  type Ref,
} from "react";
import { useDefaultLayout } from "react-resizable-panels";
import { toast } from "sonner";
import { UpdateDetail } from "./detail";

type View = "updates" | "auto" | "held" | "history";

const views: Array<{ value: View; label: string; icon: LucideIcon; tone: string }> = [
  { value: "updates", label: "可更新", icon: ArrowUpCircleIcon, tone: "text-primary" },
  { value: "auto", label: "自动更新", icon: ZapIcon, tone: "text-primary" },
  { value: "held", label: "已忽略", icon: BellOffIcon, tone: "text-muted-foreground" },
  { value: "history", label: "最近更新", icon: HistoryIcon, tone: "text-muted-foreground" },
];

const intervalLabels: Record<CheckInterval, string> = {
  off: "只在启动时检查",
  "1h": "每小时检查",
  "3h": "每 3 小时检查",
  "6h": "每 6 小时检查",
  "12h": "每 12 小时检查",
  "24h": "每天检查",
};

/** A held entry: an update held back, or a package ignored with none now. */
type Held = { item?: UpdateItem; rule?: UpdateRule; key: string; name: string };

export function UpdatesPage() {
  const { data: list, isPending } = useUpdates();
  const { data: history } = useUpdateHistory();
  const actions = useActions();
  const [view, setView] = useState<View>("updates");
  const [query, setQuery] = useState("");

  const items = useMemo(() => list?.items ?? [], [list]);
  const rules = useMemo(() => list?.rules ?? [], [list]);
  const offered = useMemo(() => items.filter((i) => !i.hold), [items]);
  const held = useMemo<Held[]>(() => {
    const out: Held[] = items
      .filter((i) => i.hold)
      .map((i) => ({ item: i, key: i.key, name: i.name }));
    const shown = new Set(out.map((h) => h.key));
    for (const r of rules) {
      if (r.mode === "ignore" && !shown.has(r.key)) out.push({ rule: r, key: r.key, name: r.name });
    }
    return out;
  }, [items, rules]);
  const auto = useMemo(() => rules.filter((r) => r.mode === "auto"), [rules]);
  const byKey = useMemo(() => new Map(items.map((i) => [i.key, i])), [items]);
  const records = history ?? [];

  const counts: Record<View, number> = {
    updates: offered.length + (list?.self.available ? 1 : 0),
    auto: auto.length,
    held: held.length,
    history: records.length,
  };
  const hit = (name: string, id: string) => matches(query, [name, id]);
  const shownUpdates = useMemo(
    () => offered.filter((i) => matches(query, [i.name, sourceID(i.source)])),
    [offered, query],
  );
  const shownAuto = auto.filter((r) => hit(r.name, sourceID(r.source)));
  const shownHeld = held.filter((h) => hit(h.name, h.key));
  const shownHistory = records.filter((r) => hit(r.name, r.key));
  const runnable = offered.filter((i) => !i.pending);
  const searchRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const order = useMemo(() => shownUpdates.map((i) => i.key), [shownUpdates]);
  const rows = useList(order, false);
  const focused = view === "updates" && rows.focus ? byKey.get(rows.focus) : undefined;
  const selected = [...rows.selected]
    .map((k) => byKey.get(k))
    .filter((i): i is UpdateItem => !!i && !i.pending);
  useEffect(() => revealRow(listRef, rows.focus), [rows.focus]);

  usePageKeys({
    focusSearch: () => searchRef.current?.focus(),
    selectAll: () => {
      if (view === "updates") rows.selectAll(true);
    },
    escape: () => {
      if (rows.selected.size > 0) {
        rows.selectAll(false);
        return true;
      }
      return false;
    },
    move: (d) => {
      if (view === "updates") rows.move(d);
    },
    space: () => {
      if (view === "updates" && rows.focus)
        rows.check(rows.focus, !rows.selected.has(rows.focus), false);
    },
    enter: () => {
      const i = view === "updates" && rows.focus ? byKey.get(rows.focus) : undefined;
      if (!i || i.pending) return false;
      actions.update([i.key]);
      return true;
    },
  });
  const loading = isPending || (!list?.loaded && items.length === 0);

  const check = () =>
    Updates.check().catch((err: unknown) =>
      toast.error("检查失败", { description: errorText(err) }),
    );
  const layout = useDefaultLayout({
    id: "updates",
    storage: localStorage,
    panelIds: focused ? ["side", "list", "detail"] : ["side", "list"],
  });
  const checkedAt = formatClock(list?.checkedAt);
  const nextAt = formatDateTime(list?.nextCheck);

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
              <SideGroup title="更新">
                {views.map((v) => (
                  <SideRow
                    key={v.value}
                    icon={v.icon}
                    iconClass={v.tone}
                    label={v.label}
                    count={counts[v.value]}
                    active={view === v.value}
                    onClick={() => setView(v.value)}
                  />
                ))}
              </SideGroup>
              <Schedule />
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
              <Tooltip>
                <TooltipTrigger asChild>
                  <span>
                    <Chip icon={list?.checking ? LoaderCircleIcon : Clock3Icon}>
                      {list?.checking ? "正在检查" : checkedAt ? `${checkedAt} 检查` : "尚未检查"}
                    </Chip>
                  </span>
                </TooltipTrigger>
                <TooltipContent>
                  {nextAt ? `下次检查：${nextAt}` : "自动检查已关闭，只在启动时和手动检查"}
                </TooltipContent>
              </Tooltip>
            </div>
            <SearchBox
              value={query}
              onChange={setQuery}
              placeholder="搜索软件名称"
              inputRef={searchRef}
              className="mx-auto w-full max-w-sm min-w-40"
            />
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="outline"
                  size="sm"
                  className="shrink-0"
                  aria-label="检查更新"
                  disabled={list?.checking ?? true}
                  onClick={() => void check()}
                >
                  <RefreshCwIcon className={list?.checking ? "animate-spin" : undefined} />
                  <WideLabel>{list?.checking ? "检查中" : "检查更新"}</WideLabel>
                </Button>
              </TooltipTrigger>
              <TooltipContent>检查更新</TooltipContent>
            </Tooltip>
            {view === "updates" && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    size="sm"
                    className="shrink-0"
                    aria-label="全部更新"
                    disabled={runnable.length === 0}
                    onClick={() => actions.update(runnable.map((i) => i.key))}
                  >
                    <ArrowUpCircleIcon />
                    <WideLabel from="xl">全部更新</WideLabel>
                    {runnable.length > 0 && ` ${runnable.length}`}
                  </Button>
                </TooltipTrigger>
                <TooltipContent>全部更新</TooltipContent>
              </Tooltip>
            )}
            {view === "history" && <ClearHistory disabled={records.length === 0} />}
          </div>
          {loading ? (
            <ListSkeleton />
          ) : view === "updates" ? (
            shownUpdates.length === 0 && !list?.self.available ? (
              query.trim() ? (
                <NoMatch query={query} onClear={() => setQuery("")} />
              ) : (
                <EmptyState
                  icon={CheckCircle2Icon}
                  title="都是最新版本"
                  hint={checkedAt ? `上次检查于 ${checkedAt}` : undefined}
                >
                  <Button variant="outline" onClick={() => void check()} disabled={list?.checking}>
                    <RefreshCwIcon className={list?.checking ? "animate-spin" : undefined} />
                    检查更新
                  </Button>
                </EmptyState>
              )
            ) : (
              <Rows viewportRef={listRef}>
                {list?.self.available && !query.trim() && (
                  <SelfCard version={list.self.version} available={list.self.available} />
                )}
                {shownUpdates.map((i) => (
                  <UpdateRow
                    key={i.key}
                    item={i}
                    checked={rows.selected.has(i.key)}
                    focused={rows.focus === i.key}
                    selecting={rows.selected.size > 0}
                    onClick={(ev) => rows.clickRow(i.key, ev)}
                    onCheck={(on, shift) => rows.check(i.key, on, shift)}
                    onUpdate={() => actions.update([i.key])}
                  />
                ))}
              </Rows>
            )
          ) : view === "auto" ? (
            shownAuto.length === 0 ? (
              query.trim() ? (
                <NoMatch query={query} onClear={() => setQuery("")} />
              ) : (
                <EmptyState
                  icon={ZapIcon}
                  title="还没有自动更新的软件"
                  hint="在可更新的软件右侧打开更新方式，选择「自动更新」"
                >
                  <Button variant="outline" onClick={() => setView("updates")}>
                    <ArrowUpCircleIcon />
                    查看可更新
                  </Button>
                </EmptyState>
              )
            ) : (
              <Rows>
                {shownAuto.map((r) => (
                  <AutoRow key={r.key} rule={r} item={byKey.get(r.key)} />
                ))}
              </Rows>
            )
          ) : view === "held" ? (
            shownHeld.length === 0 ? (
              query.trim() ? (
                <NoMatch query={query} onClear={() => setQuery("")} />
              ) : (
                <EmptyState
                  icon={BellOffIcon}
                  title="没有忽略的更新"
                  hint="跳过、暂缓或忽略的更新会列在这里，可以随时恢复"
                />
              )
            ) : (
              <Rows>
                {shownHeld.map((h) => (
                  <HeldRow key={h.key} held={h} />
                ))}
              </Rows>
            )
          ) : shownHistory.length === 0 ? (
            query.trim() ? (
              <NoMatch query={query} onClear={() => setQuery("")} />
            ) : (
              <EmptyState
                icon={HistoryIcon}
                title="还没有更新记录"
                hint="更新过的软件会记在这里，包括自动更新的"
              />
            )
          ) : (
            <Rows>
              {shownHistory.map((r, i) => (
                <HistoryRow key={`${r.key}-${r.at}-${i}`} record={r} />
              ))}
            </Rows>
          )}
          <SelectionBar
            count={view === "updates" ? rows.selected.size : 0}
            onClear={() => rows.selectAll(false)}
          >
            <Button
              size="sm"
              disabled={selected.length === 0}
              onClick={() => actions.update(selected.map((i) => i.key))}
            >
              <ArrowUpCircleIcon />
              更新
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
            <UpdateDetail
              item={focused}
              onClose={rows.closeDetails}
              onUpdate={() => actions.update([focused.key])}
            />
          </ResizablePanel>
        </>
      )}
    </ResizablePanelGroup>
  );
}

function Rows({
  children,
  viewportRef,
}: {
  children: ReactNode;
  viewportRef?: Ref<HTMLDivElement>;
}) {
  return (
    <ScrollArea className="min-h-0 flex-1" viewportRef={viewportRef}>
      <div className="list-stagger anim-fade pb-20">{children}</div>
    </ScrollArea>
  );
}

function NoMatch({ query, onClear }: { query: string; onClear: () => void }) {
  return (
    <EmptyState icon={SearchXIcon} title={`没有找到「${query.trim()}」`}>
      <Button variant="outline" onClick={onClear}>
        清除搜索
      </Button>
    </EmptyState>
  );
}

/** Schedule says when updates are checked, and opens their settings. */
function Schedule() {
  const { data: s } = useUpdateSettings();
  const actions = useActions();
  if (!s) return null;
  const parts = [intervalLabels[s.checkEvery], s.notify ? "发现后通知" : "不发通知"];
  if (s.background) parts.push("关闭窗口后留在托盘");
  return (
    <button
      type="button"
      onClick={() => actions.openSettings("updates")}
      className="group/sched mx-0.5 flex flex-col gap-1 rounded-xl border bg-card/60 px-3 py-2.5 text-left text-xs text-muted-foreground transition-colors hover:border-primary/30 hover:bg-card"
    >
      <span className="flex items-center gap-1.5 font-medium text-foreground">
        <Clock3Icon className="size-3.5 text-primary transition-transform duration-300 group-hover/sched:rotate-[-20deg]" />
        自动检查
      </span>
      <span className="leading-relaxed">{parts.join(" · ")}</span>
      <span className="text-primary opacity-0 transition-opacity group-hover/sched:opacity-100">
        更改设置 →
      </span>
    </button>
  );
}

/** SelfCard is AppShelf's own update, which its update window installs. */
function SelfCard({ version, available }: { version: string; available: string }) {
  const [busy, setBusy] = useState(false);
  const open = async () => {
    setBusy(true);
    try {
      await About.checkForUpdates();
    } catch (err) {
      toast.error("无法打开更新窗口", { description: errorText(err) });
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="mx-3 mt-3 mb-1 flex items-center gap-3 rounded-xl border border-primary/20 bg-linear-to-r from-primary/10 via-card to-card px-3.5 py-3">
      <img src={logo} alt="" className="size-10 drop-shadow-sm" draggable={false} />
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2 text-sm font-semibold">
          软件图书馆有新版本
          <Pill tone="primary">
            <SparklesIcon />v{available.replace(/^v/, "")}
          </Pill>
        </div>
        <div className="mt-0.5 text-xs text-muted-foreground">
          当前 v{version.replace(/^v/, "")}，更新说明和安装在更新窗口里
        </div>
      </div>
      <Button size="sm" onClick={() => void open()} disabled={busy}>
        {busy ? <LoaderCircleIcon className="animate-spin" /> : <DownloadCloudIcon />}
        查看并更新
      </Button>
    </div>
  );
}

/** Versions shows "1.8 → 1.9". */
function Versions({ from, to }: { from?: string; to?: string }) {
  return (
    <span className="shrink-0 text-xs text-muted-foreground tabular-nums select-text">
      {from || "?"} → <span className="text-primary">{to || "?"}</span>
    </span>
  );
}

/** RowPick makes a row one of a list's selection, checked on its own or by a click. */
interface RowPick {
  id: string;
  checked: boolean;
  disabled?: boolean;
  focused: boolean;
  selecting: boolean;
  onClick: (ev: MouseEvent) => void;
  onCheck: (on: boolean, shift: boolean) => void;
}

function RowShell({
  icon,
  name,
  children,
  sub,
  pick,
}: {
  icon: string;
  name: string;
  sub: ReactNode;
  children: ReactNode;
  pick?: RowPick;
}) {
  return (
    <div
      data-row={pick?.id}
      data-checked={pick?.checked}
      onClick={pick?.onClick}
      className={cn(
        "row-lazy list-row group/row relative flex h-[60px] items-center gap-3 px-3",
        pick && "select-none",
        pick?.focused && "row-focus",
      )}
    >
      {pick && (
        <Checkbox
          checked={pick.checked}
          disabled={pick.disabled}
          aria-label={`选择 ${name}`}
          onClick={(ev) => {
            ev.preventDefault();
            ev.stopPropagation();
            pick.onCheck(!pick.checked, ev.shiftKey);
          }}
          className={cn(
            "transition-opacity",
            pick.checked || pick.selecting
              ? "opacity-100"
              : "opacity-0 group-hover/row:opacity-100 focus-visible:opacity-100",
          )}
        />
      )}
      <AppIcon path={icon} name={name} size={36} />
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-medium select-text">{name}</div>
        <div className="mt-1 flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground">
          {sub}
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-1.5">{children}</div>
    </div>
  );
}

function UpdateRow({
  item: i,
  onUpdate,
  ...pick
}: Omit<RowPick, "id" | "disabled"> & { item: UpdateItem; onUpdate: () => void }) {
  return (
    <RowShell
      icon={i.icon}
      name={i.name}
      pick={{ ...pick, id: i.key, disabled: i.pending }}
      sub={
        <>
          <SourceBadge type={i.source.type} />
          <Versions from={i.version} to={i.latest} />
          {i.appId && <span className="truncate">已在清单</span>}
        </>
      }
    >
      {i.major && (
        <Tooltip>
          <TooltipTrigger asChild>
            <span>
              <Pill tone="warning">
                <SparklesIcon />
                大版本
              </Pill>
            </span>
          </TooltipTrigger>
          <TooltipContent className="max-w-72">
            主版本号变了，界面或用法可能有较大变化，可以先看看更新说明
          </TooltipContent>
        </Tooltip>
      )}
      {i.running && (
        <Tooltip>
          <TooltipTrigger asChild>
            <span>
              <Pill tone="warning">
                <CirclePlayIcon />
                正在运行
              </Pill>
            </span>
          </TooltipTrigger>
          <TooltipContent className="max-w-72">
            {i.mode === "auto"
              ? "自动更新会等它关闭后再进行"
              : "更新时它可能被关闭，请先保存正在编辑的内容"}
          </TooltipContent>
        </Tooltip>
      )}
      {i.pending && (
        <Pill tone="muted">
          <LoaderCircleIcon className="animate-spin" />
          任务中
        </Pill>
      )}
      <UpdateMenu
        target={{
          key: i.key,
          name: i.name,
          mode: i.mode,
          hold: i.hold,
          latest: i.latest,
          canAuto: i.canAuto,
          autoNote: i.autoNote,
        }}
      />
      <Button
        size="xs"
        variant="outline"
        disabled={i.pending}
        className="transition-colors hover:border-primary/40 hover:text-primary"
        onClick={(ev) => {
          ev.stopPropagation();
          onUpdate();
        }}
      >
        <ArrowUpCircleIcon />
        更新
      </Button>
    </RowShell>
  );
}

function AutoRow({ rule: r, item }: { rule: UpdateRule; item?: UpdateItem }) {
  let state: {
    tone: "primary" | "success" | "warning" | "muted";
    icon: LucideIcon;
    label: string;
    tip?: string;
  };
  if (!r.canAuto)
    state = { tone: "warning", icon: TriangleAlertIcon, label: "无法自动更新", tip: r.autoNote };
  else if (item?.pending) state = { tone: "muted", icon: LoaderCircleIcon, label: "更新中" };
  else if (item?.running)
    state = {
      tone: "warning",
      icon: CirclePlayIcon,
      label: "等它关闭",
      tip: "它正在运行，下次检查时再自动更新",
    };
  else if (item && !item.hold)
    state = {
      tone: "primary",
      icon: ArrowUpCircleIcon,
      label: `待更新 ${item.latest}`,
      tip: "下次检查时自动安装",
    };
  else state = { tone: "success", icon: CheckCircle2Icon, label: "已是最新" };
  const Icon = state.icon;
  const pill = (
    <Pill tone={state.tone}>
      <Icon className={state.icon === LoaderCircleIcon ? "animate-spin" : undefined} />
      {state.label}
    </Pill>
  );
  return (
    <RowShell
      icon={r.icon}
      name={r.name}
      sub={
        <>
          <SourceBadge type={r.source.type} />
          <span className="truncate font-mono">{sourceID(r.source)}</span>
        </>
      }
    >
      {state.tip ? (
        <Tooltip>
          <TooltipTrigger asChild>
            <span>{pill}</span>
          </TooltipTrigger>
          <TooltipContent className="max-w-72">{state.tip}</TooltipContent>
        </Tooltip>
      ) : (
        pill
      )}
      <UpdateMenu
        target={{
          key: r.key,
          name: r.name,
          mode: r.mode,
          hold: item?.hold,
          latest: item?.latest,
          canAuto: r.canAuto,
          autoNote: r.autoNote,
        }}
      />
    </RowShell>
  );
}

function HeldRow({ held: h }: { held: Held }) {
  const i = h.item;
  const r = h.rule;
  const icon = i?.icon ?? r?.icon ?? "";
  const source = i?.source ?? r?.source;
  const mode = i?.mode ?? r?.mode ?? "ignore";
  const ModeIcon = modeIcons[mode];
  const restore = async () => {
    try {
      await Updates.restore(h.key);
      toast.success(`已恢复提醒「${h.name}」的更新`);
    } catch (err) {
      toast.error("没有改成", { description: errorText(err) });
    }
  };
  return (
    <RowShell
      icon={icon}
      name={h.name}
      sub={
        <>
          {source && <SourceBadge type={source.type} />}
          {i ? <Versions from={i.version} to={i.latest} /> : <span>现在没有可用更新</span>}
        </>
      }
    >
      <Pill tone="muted">
        <ModeIcon />
        {holdText(i?.hold, { skip: i?.skip, until: i?.until, mode })}
      </Pill>
      <UpdateMenu
        target={{
          key: h.key,
          name: h.name,
          mode,
          hold: i?.hold,
          latest: i?.latest,
          canAuto: i?.canAuto ?? r?.canAuto,
          autoNote: i?.autoNote ?? r?.autoNote,
        }}
      />
      <Button size="xs" variant="outline" onClick={() => void restore()}>
        <RotateCcwIcon />
        恢复提醒
      </Button>
    </RowShell>
  );
}

const results: Record<
  string,
  { tone: "success" | "muted" | "danger"; icon: LucideIcon; label: string }
> = {
  succeeded: { tone: "success", icon: CheckCircle2Icon, label: "已更新" },
  skipped: { tone: "muted", icon: RotateCcwIcon, label: "跳过" },
  failed: { tone: "danger", icon: XCircleIcon, label: "失败" },
};

function HistoryRow({ record: r }: { record: UpdateRecord }) {
  const res = results[r.result] ?? results.skipped!;
  const Icon = res.icon;
  return (
    <RowShell
      icon={r.icon}
      name={r.name}
      sub={
        <>
          {(r.from || r.to) && <Versions from={r.from} to={r.to} />}
          {r.message && (
            <span className="min-w-0 truncate" title={r.message}>
              {r.message}
            </span>
          )}
        </>
      }
    >
      {r.auto && (
        <Pill tone="primary">
          <ZapIcon />
          自动
        </Pill>
      )}
      <Pill tone={res.tone}>
        <Icon />
        {res.label}
      </Pill>
      <span className="w-32 text-right text-xs text-muted-foreground tabular-nums">
        {formatDateTime(r.at)}
      </span>
    </RowShell>
  );
}

function ClearHistory({ disabled }: { disabled: boolean }) {
  const [open, setOpen] = useState(false);
  const clear = async () => {
    try {
      await Updates.clearHistory();
      toast.success("已清除更新记录");
    } catch (err) {
      toast.error("清除失败", { description: errorText(err) });
    }
  };
  return (
    <>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="outline"
            size="sm"
            className="shrink-0"
            aria-label="清除记录"
            disabled={disabled}
            onClick={() => setOpen(true)}
          >
            <Trash2Icon />
            <WideLabel>清除记录</WideLabel>
          </Button>
        </TooltipTrigger>
        <TooltipContent>清除记录</TooltipContent>
      </Tooltip>
      <AlertDialog open={open} onOpenChange={setOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>清除全部更新记录？</AlertDialogTitle>
            <AlertDialogDescription>
              只删除这里的记录，不影响已安装的软件和更新设置。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction onClick={() => void clear()}>清除</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}

function ListSkeleton() {
  return (
    <div className="flex flex-1 flex-col">
      {Array.from({ length: 9 }, (_, i) => (
        <div
          key={i}
          className="list-row pointer-events-none relative flex h-[60px] items-center gap-3 px-3"
          style={{ opacity: 1 - i * 0.09 }}
        >
          <Skeleton className="size-9 rounded-[26%]" />
          <div className="flex flex-1 flex-col gap-1.5">
            <Skeleton className="h-3.5 w-44" />
            <Skeleton className="h-3 w-60" />
          </div>
          <Skeleton className="h-6 w-14 rounded-md" />
          <Skeleton className="h-6 w-16 rounded-md" />
        </div>
      ))}
    </div>
  );
}
