// The tasks: installs, updates and removals, one at a time, with their stage, their
// progress and their output as it comes.

import { AppIcon } from "@/components/app-icon";
import { EmptyState } from "@/components/common";
import { Pill } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { errorText, useTasks } from "@/lib/api";
import { formatElapsed, lineKind, queueProgress, taskProgress } from "@/lib/tasks";
import { formatClock, formatDateTime } from "@/lib/text";
import { useTaskLog } from "@/lib/use-task-log";
import { cn } from "@/lib/utils";
import { Tasks, type TaskAction, type TaskControl, type TaskEntry, type TaskState } from "@/mygo";
import { useActions } from "@/state/actions";
import { useNav } from "@/state/nav";
import {
  AppWindowIcon,
  ArrowDownToLineIcon,
  BrushCleaningIcon,
  CheckCircle2Icon,
  ChevronDownIcon,
  CircleSlashIcon,
  CircleStopIcon,
  ClockIcon,
  CopyIcon,
  ExternalLinkIcon,
  SearchCheckIcon,
  ListChecksIcon,
  LoaderCircleIcon,
  PowerIcon,
  MonitorUpIcon,
  PauseIcon,
  PlayIcon,
  RotateCcwIcon,
  TerminalIcon,
  TimerOffIcon,
  XCircleIcon,
  XIcon,
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

/** A failed task the user canceled reads as canceled, not failed. */
const canceledState: (typeof states)[TaskState] = {
  icon: CircleSlashIcon,
  tone: "muted",
  label: () => "已取消",
};

async function run(what: string, fn: () => Promise<unknown>) {
  try {
    await fn();
  } catch (err) {
    toast.error(`${what}失败`, { description: errorText(err) });
  }
}

/** Clicks on these leave the panel open: its own button, and what opens over it. */
const keepOpenOn =
  '[data-tasks-trigger], [role="dialog"], [role="alertdialog"], [data-slot$="dialog-overlay"], [data-sonner-toaster]';

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
  const waiting = items.filter((t) => t.state === "waiting").length;
  const progress = queueProgress(items);
  // It floats beside the pages without holding them: a click outside, Esc or
  // its button close it, and the pages stay usable while it is open.
  return (
    <Sheet open={open} onOpenChange={onOpenChange} modal={false}>
      <SheetContent
        side="right"
        showCloseButton={false}
        onPointerDownOutside={(e) => {
          const target = e.detail.originalEvent.target;
          if (target instanceof Element && target.closest(keepOpenOn)) e.preventDefault();
        }}
        onFocusOutside={(e) => e.preventDefault()}
        className="glass-float top-[calc(var(--mygo-titlebar-height,44px)+0.5rem)]! right-2! bottom-auto! h-auto! max-h-[calc(100%-var(--mygo-titlebar-height,44px)-1rem)]! w-[460px]! max-w-[calc(100vw-1rem)]! gap-0 overflow-hidden rounded-xl border-0! bg-(--glass-float)"
      >
        <SheetHeader className="relative flex-row items-center gap-2 border-b py-2.5 pr-2.5 pl-4">
          <SheetTitle className="flex shrink-0 items-center gap-2 text-base">
            <ListChecksIcon className="size-5 text-primary" />
            任务
          </SheetTitle>
          <SheetDescription asChild>
            <div className="flex min-w-0 items-center gap-1.5 tabular-nums">
              {progress.total > 0 && (
                <span className="text-sm text-muted-foreground">
                  {progress.done}/{progress.total}
                </span>
              )}
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
              {!progress.active && progress.total > 0 && progress.failed === 0 && (
                <Pill tone="success">
                  <CheckCircle2Icon />
                  全部完成
                </Pill>
              )}
              {progress.failed > 0 && (
                <Pill tone="danger">
                  <XCircleIcon />
                  失败 {progress.failed}
                </Pill>
              )}
            </div>
          </SheetDescription>
          <div className="ml-auto flex shrink-0 items-center gap-0.5">
            {data?.running ? (
              <Button
                variant="ghost"
                size="sm"
                disabled={data.stopping}
                onClick={() => run("停止", () => Tasks.stop())}
              >
                <PauseIcon />
                停止
              </Button>
            ) : (
              waiting > 0 && (
                <Button
                  variant="ghost"
                  size="sm"
                  className="text-primary"
                  onClick={() => run("继续", () => Tasks.resume())}
                >
                  <PlayIcon />
                  继续
                </Button>
              )
            )}
            {progress.done > 0 && (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => run("清除", () => Tasks.clearDone())}
              >
                <BrushCleaningIcon />
                清除已完成
              </Button>
            )}
            <SheetClose asChild>
              <Button variant="ghost" size="icon-sm" aria-label="关闭">
                <XIcon />
              </Button>
            </SheetClose>
          </div>
          {progress.active && (
            <div
              className="absolute inset-x-0 -bottom-px h-0.5 overflow-hidden"
              role="progressbar"
              aria-label="整体进度"
              aria-valuemin={0}
              aria-valuemax={progress.total}
              aria-valuenow={progress.done}
            >
              <div
                className="h-full rounded-r-full bg-primary transition-[width] duration-500 ease-out"
                style={{ width: `${(progress.done / progress.total) * 100}%` }}
              />
            </div>
          )}
        </SheetHeader>
        {!!data?.reboot?.length && (
          <div className="flex items-start gap-2 border-b bg-warning/10 px-4 py-2 text-xs text-muted-foreground">
            <PowerIcon className="mt-0.5 size-3.5 shrink-0 text-warning" />
            <span className="min-w-0">
              重启电脑后才能完成：
              <span className="text-foreground">{rebootNames(data.reboot)}</span>
            </span>
          </div>
        )}
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
          <div className="min-h-0 flex-auto overflow-y-auto [scrollbar-width:thin]">
            <div className="flex flex-col gap-2 p-2.5">
              {items.map((t) => (
                <TaskRow key={t.id} task={t} />
              ))}
            </div>
          </div>
        )}
      </SheetContent>
    </Sheet>
  );
}

