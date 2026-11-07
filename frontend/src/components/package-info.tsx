// What a package's source says of it: a summary, facts, links, the long
// description in a tab of its own. For display only: failing to read it
// changes nothing else.

import { DetailTabs, type DetailTab } from "@/components/detail-panel";
import { Pill } from "@/components/status-badge";
import { Skeleton } from "@/components/ui/skeleton";
import { errorText } from "@/lib/api";
import { formatDateTime, formatStars } from "@/lib/text";
import { cn } from "@/lib/utils";
import { About, Discover, type PackageDetails, type PackageRef } from "@/mygo";
import { useQuery } from "@tanstack/react-query";
import {
  ArchiveIcon,
  ArrowUpRightIcon,
  BookOpenTextIcon,
  Building2Icon,
  CalendarClockIcon,
  CopyrightIcon,
  FileArchiveIcon,
  GlobeIcon,
  LayoutDashboardIcon,
  PackageIcon,
  RocketIcon,
  ScaleIcon,
  ScrollTextIcon,
  StarIcon,
  TagIcon,
  TriangleAlertIcon,
  type LucideIcon,
} from "lucide-react";
import { useState, type ReactNode } from "react";
import { toast } from "sonner";

/** usePackageDetails reads a package's details, once a session. */
export function usePackageDetails(ref: PackageRef | null) {
  return useQuery({
    queryKey: ["details", ref?.type ?? "", ref?.id.toLowerCase() ?? "", ref?.version ?? ""],
    queryFn: () => Discover.details(ref as PackageRef),
    enabled: ref !== null && ref.id !== "",
    retry: false,
  });
}

/** openURL opens a web page in the browser. */
export function openURL(url: string): void {
  About.openURL(url).catch((err: unknown) =>
    toast.error("无法打开链接", { description: errorText(err) }),
  );
}

function hostOf(url: string): string {
  try {
    return new URL(url).host.replace(/^www\./, "");
  } catch {
    return url;
  }
}

const isURL = (s: string) => /^https?:\/\//i.test(s.trim());

/** Fact is a tile of the overview: a named value. */
export interface Fact {
  label: string;
  icon: LucideIcon;
  value: ReactNode;
  /** The value's whole text, on hover, when it may be cut. */
  title?: string;
  mono?: boolean;
  /** Across both columns, after the others. */
  wide?: boolean;
  tone?: "primary" | "warning";
}

/** FactGrid sets facts in two columns; an odd one out spans both. */
function FactGrid({ facts, loading = 0 }: { facts: Fact[]; loading?: number }) {
  const narrow = facts.filter((f) => !f.wide);
  const cells = [
    ...narrow.map((f, i) => ({
      f,
      span: loading === 0 && i === narrow.length - 1 && narrow.length % 2 === 1,
    })),
    ...facts.filter((f) => f.wide).map((f) => ({ f, span: true })),
  ];
  if (cells.length === 0 && loading === 0) return null;
  return (
    <div className="grid grid-cols-2 gap-2">
      {cells.map(({ f, span }, i) => {
        const Icon = f.icon;
        return (
          <div
            key={f.label}
            className={cn(
              "anim-rise flex min-w-0 flex-col gap-1 rounded-xl border bg-card px-3 py-2.5 shadow-xs transition-colors hover:border-primary/25",
              span && "col-span-2",
            )}
            style={{ animationDelay: `${i * 35}ms` }}
          >
            <span className="flex items-center gap-1.5 text-[11.5px] text-muted-foreground">
              <Icon className="size-3.5 shrink-0 text-primary/70" />
              {f.label}
            </span>
            <span
              className={cn(
                "truncate text-[13.5px] font-medium select-text",
                f.mono && "font-mono text-[13px] tabular-nums",
                f.tone === "primary" && "text-primary",
                f.tone === "warning" && "text-warning",
              )}
              title={f.title}
            >
              {f.value}
            </span>
          </div>
        );
      })}
      {Array.from({ length: loading }, (_, i) => (
        <div key={i} className="flex flex-col gap-2 rounded-xl border bg-card px-3 py-3">
          <Skeleton className="h-3 w-14" />
          <Skeleton className="h-3.5 w-24" />
        </div>
      ))}
    </div>
  );
}

interface LinkItem {
  label: string;
  icon: LucideIcon;
  url: string;
}

/** LinkChips is a row of the package's pages, each opened in the browser. */
function LinkChips({ links }: { links: LinkItem[] }) {
  if (links.length === 0) return null;
  return (
    <div className="flex flex-wrap gap-1.5">
      {links.map(({ label, icon: Icon, url }) => (
        <button
          key={url}
          type="button"
          onClick={() => openURL(url)}
          title={url}
          className="group/link inline-flex h-7 max-w-full items-center gap-1.5 rounded-full border bg-card px-3 text-xs font-medium shadow-xs transition-[color,border-color,transform] duration-200 hover:-translate-y-px hover:border-primary/40 hover:text-primary active:translate-y-0"
        >
          <Icon className="size-3.5 shrink-0 text-primary/70" />
          <span className="truncate">{label}</span>
          <ArrowUpRightIcon className="size-3 shrink-0 opacity-40 transition-[opacity,transform] group-hover/link:translate-x-px group-hover/link:-translate-y-px group-hover/link:opacity-100" />
        </button>
      ))}
    </div>
  );
}

