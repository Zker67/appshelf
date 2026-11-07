// What installing or updating apps would do, app by app, before it starts.

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
import { errorText, useShelf } from "@/lib/api";
import { sourceID } from "@/lib/shelf";
import { Installed, Tasks, type PreviewEntry, type TaskAction } from "@/mygo";
import { useQuery } from "@tanstack/react-query";
import {
  ArrowUpCircleIcon,
  BanIcon,
  CircleSlashIcon,
  DownloadIcon,
  FileArchiveIcon,
  InfoIcon,
  LoaderCircleIcon,
  type LucideIcon,
} from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

export interface PreviewRequest {
  ids: string[];
  action: TaskAction;
  seq: number;
  open: boolean;
  installed?: boolean;
}

export function PreviewDialog({
  request,
  onClose,
  onStarted,
}: {
  request: PreviewRequest | null;
  onClose: () => void;
  onStarted: () => void;
}) {
  const open = request?.open ?? false;
  const action = request?.action ?? "install";
  const verb = action === "install" ? "安装" : "更新";
  const preview = useQuery({
    // Under the shelf's key: read again while the states of apps just
    // added arrive.
    queryKey: [request?.installed ? "installed" : "shelf", "preview", request?.seq ?? 0],
    queryFn: () =>
      request?.installed
        ? Installed.previewUpdates(request.ids)
        : Tasks.preview(request?.ids ?? [], action),
    enabled: open && !!request,
  });
  const { data: shelf } = useShelf();
  const checking = new Set((shelf?.apps ?? []).filter((a) => a.checking).map((a) => a.app.id));
  const entries = preview.data ?? [];
  const run = entries.filter((e) => e.runnable);
  const reading = entries.filter((e) => !e.runnable && checking.has(e.appId));
  const skipped = entries.filter((e) => !e.runnable && !checking.has(e.appId));
  const [busy, setBusy] = useState(false);

  const start = async () => {
    if (!request) return;
    setBusy(true);
    try {
      const n = request.installed
        ? await Installed.startUpdates(request.ids)
        : await Tasks.start(request.ids, action);
      toast.success(`已开始${verb} ${n} 个软件`);
      onClose();
      onStarted();
    } catch (err) {
      toast.error(`无法开始${verb}`, { description: errorText(err) });
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="flex max-h-[min(720px,calc(100vh-6rem))] flex-col gap-0 p-0 sm:max-w-2xl">
        <DialogHeader className="border-b px-5 pt-5 pb-4">
          <DialogTitle className="flex items-center gap-2 text-lg">
            {action === "install" ? (
              <DownloadIcon className="size-5 text-primary" />
            ) : (
              <ArrowUpCircleIcon className="size-5 text-primary" />
            )}
            {verb}预览
          </DialogTitle>
          <DialogDescription className="flex flex-wrap items-center gap-1.5 pt-1">
            {preview.isPending ? (
              "正在核对每个软件的状态"
            ) : (
              <>
                <Pill tone="primary">
                  {action === "install" ? <DownloadIcon /> : <ArrowUpCircleIcon />}
                  {verb} {run.length}
                </Pill>
                {reading.length > 0 && (
                  <Pill tone="muted">
                    <LoaderCircleIcon className="animate-spin" />
                    读取中 {reading.length}
                  </Pill>
                )}
                {skipped.length > 0 && (
                  <Pill tone="muted">
                    <CircleSlashIcon />
                    跳过 {skipped.length}
                  </Pill>
                )}
              </>
            )}
          </DialogDescription>
        </DialogHeader>
        <ScrollArea className="min-h-0 flex-1">
          <div className="flex flex-col gap-4 px-3 py-3">
            {preview.isPending ? (
              <div className="flex flex-col gap-1 px-2">
                {Array.from({ length: Math.min(request?.ids.length ?? 3, 6) }, (_, i) => (
                  <div key={i} className="flex items-center gap-3 py-2">
                    <Skeleton className="size-9 rounded-[26%]" />
                    <div className="flex flex-1 flex-col gap-1.5">
                      <Skeleton className="h-3.5 w-40" />
                      <Skeleton className="h-3 w-64" />
                    </div>
                    <Skeleton className="h-5 w-16 rounded-full" />
                  </div>
                ))}
              </div>
            ) : preview.isError ? (
              <div className="px-3 py-8 text-center text-sm text-destructive">
                {errorText(preview.error)}
              </div>
            ) : (
              <>
                <Group title={`将${verb}`} entries={run} kind="run" />
                <Group title="正在读取状态" entries={reading} kind="reading" />
                <Group title="跳过" entries={skipped} kind="skip" />
              </>
            )}
          </div>
        </ScrollArea>
        <DialogFooter className="mx-0 mb-0 items-center rounded-b-xl px-5 py-3.5">
          <span className="mr-auto text-xs text-muted-foreground">
            {reading.length > 0 && run.length === 0 ? "读到状态后即可开始" : "开始前会再核对一次"}
          </span>
          <Button variant="outline" onClick={onClose}>
            取消
          </Button>
          <Button onClick={start} disabled={busy || run.length === 0}>
            {busy ? (
              <LoaderCircleIcon className="animate-spin" />
            ) : action === "install" ? (
              <DownloadIcon />
            ) : (
              <ArrowUpCircleIcon />
            )}
            开始{verb}
            {run.length > 0 && <span className="tabular-nums">（{run.length}）</span>}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function Group({
  title,
  entries,
  kind,
}: {
  title: string;
  entries: PreviewEntry[];
  kind: "run" | "reading" | "skip";
}) {
  if (entries.length === 0) return null;
  return (
    <section className="anim-rise">
      <h3 className="px-2 pb-1 text-xs font-medium text-muted-foreground">
        {title} <span className="tabular-nums">{entries.length}</span>
      </h3>
      <div className="flex flex-col">
        {entries.map((e) => (
          <Row key={e.appId} entry={e} kind={kind} />
        ))}
      </div>
    </section>
  );
}

function Row({ entry: e, kind }: { entry: PreviewEntry; kind: "run" | "reading" | "skip" }) {
  const blocked = e.plan.startsWith("不可用");
  const reason = e.plan.replace(/^跳过：/, "");
  let pill: { tone: "primary" | "muted" | "danger"; icon: LucideIcon; label: string };
  if (kind === "run")
    pill = {
      tone: "primary",
      icon: e.plan.startsWith("更新") ? ArrowUpCircleIcon : DownloadIcon,
      label: e.plan,
    };
  else if (kind === "reading") pill = { tone: "muted", icon: LoaderCircleIcon, label: "读取中" };
  else
    pill = {
      tone: blocked ? "danger" : "muted",
      icon: blocked ? BanIcon : CircleSlashIcon,
      label: reason,
    };
  const Icon = pill.icon;
  return (
    <div className="flex items-center gap-3 rounded-lg px-2 py-2 transition-colors hover:bg-muted/60">
      <AppIcon path={e.icon} name={e.name} size={36} />
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-medium">{e.name}</div>
        <div className="mt-0.5 flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground">
          <SourceBadge type={e.source.type} />
          <span className="truncate font-mono select-text">{sourceID(e.source)}</span>
          {e.asset && (
            <span className="flex min-w-0 items-center gap-1">
              <FileArchiveIcon className="size-3 shrink-0" />
              <span className="truncate font-mono select-text">{e.asset}</span>
              {e.release && <span className="shrink-0">（{e.release}）</span>}
            </span>
          )}
          {e.note && (
            <Tooltip>
              <TooltipTrigger asChild>
                <InfoIcon className="size-3.5 shrink-0 text-warning" />
              </TooltipTrigger>
              <TooltipContent className="max-w-80">{e.note}</TooltipContent>
            </Tooltip>
          )}
        </div>
      </div>
      <Pill tone={pill.tone} className="max-w-[45%]">
        <Icon className={kind === "reading" ? "animate-spin" : undefined} />
        <span className="truncate">{pill.label}</span>
      </Pill>
    </div>
  );
}