/** rebootNames lists the programs waiting for a restart, at most five. */
function rebootNames(list: readonly string[]): string {
  const unique = [...new Set(list)];
  return unique.length > 5
    ? `${unique.slice(0, 5).join("、")} 等 ${unique.length} 个`
    : unique.join("、");
}

/** endedAt writes when a task ended: the time alone today, the date too before. */
function endedAt(value: string | undefined): string {
  const full = formatDateTime(value);
  if (!full || !value) return "";
  return new Date(value).toDateString() === new Date().toDateString() ? formatClock(value) : full;
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

/** What the user can do while a removal waits for its uninstaller. */
function WaitActions({ task: t }: { task: TaskEntry }) {
  const control = (what: string, c: TaskControl) => run(what, () => Tasks.control(t.id, c));
  return (
    <div className="-ml-1.5 flex flex-wrap items-center gap-0.5">
      {t.window && (
        <Button variant="ghost" size="xs" onClick={() => control("显示窗口", "showWindow")}>
          <MonitorUpIcon />
          显示窗口
        </Button>
      )}
      {t.canSwitch && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="xs"
              className="text-primary"
              onClick={() => control("改用窗口卸载", "switchToWindow")}
            >
              <AppWindowIcon />
              改用窗口卸载
            </Button>
          </TooltipTrigger>
          <TooltipContent className="max-w-72">
            结束这次静默卸载，打开它自己的卸载窗口，由你在窗口里完成
          </TooltipContent>
        </Tooltip>
      )}
      {t.wait === "stalled" && !t.canSwitch && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button variant="ghost" size="xs" onClick={() => control("结束卸载程序", "end")}>
              <CircleStopIcon />
              结束卸载程序
            </Button>
          </TooltipTrigger>
          <TooltipContent className="max-w-72">
            结束卸载程序的进程，之后复查它是否已卸载；以管理员身份运行的需要在它的窗口里关闭
          </TooltipContent>
        </Tooltip>
      )}
      <Tooltip>
        <TooltipTrigger asChild>
          <Button variant="ghost" size="xs" onClick={() => control("停止等待", "stopWaiting")}>
            <TimerOffIcon />
            停止等待
          </Button>
        </TooltipTrigger>
        <TooltipContent className="max-w-72">
          不再等待，立即复查它是否已卸载；卸载程序不会被结束
        </TooltipContent>
      </Tooltip>
    </div>
  );
}

