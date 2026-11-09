// What the details of a program of this machine show beside its source's:
// the release notes of its update, the room it takes and where, and how it
// is removed. Shared by the installed, updates and uninstall pages; for
// display only, failing to read any changes nothing else.

import { CopyButton, InfoCard, InfoRow } from "@/components/common";
import { MarkdownView } from "@/components/markdown";
import { FactGrid, openURL, type Fact } from "@/components/package-info";
import { Pill } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { errorText } from "@/lib/api";
import { formatBytes } from "@/lib/text";
import { localDate } from "@/lib/uninstall";
import { cn } from "@/lib/utils";
import {
  Discover,
  Installed,
  type LocationFrom,
  type PackageRef,
  type ReleaseNote,
  type SpaceInfo,
} from "@/mygo";
import { useQuery } from "@tanstack/react-query";
import {
  AppWindowIcon,
  ArrowRightIcon,
  ArrowUpRightIcon,
  BanIcon,
  ChevronDownIcon,
  DatabaseIcon,
  FileTextIcon,
  FolderIcon,
  FolderOpenIcon,
  HardDriveIcon,
  InfoIcon,
  PackageIcon,
  RefreshCwIcon,
  ScrollTextIcon,
  ShieldCheckIcon,
  ShieldQuestionIcon,
  SigmaIcon,
  TerminalIcon,
  TriangleAlertIcon,
  WrenchIcon,
  ZapIcon,
} from "lucide-react";
import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { toast } from "sonner";

/** Notice is a line of what to know, in a tone. */
export function Notice({
  tone = "muted",
  children,
}: {
  tone?: "muted" | "warning";
  children: ReactNode;
}) {
  const Icon = tone === "warning" ? TriangleAlertIcon : InfoIcon;
  return (
    <div
      className={cn(
        "anim-rise flex items-start gap-2.5 rounded-xl border px-3.5 py-3 text-[13px] leading-relaxed text-muted-foreground",
        tone === "warning" ? "border-warning/25 bg-warning/[0.07]" : "bg-muted/40",
      )}
    >
      <Icon
        className={cn(
          "mt-0.5 size-4 shrink-0",
          tone === "warning" ? "text-warning" : "text-primary/70",
        )}
      />
      <div className="min-w-0 flex-1 break-words">{children}</div>
    </div>
  );
}

function PanelSkeleton({ lines = 3 }: { lines?: number }) {
  return (
    <div className="flex flex-col gap-3">
      {Array.from({ length: lines }, (_, i) => (
        <div key={i} className="flex flex-col gap-2 rounded-xl border bg-card p-4">
          <Skeleton className="h-3.5 w-24" />
          <Skeleton className="h-3 w-4/5" />
          <Skeleton className="h-3 w-3/5" />
        </div>
      ))}
    </div>
  );
}

function LinkButton({ url, children }: { url: string; children: ReactNode }) {
  return (
    <Button size="xs" variant="outline" onClick={() => openURL(url)} title={url}>
      {children}
      <ArrowUpRightIcon className="opacity-60" />
    </Button>
  );
}

/**
 * useSettled reports whether a value has stayed the same for a moment:
 * reading starts once the user stops moving through the rows, so that
 * passing a row neither measures its directories nor spends GitHub's quota.
 */
function useSettled(value: string, ms = 350): boolean {
  const [settled, setSettled] = useState<string | null>(null);
  useEffect(() => {
    const t = setTimeout(() => setSettled(value), ms);
    return () => clearTimeout(t);
  }, [value, ms]);
  return settled === value;
}

// ---- Release notes ----

/** useReleaseNotes reads what changed after from up to the ref's version. */
export function useReleaseNotes(ref: PackageRef | null, from: string) {
  const settled = useSettled(`${ref?.type}:${ref?.id}@${ref?.version}#${from}`);
  return useQuery({
    queryKey: [
      "release-notes",
      ref?.type ?? "",
      ref?.id.toLowerCase() ?? "",
      ref?.version ?? "",
      from,
    ],
    queryFn: () => Discover.releaseNotes(ref as PackageRef, from),
    enabled: settled && ref !== null && ref.id !== "",
    retry: false,
    staleTime: 10 * 60 * 1000,
  });
}