/** Tags shows the first tags, the rest on asking. */
function Tags({ tags }: { tags: string[] }) {
  const [all, setAll] = useState(false);
  const shown = all ? tags : tags.slice(0, 8);
  return (
    <div className="flex flex-wrap gap-1.5">
      {shown.map((t) => (
        <span
          key={t}
          className="anim-fade inline-flex h-6 items-center gap-1 rounded-full bg-secondary px-2.5 text-xs text-secondary-foreground"
        >
          <TagIcon className="size-3 opacity-60" />
          {t}
        </span>
      ))}
      {tags.length > shown.length && (
        <button
          type="button"
          onClick={() => setAll(true)}
          className="inline-flex h-6 items-center rounded-full border border-dashed px-2.5 text-xs text-muted-foreground transition-colors hover:border-primary/40 hover:text-primary"
        >
          +{tags.length - shown.length}
        </button>
      )}
    </div>
  );
}

/** Prose sets a long text: paragraphs, and lists where lines are bulleted. */
function Prose({ text }: { text: string }) {
  const bullet = /^\s*(?:[-*•·●▪]|\d+[.)、])\s+/;
  const blocks: Array<{ list: boolean; lines: string[] }> = [];
  for (const para of text.split(/\n\s*\n/)) {
    // A blank line ends a block; within a paragraph, a change between
    // bulleted and plain lines does.
    let block: { list: boolean; lines: string[] } | null = null;
    for (const line of para.split("\n")) {
      if (!line.trim()) continue;
      const list = bullet.test(line);
      if (!block || block.list !== list) {
        block = { list, lines: [] };
        blocks.push(block);
      }
      block.lines.push(list ? line.replace(bullet, "") : line);
    }
  }
  return (
    <div className="flex flex-col gap-3 text-[13.5px] leading-[1.8] break-words text-foreground/85 select-text">
      {blocks.map((b, i) =>
        b.list ? (
          <ul key={i} className="flex flex-col gap-1.5">
            {b.lines.map((l, j) => (
              <li key={j} className="flex gap-2.5">
                <span className="mt-[0.7em] size-1.5 shrink-0 rounded-full bg-primary/60" />
                <span className="min-w-0">{l.trim()}</span>
              </li>
            ))}
          </ul>
        ) : (
          <p key={i} className="whitespace-pre-line">
            {b.lines.join("\n").trim()}
          </p>
        ),
      )}
    </div>
  );
}

function Card({ children }: { children: ReactNode }) {
  return <div className="anim-rise rounded-xl border bg-card p-4 shadow-xs">{children}</div>;
}

function facts(d: PackageDetails, latest?: string): Fact[] {
  const out: Fact[] = [];
  const version = latest || d.version;
  if (version) {
    out.push({
      label: "最新版本",
      icon: PackageIcon,
      value: version,
      title: version,
      mono: true,
      tone: "primary",
    });
  }
  const published = formatDateTime(d.release?.published);
  if (published) out.push({ label: "发布时间", icon: CalendarClockIcon, value: published });
  if (d.publisher) {
    out.push({ label: "发布者", icon: Building2Icon, value: d.publisher, title: d.publisher });
  }
  if (d.license) {
    out.push({ label: "许可证", icon: ScaleIcon, value: d.license, title: d.license });
  }
  if (d.stars !== undefined) {
    out.push({ label: "星标", icon: StarIcon, value: formatStars(d.stars) });
  }
  if (d.release?.asset) {
    out.push({
      label: "将安装的文件",
      icon: FileArchiveIcon,
      value: d.release.asset,
      title: d.release.asset,
      mono: true,
      wide: true,
    });
  } else if (d.release?.assetError) {
    out.push({
      label: "将安装的文件",
      icon: FileArchiveIcon,
      value: d.release.assetError,
      title: d.release.assetError,
      wide: true,
      tone: "warning",
    });
  }
  return out;
}

function links(d: PackageDetails): LinkItem[] {
  const out: LinkItem[] = [];
  const add = (label: string, icon: LucideIcon, url?: string) => {
    if (url && isURL(url) && !out.some((l) => l.url === url)) out.push({ label, icon, url });
  };
  add(d.homepage ? hostOf(d.homepage) : "", GlobeIcon, d.homepage);
  add("发布者", Building2Icon, d.publisherUrl);
  add("许可证", ScaleIcon, d.licenseUrl);
  add("发行页", RocketIcon, d.release?.url);
  add("更新说明", ScrollTextIcon, d.releaseNotes);
  return out;
}

