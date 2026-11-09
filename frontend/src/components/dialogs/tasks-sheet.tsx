// The tasks: installs, updates and removals, one at a time, with their stage, their
// progress and their output as it comes.

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
import { formatElapsed, lineKind, queueProgress, taskProgress } from "@/lib/tasks";
import { formatDateTime } from "@/lib/text";
import { useTaskLog } from "@/lib/use-task-log";
import { cn } from "@/lib/utils";
import { Tasks, type TaskAction, type TaskEntry, type TaskState } from "@/mygo";
import { useActions } from "@/state/actions";
import { useNav } from "@/state/nav";
import {
  AppWindowIcon,
  ArrowDownToLineIcon,
  BrushCleaningIcon,
  CheckCircle2Icon,
  ChevronDownIcon,
  CircleSlashIcon,
  ClockIcon,
  CopyIcon,
  ExternalLinkIcon,
  SearchCheckIcon,
  ListChecksIcon,
  LoaderCircleIcon,
  PauseIcon,
  PlayIcon,
  RotateCcwIcon,
  TerminalIcon,
  XCircleIcon,
  type LucideIcon,
} from "lucide-react";
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { toast } from "sonner";

const verbs: Record<TaskAction, { doing: string; done: string }> = {
  install: { doing: "正在安装", done: "已安装" },
  upgrade: { doing: "正在更新", done: "已更新" },
  uninstall: { doing: "正在卸载", done: "已卸载" },
};

const states: Record<
  TaskState,
  {
    icon: LucideIcon;
    tone: "primary" | "success" | "muted" | "danger";
    label: (action: TaskAction) => string;
  }
> = {
  waiting: { icon: ClockIcon, tone: "muted", label: () => "等待中" },
  running: { icon: LoaderCircleIcon, tone: "primary", label: (a) => verbs[a].doing },
  succeeded: { icon: CheckCircle2Icon, tone: "success", label: (a) => verbs[a].done },
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
  const progress = queueProgress(items);
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="right"
        className="gap-0 data-[side=right]:top-[var(--mygo-titlebar-height,44px)] data-[side=right]:h-[calc(100%-var(--mygo-titlebar-height,44px))] data-[side=right]:w-[480px] data-[side=right]:sm:max-w-[calc(100vw-4rem)]"
      >
        <SheetHeader className="gap-2 border-b px-5 pt-4 pb-3.5">
          <SheetTitle className="flex items-center gap-2 text-base">
            <ListChecksIcon className="size-5 text-primary" />
            任务
            {progress.total > 0 && (
              <span className="ml-1 text-sm font-normal text-muted-foreground tabular-nums">
                {progress.done}/{progress.total}
              </span>
            )}
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
              {progress.failed > 0 && (
                <Pill tone="danger">
                  <XCircleIcon />
                  失败 {progress.failed}
                </Pill>
              )}
            </div>
          </SheetDescription>
          {progress.total > 0 && (
            <div
              className="h-1 overflow-hidden rounded-full bg-muted"
              role="progressbar"
              aria-label="整体进度"
              aria-valuemin={0}
              aria-valuemax={progress.total}
              aria-valuenow={progress.done}
            >
              <div
                className={cn(
                  "h-full rounded-full transition-[width] duration-500 ease-out",
                  progress.failed > 0 && !progress.active ? "bg-destructive/70" : "bg-primary",
                )}
                style={{ width: `${(progress.done / progress.total) * 100}%` }}
              />
            </div>
          )}
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
              disabled={progress.done === 0}
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

/** useNow is the time, every second while on. */
function useNow(on: boolean): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!on) return;
    setNow(Date.now());
    const t = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(t);
  }, [on]);
  return now;
}

