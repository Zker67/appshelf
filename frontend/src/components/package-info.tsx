// What a package's source says of it: a summary, links, license, release.
// For display only: failing to read it changes nothing else.

import { InfoCard, InfoRow } from "@/components/common";
import { Pill } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { errorText } from "@/lib/api";
import { formatDateTime, formatStars } from "@/lib/text";
import { cn } from "@/lib/utils";
import { About, Discover, type PackageDetails, type PackageRef } from "@/mygo";
import { useQuery } from "@tanstack/react-query";
import {
  ArchiveIcon,
  Building2Icon,
  ChevronDownIcon,
  CopyrightIcon,
  ExternalLinkIcon,
  FileArchiveIcon,
  FileTextIcon,
  GlobeIcon,
  PackageIcon,
  RocketIcon,
  ScaleIcon,
  ScrollTextIcon,
  StarIcon,
  TagIcon,
  TagsIcon,
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

function Link({ url, children }: { url: string; children: ReactNode }) {
  return (
    <button
      type="button"
      onClick={() => openURL(url)}
      className="group -mx-1.5 inline-flex max-w-full items-center gap-1 rounded-md px-1.5 py-0.5 text-left text-primary transition-colors hover:bg-primary/10"
      title={url}
    >
      <span className="truncate">{children}</span>
      <ExternalLinkIcon className="size-3 shrink-0 opacity-50 transition-[opacity,transform] group-hover:translate-x-0.5 group-hover:-translate-y-0.5 group-hover:opacity-100" />
    </button>
  );
}

function hostOf(url: string): string {
  try {
    return new URL(url).host.replace(/^www\./, "");
  } catch {
    return url;
  }
}

/** Version is a version number set in a chip. */
export function Version({ children }: { children: ReactNode }) {
  return (
    <span className="inline-flex h-6 max-w-full items-center rounded-md bg-primary/10 px-2 font-mono text-[12.5px] font-medium text-primary tabular-nums ring-1 ring-primary/15 ring-inset">
      <span className="truncate">{children}</span>
    </span>
  );
}

/** LongText is a long text folded to a few lines, fading out, until expanded. */
function LongText({ text, lines = 6 }: { text: string; lines?: number }) {
  const [open, setOpen] = useState(false);
  const long = text.length > 280 || text.split("\n").length > lines;
  const folded = !open && long;
  return (
    <div className="flex flex-col items-start gap-2">
      <p
        className={cn(
          "text-[13.5px] leading-[1.75] break-words whitespace-pre-line text-foreground/80 select-text",
          folded && "line-clamp-6 [mask-image:linear-gradient(to_bottom,black_55%,transparent)]",
        )}
        style={folded ? { WebkitLineClamp: lines } : undefined}
      >
        {text}
      </p>
      {long && (
        <Button
          variant="secondary"
          size="xs"
          className="rounded-full px-2.5 text-primary"
          onClick={() => setOpen((o) => !o)}
        >
          {open ? "收起" : "展开全部"}
          <ChevronDownIcon
            className={cn("transition-transform duration-300", open && "rotate-180")}
          />
        </Button>
      )}
    </div>
  );
}

/** Section is a titled card of the details. */
function Section({
  title,
  icon: Icon,
  children,
}: {
  title: string;
  icon: LucideIcon;
  children: ReactNode;
}) {
  return (
    <section className="anim-rise flex flex-col gap-2.5 rounded-xl border bg-card p-4 shadow-xs">
      <h4 className="flex items-center gap-2 text-[13px] font-semibold">
        <span className="flex size-6 items-center justify-center rounded-md bg-primary/10 text-primary">
          <Icon className="size-3.5" />
        </span>
        {title}
      </h4>
      {children}
    </section>
  );
}

function InfoSkeleton({ before }: { before?: ReactNode }) {
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-2 rounded-xl border bg-card p-4">
        <Skeleton className="h-4 w-4/5" />
        <Skeleton className="h-4 w-3/5" />
      </div>
      <InfoCard>
        {before}
        {[0, 1, 2, 3].map((i) => (
          <div key={i} className="flex gap-3 py-3">
            <Skeleton className="h-3.5 w-20" />
            <Skeleton className="h-3.5 flex-1" />
          </div>
        ))}
      </InfoCard>
      <Skeleton className="h-28 w-full rounded-xl" />
    </div>
  );
}

/**
 * PackageInfo shows a package's details: the facts first, the long texts
 * after.
 */