/**
 * PackageInfo shows a package's details in tabs: an overview of the facts
 * first, the long description in its own. The caller's overview, facts and
 * tabs join the source's.
 */
export function PackageInfo({
  pkgRef,
  overview,
  facts: own = [],
  latest,
  tabs: extra = [],
}: {
  pkgRef: PackageRef | null;
  /** What the caller knows, first in the overview. */
  overview?: ReactNode;
  /** The caller's facts, before the source's. */
  facts?: Fact[];
  /** The latest version the caller knows, over the source's. */
  latest?: string;
  /** The caller's tabs, after the source's. */
  tabs?: DetailTab[];
}) {
  const q = usePackageDetails(pkgRef);
  const loading = q.isPending && pkgRef !== null;
  const overviewTab = (content: ReactNode): DetailTab => ({
    value: "overview",
    label: "概览",
    icon: LayoutDashboardIcon,
    content: (
      <div className="flex flex-col gap-3">
        {overview}
        {content}
      </div>
    ),
  });

  if (loading) {
    const tab = overviewTab(
      <>
        <div className="flex flex-col gap-2 rounded-xl border bg-card p-4">
          <Skeleton className="h-4 w-4/5" />
          <Skeleton className="h-4 w-3/5" />
        </div>
        <FactGrid facts={own} loading={own.length % 2 === 0 ? 4 : 3} />
        <div className="flex gap-1.5">
          {[20, 16, 18].map((w) => (
            <Skeleton key={w} className="h-7 rounded-full" style={{ width: `${w * 4}px` }} />
          ))}
        </div>
      </>,
    );
    return <DetailTabs tabs={[tab, ...extra]} pending={pkgRef?.type === "winget" ? 1 : 0} />;
  }

  if (q.isError) {
    const tab = overviewTab(
      <>
        <FactGrid facts={own} />
        <div className="flex items-start gap-2.5 rounded-xl border border-warning/25 bg-warning/[0.07] px-3.5 py-3 text-[13px] text-muted-foreground">
          <TriangleAlertIcon className="mt-px size-4 shrink-0 text-warning" />
          <span className="break-words">详情读取失败：{errorText(q.error)}</span>
        </div>
      </>,
    );
    return <DetailTabs tabs={[tab, ...extra]} />;
  }

  const d: PackageDetails = q.data ?? {};
  const intro = d.description && d.description !== d.summary ? d.description : "";
  const notes = d.releaseNotes && !isURL(d.releaseNotes) ? d.releaseNotes : "";
  const lead = d.summary || intro;
  const tabs: DetailTab[] = [
    overviewTab(
      <>
        {(lead || d.archived) && (
          <div className="anim-rise flex flex-col gap-2.5 rounded-xl bg-linear-to-br from-primary/[0.07] to-primary/[0.02] p-4 ring-1 ring-primary/10 ring-inset dark:from-primary/[0.12] dark:to-primary/[0.04]">
            {lead && (
              <p
                className={cn(
                  "text-[14.5px] leading-relaxed font-medium text-foreground select-text",
                  !d.summary && "line-clamp-3",
                )}
              >
                {lead}
              </p>
            )}
            {d.archived && (
              <Pill tone="muted" className="w-fit">
                <ArchiveIcon />
                已归档
              </Pill>
            )}
          </div>
        )}
        <FactGrid
          facts={[...own, ...facts(d, latest).filter((f) => !own.some((o) => o.label === f.label))]}
        />
        <LinkChips links={links(d)} />
        {d.tags && d.tags.length > 0 && <Tags tags={d.tags} />}
        {d.copyright && (
          <p className="flex items-start gap-1.5 px-0.5 text-[11.5px] leading-relaxed text-muted-foreground">
            <CopyrightIcon className="mt-[3px] size-3 shrink-0" />
            <span className="line-clamp-2 select-text">
              {d.copyright.replace(/^(©|\(c\)|copyright)\s*/i, "")}
            </span>
          </p>
        )}
      </>,
    ),
  ];
  if (intro || notes) {
    tabs.push({
      value: "intro",
      label: "介绍",
      icon: BookOpenTextIcon,
      content: (
        <div className="flex flex-col gap-3">
          {intro && (
            <Card>
              <Prose text={intro} />
            </Card>
          )}
          {notes && (
            <Card>
              <h4 className="mb-2 flex items-center gap-2 text-[13px] font-semibold">
                <ScrollTextIcon className="size-3.5 text-primary" />
                更新说明
              </h4>
              <Prose text={notes} />
            </Card>
          )}
        </div>
      ),
    });
  }
  return <DetailTabs tabs={[...tabs, ...extra]} />;
}