function TaskRow({ task: t }: { task: TaskEntry }) {
  const actions = useActions();
  const running = t.state === "running";
  const canceled = t.state === "failed" && !!t.canceled;
  const failed = t.state === "failed" && !canceled;
  const st = canceled ? canceledState : states[t.state];
  const Icon = st.icon;
  const canLog = running || t.hasLog;
  // A failed task shows its output; the others when asked.
  const [shown, setShown] = useState<boolean | null>(null);
  const logOpen = canLog && (shown ?? failed);
  // The uninstaller waits for the user, or seems stuck.
  const alert = running && (t.wait === "user" || t.wait === "stalled");
  const now = useNow(running);
  const started = t.started ? new Date(t.started).getTime() : 0;
  const elapsed = running && started ? formatElapsed((now - started) / 1000) : "";
  const progress = taskProgress(t);
  const ended = endedAt(t.ended);
  return (
    <div
      className={cn(
        "anim-rise rounded-xl border bg-card px-3 py-2.5 shadow-xs transition-colors",
        running && "border-primary/25 bg-primary/6",
        failed && "border-destructive/25 bg-destructive/6",
        !running && !failed && "hover:border-primary/30",
      )}
    >
      <div className="flex items-start gap-3">
        <AppIcon path={t.icon} name={t.name} size={36} />
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm font-medium">{t.name}</div>
          {running && (
            <div className="mt-2 flex flex-col gap-1.5">
              <div className="flex items-baseline gap-2 text-xs">
                <span
                  className={cn(
                    "min-w-0 truncate font-medium",
                    alert ? "text-warning" : "text-primary",
                  )}
                >
                  {t.stage || verbs[t.action].doing}
                </span>
                <span className="ml-auto shrink-0 text-muted-foreground tabular-nums">
                  {progress.text}
                  {progress.text && elapsed && " · "}
                  {elapsed}
                </span>
              </div>
              <ProgressBar percent={progress.percent} />
              {t.prompt && (
                <div className="line-clamp-3 text-xs break-words text-muted-foreground select-text">
                  {t.prompt}
                </div>
              )}
              {t.wait && <WaitActions task={t} />}
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
            <Pill tone={st.tone}>
              <Icon className={running ? "animate-spin" : undefined} />
              {st.label(t.action)}
            </Pill>
            {!running && ended && <span>{ended}</span>}
            <span className="ml-auto flex items-center gap-0.5">
              {t.cancelable && (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="ghost"
                      size="xs"
                      onClick={() => run("取消", () => Tasks.cancel(t.id))}
                    >
                      <CircleSlashIcon />
                      取消
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent className="max-w-72">
                    {running
                      ? "现在还没有改动这台电脑：停止它，这个任务记为已取消"
                      : "移出队列，不再执行"}
                  </TooltipContent>
                </Tooltip>
              )}
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
              {t.state === "failed" && t.interactive && (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="ghost"
                      size="xs"
                      onClick={() => run("重试", () => Tasks.retryInteractive(t.id))}
                    >
                      <AppWindowIcon />
                      {t.action === "upgrade" ? "打开安装程序重试" : "打开卸载程序重试"}
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent className="max-w-72">
                    {t.action === "upgrade"
                      ? "不再静默更新，打开它自己的安装程序窗口，由你在窗口里完成"
                      : "不再静默执行，打开它自己的卸载窗口，由你在窗口里完成"}
                  </TooltipContent>
                </Tooltip>
              )}
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