export function PackageInfo({ pkgRef, before }: { pkgRef: PackageRef | null; before?: ReactNode }) {
  const q = usePackageDetails(pkgRef);
  if (q.isPending && pkgRef) {
    return <InfoSkeleton before={before} />;
  }
  if (q.isError) {
    return (
      <div className="flex flex-col gap-4">
        {before && <InfoCard>{before}</InfoCard>}
        <div className="flex items-start gap-2.5 rounded-xl border border-warning/25 bg-warning/[0.07] px-3.5 py-3 text-[13px] text-muted-foreground">
          <TriangleAlertIcon className="mt-px size-4 shrink-0 text-warning" />
          <span className="break-words">详情读取失败：{errorText(q.error)}</span>
        </div>
      </div>
    );
  }
  const d: PackageDetails = q.data ?? {};
  const hasFacts = Boolean(
    before || d.version || d.publisher || d.homepage || d.license || d.copyright || d.release,
  );
  return (
    <div className="flex flex-col gap-4">
      {(d.summary || d.stars !== undefined || d.archived) && (
        <div className="anim-rise flex flex-col gap-3 rounded-xl bg-linear-to-br from-primary/[0.07] to-primary/[0.02] p-4 ring-1 ring-primary/10 ring-inset dark:from-primary/[0.12] dark:to-primary/[0.04]">
          {d.summary && (
            <p className="text-[14.5px] leading-relaxed font-medium text-foreground select-text">
              {d.summary}
            </p>
          )}
          {(d.stars !== undefined || d.archived) && (
            <div className="flex items-center gap-1.5">
              {d.stars !== undefined && (
                <Pill tone="warning">
                  <StarIcon className="fill-current" />
                  {formatStars(d.stars)}
                </Pill>
              )}
              {d.archived && (
                <Pill tone="muted">
                  <ArchiveIcon />
                  已归档
                </Pill>
              )}
            </div>
          )}
        </div>
      )}
      {hasFacts && (
        <InfoCard className="[animation-delay:40ms]">
          {before}
          {d.version && (
            <InfoRow label="最新版本" icon={PackageIcon}>
              <Version>{d.version}</Version>
            </InfoRow>
          )}
          {d.publisher && (
            <InfoRow label="发布者" icon={Building2Icon}>
              {d.publisherUrl ? <Link url={d.publisherUrl}>{d.publisher}</Link> : d.publisher}
            </InfoRow>
          )}
          {d.homepage && (
            <InfoRow label="主页" icon={GlobeIcon}>
              <Link url={d.homepage}>{hostOf(d.homepage)}</Link>
            </InfoRow>
          )}
          {d.license && (
            <InfoRow label="许可证" icon={ScaleIcon}>
              {d.licenseUrl ? <Link url={d.licenseUrl}>{d.license}</Link> : d.license}
            </InfoRow>
          )}
          {d.copyright && (
            <InfoRow label="版权" icon={CopyrightIcon}>
              <span className="text-foreground/80">{d.copyright}</span>
            </InfoRow>
          )}
          {d.release && (
            <InfoRow label="最新发行" icon={RocketIcon}>
              <div className="flex flex-col gap-1.5">
                <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
                  {d.release.url ? (
                    <Link url={d.release.url}>{d.release.tag}</Link>
                  ) : (
                    <Version>{d.release.tag}</Version>
                  )}
                  {formatDateTime(d.release.published) && (
                    <span className="text-xs text-muted-foreground tabular-nums">
                      {formatDateTime(d.release.published)}
                    </span>
                  )}
                </span>
                {d.release.asset ? (
                  <span className="flex w-fit max-w-full items-center gap-1.5 rounded-md bg-muted px-2 py-1 text-xs text-muted-foreground">
                    <FileArchiveIcon className="size-3.5 shrink-0" />
                    <span className="truncate font-mono">{d.release.asset}</span>
                  </span>
                ) : (
                  d.release.assetError && (
                    <span className="text-xs text-warning">{d.release.assetError}</span>
                  )
                )}
              </div>
            </InfoRow>
          )}
        </InfoCard>
      )}
      {d.tags && d.tags.length > 0 && (
        <Section title="标签" icon={TagsIcon}>
          <div className="flex flex-wrap gap-1.5">
            {d.tags.slice(0, 16).map((t) => (
              <span
                key={t}
                className="inline-flex h-6 items-center gap-1 rounded-full bg-secondary px-2.5 text-xs text-secondary-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
              >
                <TagIcon className="size-3 opacity-60" />
                {t}
              </span>
            ))}
          </div>
        </Section>
      )}
      {d.description && d.description !== d.summary && (
        <Section title="介绍" icon={FileTextIcon}>
          <LongText text={d.description} />
        </Section>
      )}
      {d.releaseNotes && (
        <Section title="更新说明" icon={ScrollTextIcon}>
          <LongText text={d.releaseNotes} lines={8} />
        </Section>
      )}
    </div>
  );
}
