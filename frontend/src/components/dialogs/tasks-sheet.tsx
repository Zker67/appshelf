// The tasks: installs and updates, one at a time, with their logs.

import { AppIcon } from "@/components/app-icon";
import { EmptyState } from "@/components/common";
import { Pill } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { errorText, useTasks } from "@/lib/api";
import { formatDateTime } from "@/lib/text";
import { cn } from "@/lib/utils";
import { Tasks, type TaskEntry, type TaskState } from "@/mygo";
import { useNav } from "@/state/nav";
import {
  BrushCleaningIcon,
  CheckCircle2Icon,
  CircleSlashIcon,
  ClockIcon,
  FileTextIcon,
  ListChecksIcon,
  LoaderCircleIcon,
  PauseIcon,
  PlayIcon,
  RotateCcwIcon,
  XCircleIcon,
  type LucideIcon,
} from "lucide-react";
import { toast } from "sonner";

const states: Record<
  TaskState,
  {
    icon: LucideIcon;
    tone: "primary" | "success" | "muted" | "danger";
    label: (install: boolean) => string;
  }
> = {
  waiting: { icon: ClockIcon, tone: "muted", label: () => "等待中" },
  running: { icon: LoaderCircleIcon, tone: "primary", label: (i) => (i ? "正在安装" : "正在更新") },
  succeeded: { icon: CheckCircle2Icon, tone: "success", label: (i) => (i ? "已安装" : "已更新") },
  skipped: { icon: CircleSlashIcon, tone: "muted", label: () => "已跳过" },
  failed: { icon: XCircleIcon, tone: "danger", label: () => "失败" },
};

async function run(what: string, fn: () => Promise<unknown>) {
  try {
    await fn();
  } catch (err) {
    toast.error(`${what}失败`, { description: errorText(err) });
  }
}

export function TasksSheet({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const nav = useNav();
  const { data } = useTasks();
  const items = data?.items ?? [];
  const count = (s: TaskState) => items.filter((t) => t.state === s).length;
  const waiting = count("waiting");
  const done = items.length - waiting - count("running");
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="right"
        className="gap-0 data-[side=right]:top-[var(--mygo-titlebar-height,44px)] data-[side=right]:h-[calc(100%-var(--mygo-titlebar-height,44px))] data-[side=right]:w-[440px] data-[side=right]:sm:max-w-[calc(100vw-4rem)]"
      >
        <SheetHeader className="gap-2 border-b px-5 pt-4 pb-3.5">
          <SheetTitle className="flex items-center gap-2 text-base">
            <ListChecksIcon className="size-5 text-primary" />
            任务
          </SheetTitle>
          <SheetDescription asChild>
            <div className="flex flex-wrap items-center gap-1.5">
              {data?.running && (
                <Pill tone="primary">
                  <LoaderCircleIcon className="animate-spin" />
                  {data.stopping ? "正在停止" : "执行中"}
                </Pill>
              )}
              {!data?.running && waiting > 0 && (
                <Pill tone="warning">
                  <PauseIcon />
                  已停止
                </Pill>
              )}
              <Pill tone="muted">
                <ClockIcon />
                等待 {waiting}
              </Pill>
              <Pill tone="success">
                <CheckCircle2Icon />
                成功 {count("succeeded")}
              </Pill>
              {count("failed") > 0 && (
                <Pill tone="danger">
                  <XCircleIcon />
                  失败 {count("failed")}
                </Pill>
              )}
            </div>
          </SheetDescription>
          <div className="flex items-center gap-1.5 pt-1">
            {data?.running ? (
              <Button
                variant="outline"
                size="sm"
                disabled={data.stopping}
                onClick={() => run("停止", () => Tasks.stop())}
              >
                <PauseIcon />
                停止
              </Button>
            ) : (
              <Button
                variant="outline"
                size="sm"
                disabled={waiting === 0}
                onClick={() => run("继续", () => Tasks.resume())}
              >
                <PlayIcon />
                继续
              </Button>
            )}
            <Button
              variant="ghost"
              size="sm"
              disabled={done === 0}
              onClick={() => run("清除", () => Tasks.clearDone())}
            >
              <BrushCleaningIcon />
              清除已完成
            </Button>
          </div>
        </SheetHeader>
        {items.length === 0 ? (
          <EmptyState icon={ListChecksIcon} title="没有任务">
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                onOpenChange(false);
                nav.go("shelf");
              }}
            >
              去我的清单
            </Button>
          </EmptyState>
        ) : (
          <ScrollArea className="min-h-0 flex-1">
            <div className="flex flex-col gap-1.5 p-3">
              {items.map((t) => (
                <TaskRow key={t.id} task={t} />
              ))}
            </div>
          </ScrollArea>
        )}
      </SheetContent>
    </Sheet>
  );
}

function TaskRow({ task: t }: { task: TaskEntry }) {
  const st = states[t.state];
  const Icon = st.icon;
  const started = formatDateTime(t.started);
  const ended = formatDateTime(t.ended);
  return (
    <div
      className={cn(
        "anim-rise relative overflow-hidden rounded-xl border bg-card p-3 shadow-xs transition-colors",
        t.state === "running" && "border-primary/35",
        t.state === "failed" && "border-destructive/30",
      )}
    >
      <div className="flex items-start gap-3">
        <AppIcon path={t.icon} name={t.name} size={36} />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="truncate text-sm font-medium">{t.name}</span>
            <Pill tone={st.tone} className="ml-auto">
              <Icon className={t.state === "running" ? "animate-spin" : undefined} />
              {st.label(t.action === "install")}
            </Pill>
          </div>
          {t.message && (
            <div className="mt-1 text-xs break-words text-muted-foreground select-text">
              {t.message}
            </div>
          )}
          <div className="mt-1.5 flex items-center gap-2 text-[11px] text-muted-foreground tabular-nums">
            {started && <span>开始 {started}</span>}
            {ended && <span>结束 {ended}</span>}
            <span className="ml-auto flex items-center gap-0.5">
              {t.state === "failed" && (
                <Button
                  variant="ghost"
                  size="xs"
                  onClick={() => run("重试", () => Tasks.retry(t.id))}
                >
                  <RotateCcwIcon />
                  重试
                </Button>
              )}
              {t.hasLog && (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="ghost"
                      size="xs"
                      onClick={() => run("打开日志", () => Tasks.openLog(t.id))}
                    >
                      <FileTextIcon />
                      日志
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>用默认程序打开这次的日志</TooltipContent>
                </Tooltip>
              )}
            </span>
          </div>
        </div>
      </div>
      {t.state === "running" && <div className="shimmer absolute inset-x-0 bottom-0 h-0.5" />}
    </div>
  );
}
