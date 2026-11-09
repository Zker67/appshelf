// What removing programs would do, program by program, before it starts:
// how each is removed, the command, and what to know first.

import { AppIcon } from "@/components/app-icon";
import { SourceBadge } from "@/components/source-badge";
import { Pill } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Skeleton } from "@/components/ui/skeleton";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { errorText } from "@/lib/api";
import { formatBytes } from "@/lib/text";
import { cn } from "@/lib/utils";
import { Installed, type UninstallPreview } from "@/mygo";
import { useQuery } from "@tanstack/react-query";
import {
  BanIcon,
  BookMarkedIcon,
  ChevronDownIcon,
  FolderIcon,
  LoaderCircleIcon,
  ShieldCheckIcon,
  ShieldQuestionIcon,
  PackageXIcon,
  TriangleAlertIcon,
  type LucideIcon,
} from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

export interface UninstallRequest {
  /** keys are of the installed list. */
  keys: string[];
  seq: number;
  open: boolean;
}

export function UninstallDialog({
  request,
  onClose,
  onStarted,
}: {
  request: UninstallRequest | null;
  onClose: () => void;
  onStarted: () => void;
}) {
  const open = request?.open ?? false;
  const preview = useQuery({
    queryKey: ["installed", "uninstall", request?.seq ?? 0],
    queryFn: () => Installed.previewUninstall(request?.keys ?? []),
    enabled: open && !!request,
    // The installed list changes while it is open: keep what was read.
    staleTime: Infinity,
    refetchOnMount: false,
  });
  const entries = preview.data ?? [];
  const run = entries.filter((e) => e.runnable);
  const skipped = entries.filter((e) => !e.runnable);
  const windows = run.filter((e) => !e.quiet).length;
  const [busy, setBusy] = useState(false);

  const start = async () => {
    if (!request) return;
    setBusy(true);
    try {
      const n = await Installed.startUninstall(run.map((e) => e.key));
      toast.success(`已开始卸载 ${n} 个软件`);
      onClose();
      onStarted();
    } catch (err) {
      toast.error("无法开始卸载", { description: errorText(err) });
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="flex max-h-[min(760px,calc(100vh-6rem))] flex-col gap-0 p-0 sm:max-w-2xl">
        <DialogHeader className="border-b px-5 pt-5 pb-4">
          <DialogTitle className="flex items-center gap-2 text-lg">
            <PackageXIcon className="size-5 text-destructive" />
            卸载预览
          </DialogTitle>
          <DialogDescription className="flex flex-wrap items-center gap-1.5 pt-1">
            {preview.isPending ? (
              "正在核对每个软件的卸载方式"
            ) : (
              <>
                <Pill tone="danger">
                  <PackageXIcon />
                  卸载 {run.length}
                </Pill>
                {skipped.length > 0 && (
                  <Pill tone="muted">
                    <BanIcon />
                    不能卸载 {skipped.length}
                  </Pill>
                )}
                <span className="text-xs">卸载后软件仍留在清单里</span>
              </>
            )}
          </DialogDescription>
        </DialogHeader>
        <ScrollArea
          className="flex min-h-0 flex-1 flex-col"
          viewportClassName="h-auto min-h-0 flex-1"
        >
          <div className="flex flex-col gap-4 px-3 py-3">
            {preview.isPending ? (
              <div className="flex flex-col gap-1 px-2">
                {Array.from({ length: Math.min(request?.keys.length ?? 2, 5) }, (_, i) => (
                  <div key={i} className="flex items-center gap-3 py-2.5">
                    <Skeleton className="size-9 rounded-[26%]" />
                    <div className="flex flex-1 flex-col gap-1.5">
                      <Skeleton className="h-3.5 w-40" />
                      <Skeleton className="h-3 w-72" />
                    </div>
                  </div>
                ))}
              </div>
            ) : preview.isError ? (
              <div className="px-3 py-8 text-center text-sm text-destructive">
                {errorText(preview.error)}
              </div>
            ) : (
              <>
                <Group title="将卸载" entries={run} />
                <Group title="不能卸载" entries={skipped} />
              </>
            )}
          </div>
        </ScrollArea>
        <DialogFooter className="mx-0 mb-0 items-center rounded-b-xl px-5 py-3.5">
          <span className="mr-auto text-xs text-muted-foreground">
            {windows > 0
              ? `${windows} 个会打开卸载程序的窗口，需要你在窗口里操作`
              : "逐个执行，每个卸载后都会复查是否已移除"}
          </span>
          <Button variant="outline" onClick={onClose}>
            取消
          </Button>
          <Button variant="destructive" onClick={start} disabled={busy || run.length === 0}>
            {busy ? <LoaderCircleIcon className="animate-spin" /> : <PackageXIcon />}
            开始卸载
            {run.length > 0 && <span className="tabular-nums">（{run.length}）</span>}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function Group({ title, entries }: { title: string; entries: UninstallPreview[] }) {
  if (entries.length === 0) return null;
  return (
    <section className="anim-rise">
      <h3 className="px-2 pb-1 text-xs font-medium text-muted-foreground">
        {title} <span className="tabular-nums">{entries.length}</span>
      </h3>
      <div className="flex flex-col gap-0.5">
        {entries.map((e) => (
          <Row key={e.key} entry={e} />
        ))}
      </div>
    </section>
  );
}

function Row({ entry: e }: { entry: UninstallPreview }) {
  const [more, setMore] = useState(false);
  const how = e.installer ? `${e.method} · ${e.installer}` : e.method;
  return (
    <div className="rounded-lg px-2 py-2 transition-colors hover:bg-muted/50">
      <div className="flex items-center gap-3">
        <AppIcon path={e.icon} name={e.name} size={36} />
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 items-baseline gap-2">
            <span className="truncate text-sm font-medium">{e.name}</span>
            {e.version && (
              <span className="shrink-0 text-xs text-muted-foreground tabular-nums">
                {e.version}
              </span>
            )}
          </div>
          <div className="mt-0.5 flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground">
            <SourceBadge type={e.kind} />
            {e.publisher && <span className="min-w-0 truncate">{e.publisher}</span>}
            {e.size ? <span className="shrink-0 tabular-nums">{formatBytes(e.size)}</span> : null}
            {e.inShelf.length > 0 && (
              <span className="flex shrink-0 items-center gap-0.5 text-success">
                <BookMarkedIcon className="size-3" />
                在清单中
              </span>
            )}
          </div>
        </div>
        {e.runnable ? (
          <Pill tone={e.quiet ? "primary" : "warning"}>
            {e.quiet ? "静默卸载" : "打开卸载程序"}
          </Pill>
        ) : (
          <Pill tone="danger" className="max-w-[45%]">
            <BanIcon />
            <span className="truncate">{e.plan.replace(/^(不能卸载|跳过)：/, "")}</span>
          </Pill>
        )}
        {e.runnable && (
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label="详情"
            aria-expanded={more}
            onClick={() => setMore(!more)}
          >
            <ChevronDownIcon
              className={cn("transition-transform duration-200", more && "rotate-180")}
            />
          </Button>
        )}
      </div>
      {e.runnable && e.notes.length > 0 && (
        <ul className="mt-1.5 ml-12 flex flex-col gap-0.5">
          {e.notes.map((n) => (
            <li key={n} className="flex items-start gap-1.5 text-xs text-warning">
              <TriangleAlertIcon className="mt-0.5 size-3 shrink-0" />
              {n}
            </li>
          ))}
        </ul>
      )}
      {more && (
        <dl className="anim-fade mt-2 ml-12 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 rounded-md border bg-muted/30 px-3 py-2 text-xs">
          <Fact label="方式" value={how} />
          <Fact label="命令" value={e.command} mono />
          {e.location && <Fact label="位置" value={e.location} mono icon={FolderIcon} />}
          <Fact
            label="签名"
            value={e.signer ? `${e.signer}${e.signed ? "" : "（签名无效）"}` : "未签名或未知"}
            icon={e.signer && e.signed ? ShieldCheckIcon : ShieldQuestionIcon}
          />
        </dl>
      )}
    </div>
  );
}

function Fact({
  label,
  value,
  mono,
  icon: Icon,
}: {
  label: string;
  value: string;
  mono?: boolean;
  icon?: LucideIcon;
}) {
  return (
    <>
      <dt className="text-muted-foreground">{label}</dt>
      <dd
        className={cn("flex min-w-0 items-start gap-1 break-all select-text", mono && "font-mono")}
      >
        {Icon && <Icon className="mt-0.5 size-3 shrink-0 text-muted-foreground" />}
        {value}
      </dd>
    </>
  );
}

/** UninstallButton is the action a row of the installed list offers. */
export function UninstallButton({
  onClick,
  className,
}: {
  onClick: () => void;
  className?: string;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size="icon-xs"
          aria-label="卸载"
          className={cn("text-muted-foreground hover:text-destructive", className)}
          onClick={(ev) => {
            ev.stopPropagation();
            onClick();
          }}
        >
          <PackageXIcon />
        </Button>
      </TooltipTrigger>
      <TooltipContent>卸载…</TooltipContent>
    </Tooltip>
  );
}