/** Versions is "1.8 → 1.9", large. */
function VersionStep({ from, to }: { from?: string; to?: string }) {
  return (
    <div className="flex min-w-0 items-center gap-2.5 font-medium tabular-nums select-text">
      <span className="truncate text-[15px] text-muted-foreground" title={from}>
        {from || "未知版本"}
      </span>
      <ArrowRightIcon className="size-4 shrink-0 text-primary/70" />
      <span className="truncate text-[17px] text-primary" title={to}>
        {to || "?"}
      </span>
    </div>
  );
}

/** NoteBody is a version's notes, folded when long. */
function NoteBody({ note, markdown }: { note: ReleaseNote; markdown: boolean }) {
  const box = useRef<HTMLDivElement>(null);
  const [long, setLong] = useState(false);
  const [open, setOpen] = useState(false);
  useLayoutEffect(() => {
    const el = box.current;
    if (el) setLong(el.scrollHeight > 320);
  }, [note.body]);
  if (!note.body) {
    return <p className="text-[13px] text-muted-foreground">这一版没有写更新说明。</p>;
  }
  return (
    <div>
      <div
        ref={box}
        className={cn(
          "relative overflow-hidden",
          long && !open && "max-h-72 mask-b-from-70% mask-b-to-100%",
        )}
      >
        {markdown ? (
          <MarkdownView source={note.body} />
        ) : (
          <p className="text-[13.5px] leading-relaxed whitespace-pre-line text-foreground/90 select-text">
            {note.body}
          </p>
        )}
      </div>
      {long && (
        <button
          type="button"
          onClick={() => setOpen(!open)}
          className="mt-1.5 inline-flex items-center gap-1 text-xs font-medium text-primary hover:underline"
        >
          {open ? "收起" : "展开全部"}
          <ChevronDownIcon
            className={cn("size-3.5 transition-transform duration-200", open && "rotate-180")}
          />
        </button>
      )}
    </div>
  );
}

/**
 * ReleaseNotesView shows the update from one version to another: the
 * versions, what to know first, then the notes of each version between,
 * newest first, with where they come from.
 */
export function ReleaseNotesView({
  pkgRef,
  from,
  to,
  lead,
}: {
  pkgRef: PackageRef | null;
  from?: string;
  to?: string;
  /** What to show under the versions, as how it updates. */
  lead?: ReactNode;
}) {
  const ref = pkgRef ? { ...pkgRef, version: to } : null;
  const q = useReleaseNotes(ref, from ?? "");
  const n = q.data;
  const source =
    n?.from === "github" && n.repo
      ? `来自 GitHub 仓库 ${n.repo} 的 release`
      : n?.from === "winget"
        ? "来自 winget 清单"
        : "";
  return (
    <div className="flex flex-col gap-3">
      <div className="anim-rise flex flex-col gap-2.5 rounded-xl bg-linear-to-br from-primary/[0.07] to-primary/[0.02] p-4 ring-1 ring-primary/10 ring-inset dark:from-primary/[0.12] dark:to-primary/[0.04]">
        <VersionStep from={from} to={to} />
        {lead}
      </div>
      {q.isPending && ref ? (
        <PanelSkeleton lines={2} />
      ) : q.isError ? (
        <Notice tone="warning">更新日志读取失败：{errorText(q.error)}</Notice>
      ) : !n || n.notes.length === 0 ? (
        <Notice>
          <div className="flex flex-col gap-2">
            <span>{n?.reason || "来源没有提供更新日志。"}</span>
            {n?.link && (
              <span>
                <LinkButton url={n.link}>
                  <ScrollTextIcon />
                  查看更新说明
                </LinkButton>
              </span>
            )}
          </div>
        </Notice>
      ) : (
        <>
          {n.notes.map((note, i) => (
            <div
              key={`${note.version}-${i}`}
              className="anim-rise rounded-xl border bg-card p-4 shadow-xs"
              style={{ animationDelay: `${i * 40}ms` }}
            >
              <div className="mb-2.5 flex min-w-0 items-center gap-2">
                <Pill tone={i === 0 ? "primary" : "muted"} className="font-mono">
                  {note.version}
                </Pill>
                {note.prerelease && <Pill tone="warning">预发布</Pill>}
                {note.title && note.title !== note.version && (
                  <span className="min-w-0 truncate text-[13px] font-medium" title={note.title}>
                    {note.title}
                  </span>
                )}
                <span className="ml-auto shrink-0 text-[11.5px] text-muted-foreground tabular-nums">
                  {localDate(note.published)}
                </span>
                {note.url && (
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Button
                        size="icon-xs"
                        variant="ghost"
                        className="-mr-1 text-muted-foreground"
                        aria-label="在浏览器中打开"
                        onClick={() => openURL(note.url as string)}
                      >
                        <ArrowUpRightIcon />
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent>在浏览器中打开</TooltipContent>
                  </Tooltip>
                )}
              </div>
              <NoteBody note={note} markdown={n.from === "github"} />
            </div>
          ))}
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5 px-0.5 text-[11.5px] text-muted-foreground">
            {source && <span>{source}</span>}
            {n.more && <span>还有更早的版本没有列出</span>}
            {n.link && (
              <button
                type="button"
                onClick={() => openURL(n.link as string)}
                className="inline-flex items-center gap-0.5 text-primary hover:underline"
              >
                全部更新说明
                <ArrowUpRightIcon className="size-3" />
              </button>
            )}
          </div>
        </>
      )}
    </div>
  );
}

