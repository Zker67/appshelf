// What a package's source says of it: a summary, links, license, release.
// For display only: failing to read it changes nothing else.

import { InfoRow } from "@/components/common";
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
  ChevronDownIcon,
  ExternalLinkIcon,
  FileArchiveIcon,
  StarIcon,
  TagIcon,
  TriangleAlertIcon,
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
      className="group inline-flex max-w-full items-center gap-1 text-left text-primary hover:underline"
      title={url}
    >
      <span className="truncate">{children}</span>
      <ExternalLinkIcon className="size-3 shrink-0 opacity-60 group-hover:opacity-100" />
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

/** LongText is a long text folded to a few lines until expanded. */
function LongText({ text, lines = 6 }: { text: string; lines?: number }) {
  const [open, setOpen] = useState(false);
  const long = text.length > 280 || text.split("\n").length > lines;
  return (
    <div className="flex flex-col items-start gap-1">
      <p
        className={cn(
          "text-sm leading-relaxed break-words whitespace-pre-line text-foreground/85 select-text",
          !open && long && "line-clamp-6",
        )}
        style={!open && long ? { WebkitLineClamp: lines } : undefined}
      >
        {text}
      </p>
      {long && (
        <Button variant="link" size="xs" className="h-5 px-0" onClick={() => setOpen((o) => !o)}>
          {open ? "收起" : "展开全部"}
          <ChevronDownIcon className={cn("transition-transform", open && "rotate-180")} />
        </Button>
      )}
    </div>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-1.5">
      <h4 className="text-xs font-medium text-muted-foreground">{title}</h4>
      {children}
    </section>
  );
}

/**
 * PackageInfo shows a package's details: the facts first, the long texts
 * after.
 */
export function PackageInfo({ pkgRef, before }: { pkgRef: PackageRef | null; before?: ReactNode }) {
  const q = usePackageDetails(pkgRef);
  if (q.isPending && pkgRef) {
    return (
      <div className="flex flex-col gap-3">
        {before}
        <Skeleton className="h-4 w-4/5" />
        <Skeleton className="h-4 w-3/5" />
        <div className="flex flex-col gap-2 pt-2">
          {[0, 1, 2, 3].map((i) => (
            <div key={i} className="flex gap-3">
              <Skeleton className="h-3.5 w-16" />
              <Skeleton className="h-3.5 flex-1" />
            </div>
          ))}
        </div>
        <Skeleton className="mt-2 h-20 w-full" />
      </div>
    );
  }
  if (q.isError) {
    return (
      <div className="flex flex-col gap-3">
        {before}
        <div className="flex items-start gap-2 rounded-lg bg-muted/60 px-3 py-2.5 text-xs text-muted-foreground">
          <TriangleAlertIcon className="mt-px size-3.5 shrink-0 text-warning" />
          <span className="break-words">详情读取失败：{errorText(q.error)}</span>
        </div>
      </div>
    );
  }
  const d: PackageDetails = q.data ?? {};
  return (
    <div className="flex flex-col gap-4">
      {d.summary && (
        <p className="text-sm leading-relaxed font-medium text-foreground/90 select-text">
          {d.summary}
        </p>
      )}
      {(d.stars !== undefined || d.archived) && (
        <div className="flex items-center gap-1.5">
          {d.stars !== undefined && (
            <Pill tone="muted">
              <StarIcon />
              {formatStars(d.stars)}
            </Pill>
          )}
          {d.archived && (
            <Pill tone="warning">
              <ArchiveIcon />
              已归档
            </Pill>
          )}
        </div>
      )}
      <div className="flex flex-col">
        {before}
        {d.version && <InfoRow label="最新版本">{d.version}</InfoRow>}
        {d.publisher && (
          <InfoRow label="发布者">
            {d.publisherUrl ? <Link url={d.publisherUrl}>{d.publisher}</Link> : d.publisher}
          </InfoRow>
        )}
        {d.homepage && (
          <InfoRow label="主页">
            <Link url={d.homepage}>{hostOf(d.homepage)}</Link>
          </InfoRow>
        )}
        {d.license && (
          <InfoRow label="许可证">
            {d.licenseUrl ? <Link url={d.licenseUrl}>{d.license}</Link> : d.license}
          </InfoRow>
        )}
        {d.copyright && <InfoRow label="版权">{d.copyright}</InfoRow>}
        {d.release && (
          <InfoRow label="最新发行">
            <div className="flex flex-col gap-1">
              <span className="flex flex-wrap items-center gap-x-2">
                {d.release.url ? <Link url={d.release.url}>{d.release.tag}</Link> : d.release.tag}
                {formatDateTime(d.release.published) && (
                  <span className="text-xs text-muted-foreground tabular-nums">
                    {formatDateTime(d.release.published)}
                  </span>
                )}
              </span>
              {d.release.asset ? (
                <span className="flex items-center gap-1 text-xs text-muted-foreground">
                  <FileArchiveIcon className="size-3 shrink-0" />
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
      </div>
      {d.tags && d.tags.length > 0 && (
        <div className="flex flex-wrap gap-1">
          {d.tags.slice(0, 16).map((t) => (
            <span
              key={t}
              className="inline-flex h-5 items-center gap-1 rounded-md bg-muted px-1.5 text-[11px] text-muted-foreground"
            >
              <TagIcon className="size-2.5" />
              {t}
            </span>
          ))}
        </div>
      )}
      {d.description && d.description !== d.summary && (
        <Section title="介绍">
          <LongText text={d.description} />
        </Section>
      )}
      {d.releaseNotes && (
        <Section title="更新说明">
          <LongText text={d.releaseNotes} lines={8} />
        </Section>
      )}
    </div>
  );
}