function TaskRow({ task: t }: { task: TaskEntry }) {
  const actions = useActions();
  const st = states[t.state];
  const Icon = st.icon;
  const running = t.state === "running";
  const failed = t.state === "failed";
  const canLog = running || t.hasLog;
  // A failed task shows its output; the others when asked.
  const [shown, setShown] = useState<boolean | null>(null);
  const logOpen = canLog && (shown ?? failed);
  const now = useNow(running);
  const started = t.started ? new Date(t.started).getTime() : 0;
  const elapsed = running && started ? formatElapsed((now - started) / 1000) : "";
  const progress = taskProgress(t);
  const ended = formatDateTime(t.ended);
  return (
    <div
      className={cn(
        "anim-rise relative overflow-hidden rounded-xl border bg-card p-3 shadow-xs transition-colors",
        running && "border-primary/35",
        failed && "border-destructive/30",
      )}
    >
      <div className="flex items-start gap-3">
        <AppIcon path={t.icon} name={t.name} size={36} />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="truncate text-sm font-medium">{t.name}</span>
            <Pill tone={st.tone} className="ml-auto">
              <Icon className={running ? "animate-spin" : undefined} />
              {st.label(t.action)}
            </Pill>
          </div>
          {running && (
            <div className="mt-2 flex flex-col gap-1.5">
              <div className="flex items-baseline gap-2 text-xs">
                <span className="min-w-0 truncate font-medium text-primary">
                  {t.stage || verbs[t.action].doing}
                </span>
                <span className="ml-auto shrink-0 text-muted-foreground tabular-nums">
                  {progress.bytes}
                  {progress.bytes && elapsed && " · "}
                  {elapsed}
                </span>
              </div>
              <ProgressBar percent={progress.percent} />
            </div>
          )}
          {t.message && (
            <div
              className={cn(
                "mt-1 text-xs break-words select-text",
                failed ? "text-destructive" : "text-muted-foreground",
              )}
            >
              {t.message}
            </div>
          )}
          <div className="mt-1.5 flex items-center gap-2 text-[11px] text-muted-foreground tabular-nums">
            {!running && ended && <span>结束 {ended}</span>}
            {t.state === "waiting" && <span>排队中</span>}
            <span className="ml-auto flex items-center gap-0.5">
              {t.leftovers && (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="ghost"
                      size="xs"
                      className="text-primary"
                      onClick={() => actions.findLeftovers(t.id, t.name)}
                    >
                      <SearchCheckIcon />
                      查找残留
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent className="max-w-72">
                    查找它留下的目录、设置、快捷方式和开机启动项，逐项确认后再清理
                  </TooltipContent>
                </Tooltip>
              )}
              {failed && t.interactive && (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="ghost"
                      size="xs"
                      onClick={() => run("重试", () => Tasks.retryInteractive(t.id))}
                    >
                      <AppWindowIcon />
                      打开卸载程序重试
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent className="max-w-72">
                    不再静默执行，打开它自己的卸载窗口，由你在窗口里完成
                  </TooltipContent>
                </Tooltip>
              )}
              {failed && (
                <Button
                  variant="ghost"
                  size="xs"
                  onClick={() => run("重试", () => Tasks.retry(t.id))}
                >
                  <RotateCcwIcon />
                  重试
                </Button>
              )}
              {canLog && (
                <Button
                  variant="ghost"
                  size="xs"
                  aria-expanded={logOpen}
                  onClick={() => setShown(!logOpen)}
                >
                  <TerminalIcon />
                  输出
                  <ChevronDownIcon
                    className={cn("transition-transform duration-200", logOpen && "rotate-180")}
                  />
                </Button>
              )}
              {t.hasLog && (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="ghost"
                      size="icon-xs"
                      aria-label="打开日志文件"
                      onClick={() => run("打开日志", () => Tasks.openLog(t.id))}
                    >
                      <ExternalLinkIcon />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>用默认程序打开这次的日志文件</TooltipContent>
                </Tooltip>
              )}
            </span>
          </div>
        </div>
      </div>
      {logOpen && <LogView key={`${t.id}-${t.started ?? ""}`} id={t.id} failed={failed} />}
    </div>
  );
}

function ProgressBar({ percent }: { percent: number | null }) {
  return (
    <div
      className="h-1.5 overflow-hidden rounded-full bg-muted"
      role="progressbar"
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={percent ?? undefined}
    >
      {percent === null ? (
        <div className="shimmer h-full w-full" />
      ) : (
        <div
          className="h-full rounded-full bg-primary transition-[width] duration-300 ease-out"
          style={{ width: `${percent}%` }}
        />
      )}
    </div>
  );
}

/** How close to the end, in pixels, still counts as following the output. */
const followSlack = 24;

function LogView({ id, failed }: { id: number; failed: boolean }) {
  const log = useTaskLog(id, true);
  const viewport = useRef<HTMLDivElement>(null);
  const [follow, setFollow] = useState(true);
  const count = log.lines.length;

  // Following, new output scrolls into view; scrolled up, it stays put.
  useLayoutEffect(() => {
    const el = viewport.current;
    if (el && follow) el.scrollTop = el.scrollHeight;
  }, [count, log.live, follow]);

  const onScroll = () => {
    const el = viewport.current;
    if (!el) return;
    const atEnd = el.scrollHeight - el.scrollTop - el.clientHeight <= followSlack;
    if (atEnd !== follow) setFollow(atEnd);
  };
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(
        [...log.lines, ...(log.live ? [log.live] : [])].join("\n"),
      );
      toast.success("已复制输出");
    } catch (err) {
      toast.error("复制失败", { description: errorText(err) });
    }
  };

  return (
    <div className="anim-fade mt-2.5 overflow-hidden rounded-lg border bg-muted/40">
      <div className="flex h-7 items-center gap-1 border-b px-2 text-[11px] text-muted-foreground">
        <TerminalIcon className="size-3" />
        <span>
          {log.dropped ? "只显示最后 2000 行，完整内容见日志文件" : `${count} 行`}
          {failed && count > 0 && " · 红色是可能的出错行"}
        </span>
        <span className="ml-auto flex items-center">
          {!follow && !log.done && (
            <Button variant="ghost" size="xs" onClick={() => setFollow(true)}>
              <ArrowDownToLineIcon />
              跟随
            </Button>
          )}
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon-xs"
                aria-label="复制输出"
                disabled={count === 0 && !log.live}
                onClick={() => void copy()}
              >
                <CopyIcon />
              </Button>
            </TooltipTrigger>
            <TooltipContent>复制全部输出</TooltipContent>
          </Tooltip>
        </span>
      </div>
      <div
        ref={viewport}
        onScroll={onScroll}
        className="max-h-60 min-h-12 overflow-y-auto px-2.5 py-2 font-mono text-[11px] leading-[1.55] select-text [scrollbar-width:thin]"
      >
        {count === 0 && !log.live ? (
          <div className="text-muted-foreground">
            {log.error || (log.done ? "没有输出" : "等待输出…")}
          </div>
        ) : (
          <>
            {log.lines.map((line, i) => (
              <LogLine key={i} line={line} />
            ))}
            {log.live && <div className="whitespace-pre-wrap text-primary">{log.live}</div>}
          </>
        )}
      </div>
    </div>
  );
}

function LogLine({ line }: { line: string }) {
  if (line === "") return <div className="h-[1.55em]" />;
  const kind = lineKind(line);
  return (
    <div
      className={cn(
        "break-all whitespace-pre-wrap",
        kind === "appshelf" && "text-primary/90",
        kind === "error" && "-mx-1 rounded-sm bg-destructive/8 px-1 text-destructive",
      )}
    >
      {line}
    </div>
  );
}