// ---- Space ----

/** useSpace measures the room a program takes, once a while. */
export function useSpace(key: string) {
  const settled = useSettled(key);
  return useQuery({
    queryKey: ["space", key],
    queryFn: () => Installed.space(key),
    enabled: settled,
    retry: false,
    staleTime: 5 * 60 * 1000,
  });
}

/** openLocation opens a program's directory, or one of its data directories. */
export function openLocation(key: string, id = ""): void {
  Installed.openLocation(key, id).catch((err: unknown) =>
    toast.error("无法打开目录", { description: errorText(err) }),
  );
}

const fromText: Record<LocationFrom, string> = {
  registry: "注册表登记的安装目录",
  guessed: "由程序图标或卸载程序所在目录推断",
  scoop: "scoop 的应用目录，含保留的旧版本",
  store: "应用商店的应用包目录",
};

/** Share is a bar of the program's part and its data's. */
function Share({ program, data }: { program: number; data: number }) {
  const total = program + data;
  if (total <= 0) return null;
  const p = (program / total) * 100;
  return (
    <div className="flex h-2 w-full overflow-hidden rounded-full bg-track">
      <span
        className="h-full bg-primary/70 transition-[width] duration-500"
        style={{ width: `${p}%` }}
      />
      <span
        className="h-full bg-warning/60 transition-[width] duration-500"
        style={{ width: `${100 - p}%` }}
      />
    </div>
  );
}

function spaceFacts(s: SpaceInfo): Fact[] {
  const facts: Fact[] = [
    {
      label: "程序目录",
      icon: PackageIcon,
      value: s.measured ? formatBytes(s.program ?? 0) : "未统计",
      tone: s.measured ? "primary" : undefined,
    },
    {
      label: "可能相关的数据",
      icon: DatabaseIcon,
      value: s.data.length > 0 ? formatBytes(s.dataSize ?? 0) : "没有找到",
    },
  ];
  if (s.registered) {
    facts.push({ label: "注册表登记", icon: FileTextIcon, value: formatBytes(s.registered) });
  }
  return facts;
}

/**
 * SpaceView shows the room a program takes, measured: its directory, to
 * open, and the directories named as it that may hold its data.
 */
export function SpaceView({ entryKey }: { entryKey: string }) {
  const q = useSpace(entryKey);
  if (q.isPending) {
    return (
      <div className="flex flex-col gap-3">
        <p className="flex items-center gap-2 px-0.5 text-[13px] text-muted-foreground">
          <RefreshCwIcon className="size-3.5 animate-spin" />
          正在统计目录大小…
        </p>
        <PanelSkeleton lines={2} />
      </div>
    );
  }
  if (q.isError) {
    return (
      <div className="flex flex-col gap-3">
        <Notice tone="warning">统计失败：{errorText(q.error)}</Notice>
        <span>
          <Button size="sm" variant="outline" onClick={() => void q.refetch()}>
            <RefreshCwIcon />
            重新统计
          </Button>
        </span>
      </div>
    );
  }
  const s = q.data;
  const total = (s.measured ? (s.program ?? 0) : 0) + (s.dataSize ?? 0);
  return (
    <div className="flex flex-col gap-3">
      <div className="anim-rise flex flex-col gap-2.5 rounded-xl border bg-card p-4 shadow-xs">
        <div className="flex items-baseline gap-2">
          <SigmaIcon className="size-4 self-center text-primary/70" />
          <span className="text-[13px] text-muted-foreground">实际占用</span>
          <span className="ml-auto text-[20px] font-semibold tracking-tight tabular-nums">
            {total > 0 ? formatBytes(total) : "—"}
          </span>
        </div>
        <Share program={s.measured ? (s.program ?? 0) : 0} data={s.dataSize ?? 0} />
        <div className="flex items-center gap-3 text-[11.5px] text-muted-foreground">
          <span className="flex items-center gap-1.5">
            <span className="size-2 rounded-full bg-primary/70" />
            程序
          </span>
          <span className="flex items-center gap-1.5">
            <span className="size-2 rounded-full bg-warning/60" />
            可能相关的数据
          </span>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                size="icon-xs"
                variant="ghost"
                className="ml-auto text-muted-foreground"
                aria-label="重新统计"
                disabled={q.isFetching}
                onClick={() => void q.refetch()}
              >
                <RefreshCwIcon className={q.isFetching ? "animate-spin" : undefined} />
              </Button>
            </TooltipTrigger>
            <TooltipContent>重新统计</TooltipContent>
          </Tooltip>
        </div>
      </div>
      <FactGrid facts={spaceFacts(s)} />

      <InfoCard>
        <InfoRow label="安装位置" icon={FolderIcon}>
          {s.location ? (
            <div className="flex min-w-0 flex-col gap-1.5">
              <div className="flex min-w-0 items-start gap-1">
                <span className="min-w-0 font-mono text-[12.5px] break-all select-text">
                  {s.location}
                </span>
                <CopyButton text={s.location} what="路径" className="-mt-0.5 shrink-0" />
              </div>
              {s.from && (
                <span className="text-[11.5px] text-muted-foreground">{fromText[s.from]}</span>
              )}
              {s.note && <span className="text-[11.5px] text-warning">{s.note}</span>}
              {s.openable && (
                <span>
                  <Button size="xs" variant="outline" onClick={() => openLocation(entryKey)}>
                    <FolderOpenIcon />
                    打开位置
                  </Button>
                </span>
              )}
            </div>
          ) : (
            <span className="text-muted-foreground">{s.note || "不知道它装在哪个目录"}</span>
          )}
        </InfoRow>
      </InfoCard>

      <section className="anim-rise flex flex-col gap-2">
        <h4 className="flex items-center gap-2 px-0.5 text-[13px] font-semibold">
          <DatabaseIcon className="size-3.5 text-primary" />
          可能相关的数据
          <span className="font-normal text-muted-foreground tabular-nums">{s.data.length}</span>
        </h4>
        <p className="px-0.5 text-[11.5px] leading-relaxed text-muted-foreground">
          按名称推测的设置、缓存目录，仅供参考。卸载不会删除它们，卸载后可在「最近卸载」里查找残留再清理。
        </p>
        {s.data.length === 0 ? (
          <div className="rounded-xl border border-dashed px-3.5 py-3 text-[13px] text-muted-foreground">
            没有找到以它命名的数据目录
          </div>
        ) : (
          <div className="flex flex-col divide-y divide-border/70 rounded-xl border bg-card shadow-xs">
            {s.data.map((d) => (
              <div key={d.id} className="group/data flex items-center gap-2.5 px-3 py-2.5">
                <FolderIcon className="size-4 shrink-0 text-warning/80" />
                <div className="min-w-0 flex-1">
                  <div className="truncate font-mono text-[12px] select-text" title={d.path}>
                    {d.path}
                  </div>
                  <div className="mt-0.5 flex items-center gap-1.5 text-[11px] text-muted-foreground">
                    <Pill tone={d.level === "high" ? "warning" : "muted"}>
                      {d.level === "high" ? "很可能" : "可能"}
                    </Pill>
                    {d.reasons.length > 0 && (
                      <span className="truncate" title={d.reasons.join("；")}>
                        {d.reasons.join("；")}
                      </span>
                    )}
                  </div>
                </div>
                <span className="shrink-0 text-[13px] font-medium tabular-nums">
                  {formatBytes(d.size ?? 0)}
                </span>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      size="icon-xs"
                      variant="ghost"
                      className="text-muted-foreground"
                      aria-label="打开目录"
                      onClick={() => openLocation(entryKey, d.id)}
                    >
                      <FolderOpenIcon />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>打开目录</TooltipContent>
                </Tooltip>
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}

// ---- Removal ----

/** useRemoval reads how a program would be removed; nothing runs. */
export function useRemoval(key: string) {
  const settled = useSettled(key);
  return useQuery({
    queryKey: ["uninstall-preview", key],
    queryFn: async () => (await Installed.previewUninstall([key]))[0] ?? null,
    enabled: settled,
    retry: false,
    staleTime: 60 * 1000,
  });
}

/** RemovalView says how a program would be removed, and what to know first. */
export function RemovalView({ entryKey }: { entryKey: string }) {
  const q = useRemoval(entryKey);
  if (q.isPending) return <PanelSkeleton lines={1} />;
  if (q.isError) return <Notice tone="warning">读取失败：{errorText(q.error)}</Notice>;
  const p = q.data;
  if (!p) return <Notice>没有读到它的卸载方式</Notice>;
  const how = p.installer ? `${p.method} · ${p.installer}` : p.method;
  return (
    <div className="flex flex-col gap-3">
      {p.runnable ? (
        <div className="anim-rise flex items-center gap-2.5 rounded-xl border bg-card px-4 py-3 shadow-xs">
          {p.quiet ? (
            <ZapIcon className="size-4 text-primary" />
          ) : (
            <AppWindowIcon className="size-4 text-warning" />
          )}
          <span className="text-[13.5px] font-medium">
            {p.quiet ? "可以静默卸载" : "会打开它自己的卸载程序"}
          </span>
          <Pill tone={p.quiet ? "primary" : "warning"} className="ml-auto">
            {p.quiet ? "静默" : "需要操作"}
          </Pill>
        </div>
      ) : (
        <Notice tone="warning">
          <span className="flex items-center gap-1.5">
            <BanIcon className="size-3.5" />
            {p.plan}
          </span>
        </Notice>
      )}
      {p.notes.map((n) => (
        <Notice key={n} tone="warning">
          {n}
        </Notice>
      ))}
      <InfoCard>
        <InfoRow label="方式" icon={WrenchIcon}>
          {how}
        </InfoRow>
        {p.command && (
          <InfoRow label="命令" icon={TerminalIcon}>
            <div className="flex min-w-0 items-start gap-1">
              <span className="min-w-0 font-mono text-[12.5px] break-all select-text">
                {p.command}
              </span>
              <CopyButton text={p.command} what="命令" className="-mt-0.5 shrink-0" />
            </div>
          </InfoRow>
        )}
        <InfoRow label="签名" icon={p.signer && p.signed ? ShieldCheckIcon : ShieldQuestionIcon}>
          {p.signer ? (
            <span className={cn(!p.signed && "text-warning")}>
              {p.signer}
              {p.signed ? "" : "（签名无效）"}
            </span>
          ) : (
            <span className="text-muted-foreground">未签名或未知</span>
          )}
        </InfoRow>
        {p.size ? (
          <InfoRow label="登记大小" icon={HardDriveIcon}>
            <span className="tabular-nums">{formatBytes(p.size)}</span>
          </InfoRow>
        ) : null}
      </InfoCard>
      <p className="px-0.5 text-[11.5px] leading-relaxed text-muted-foreground">
        点「卸载」后还会再预览一次，确认后才执行；卸载前后都会复查它是否还在。
      </p>
    </div>
  );
}
