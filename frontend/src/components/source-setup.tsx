// Setting up winget and scoop: what installing each does, installing it
// one source at a time, and the prompt shown the first time one is missing.

import { CopyButton } from "@/components/common";
import { SourceIcon } from "@/components/source-badge";
import { Pill } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { errorText, useSources } from "@/lib/api";
import { cn } from "@/lib/utils";
import { About, Sources, type SourceType } from "@/mygo";
import {
  CheckCircle2Icon,
  DownloadIcon,
  LoaderCircleIcon,
  RotateCwIcon,
  ShieldCheckIcon,
  SparklesIcon,
  TerminalIcon,
  TriangleAlertIcon,
} from "lucide-react";
import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import { toast } from "sonner";

/** Setup is what installing a source's missing part does. */
export interface Setup {
  /** what is installed */
  name: string;
  /** the button */
  action: string;
  /** what it is for */
  use: string;
  /** how it is installed */
  how: string;
  command: string;
}

export const setups: Partial<Record<SourceType, Setup>> = {
  winget: {
    name: "Microsoft.WinGet.Client",
    action: "安装模块",
    use: "读取 winget 的已装软件、可用更新和搜索结果",
    how: "从 PowerShell Gallery 装到当前用户，不需要管理员，约 1–3 分钟",
    command: "Install-Module Microsoft.WinGet.Client -Scope CurrentUser",
  },
  scoop: {
    name: "scoop",
    action: "安装 scoop",
    use: "把软件装在用户目录，安装和更新都不弹窗口",
    how: "官方安装器，装到当前用户，不需要管理员；PowerShell 禁止本地脚本时按官方说明为当前用户设为 RemoteSigned",
    command: "irm get.scoop.sh | iex",
  },
};

export type SetupPhase = "idle" | "running" | "done" | "failed";

// The phases are shared by the prompt and the settings, which can both
// start a setup and show it running.
const phases = new Map<SourceType, SetupPhase>();
const listeners = new Set<() => void>();
let version = 0;

function setPhase(type: SourceType, phase: SetupPhase) {
  phases.set(type, phase);
  version++;
  for (const l of listeners) l();
}

function subscribe(l: () => void) {
  listeners.add(l);
  return () => listeners.delete(l);
}

/** useSetupPhase is where the setup of a source is. */
export function useSetupPhase(type: SourceType): SetupPhase {
  useSyncExternalStore(subscribe, () => version);
  return phases.get(type) ?? "idle";
}

/** useRunningSetups lists the sources being set up. */
export function useRunningSetups(): SourceType[] {
  useSyncExternalStore(subscribe, () => version);
  return [...phases].filter(([, p]) => p === "running").map(([t]) => t);
}

/**
 * runSetup installs what a source misses. While it runs, the title bar
 * shows it (SetupActivity), covering nothing; a toast tells how it ended.
 */
export async function runSetup(type: SourceType) {
  const setup = setups[type];
  if (!setup || phases.get(type) === "running") return;
  setPhase(type, "running");
  try {
    await Sources.fix(type);
    setPhase(type, "done");
    toast.success(`已安装 ${setup.name}`, { description: "已重新检查来源", duration: 4000 });
  } catch (err) {
    setPhase(type, "failed");
    toast.error(`安装 ${setup.name} 失败`, {
      description: errorText(err),
      duration: 15000,
      action: {
        label: "打开日志",
        onClick: () => void About.openLogs().catch(() => undefined),
      },
    });
  }
}

/** SetupButton starts or retries a source's setup, and shows it running. */
export function SetupButton({
  type,
  onStart,
  className,
}: {
  type: SourceType;
  /** onStart, when set, is called instead of starting, as to confirm first. */
  onStart?: () => void;
  className?: string;
}) {
  const setup = setups[type];
  const phase = useSetupPhase(type);
  if (!setup) return null;
  const start = onStart ?? (() => void runSetup(type));
  return (
    <Button
      size="sm"
      className={cn("min-w-24", className)}
      variant={phase === "done" ? "outline" : "default"}
      disabled={phase === "running" || phase === "done"}
      onClick={start}
    >
      {phase === "running" ? (
        <LoaderCircleIcon className="animate-spin" />
      ) : phase === "done" ? (
        <CheckCircle2Icon className="text-success" />
      ) : phase === "failed" ? (
        <RotateCwIcon />
      ) : (
        <DownloadIcon />
      )}
      {phase === "running"
        ? "安装中"
        : phase === "done"
          ? "已安装"
          : phase === "failed"
            ? "重试"
            : setup.action}
    </Button>
  );
}

function PhasePill({ phase }: { phase: SetupPhase }) {
  switch (phase) {
    case "running":
      return (
        <Pill tone="primary">
          <LoaderCircleIcon className="animate-spin" />
          安装中
        </Pill>
      );
    case "done":
      return (
        <Pill tone="success" className="anim-fade">
          <CheckCircle2Icon />
          已就绪
        </Pill>
      );
    case "failed":
      return (
        <Pill tone="danger">
          <TriangleAlertIcon />
          安装失败
        </Pill>
      );
    default:
      return (
        <Pill tone="warning">
          <TriangleAlertIcon />
          未安装
        </Pill>
      );
  }
}

/** SetupCard is a source to set up, in the prompt. */
function SetupCard({ type, index }: { type: SourceType; index: number }) {
  const setup = setups[type];
  const phase = useSetupPhase(type);
  if (!setup) return null;
  return (
    <div
      className={cn(
        "anim-rise group relative overflow-hidden rounded-xl border bg-card/70 p-4 shadow-xs transition-[border-color,box-shadow] duration-300",
        phase === "running" && "border-primary/40 shadow-sm",
        phase === "done" && "border-success/40",
      )}
      style={{ animationDelay: `${80 + index * 70}ms` }}
    >
      {phase === "running" && (
        <div className="shimmer pointer-events-none absolute inset-x-0 top-0 h-0.5" aria-hidden />
      )}
      <div className="flex items-start gap-3.5">
        <div
          className={cn(
            "grid size-10 shrink-0 place-items-center rounded-xl bg-gradient-to-br from-primary/18 to-primary/5 text-primary ring-1 ring-primary/20 transition-transform duration-300 ring-inset group-hover:scale-105",
            phase === "done" && "from-success/18 to-success/5 text-success ring-success/25",
          )}
        >
          {phase === "done" ? (
            <CheckCircle2Icon className="anim-fade size-5" />
          ) : (
            <SourceIcon type={type} className="size-5" />
          )}
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="text-sm font-semibold">{setup.name}</span>
            <PhasePill phase={phase} />
          </div>
          <p className="mt-1 text-xs text-muted-foreground">{setup.use}</p>
        </div>
        <SetupButton type={type} className="shrink-0" />
      </div>
      <div className="mt-3 flex items-center gap-2 rounded-lg border border-border/70 bg-muted/50 py-1 pr-1 pl-2.5">
        <TerminalIcon className="size-3.5 shrink-0 text-muted-foreground" />
        <code
          className="min-w-0 flex-1 truncate font-mono text-xs select-text"
          title={setup.command}
        >
          {setup.command}
        </code>
        <CopyButton text={setup.command} what="命令" />
      </div>
      <p className="mt-2 flex items-start gap-1.5 text-[11px] leading-relaxed text-muted-foreground">
        <ShieldCheckIcon className="mt-px size-3.5 shrink-0 text-success" />
        {setup.how}
      </p>
    </div>
  );
}

/**
 * SourceSetupPrompt offers, the first time the sources are checked and one
 * is missing what AppShelf can install, to install it there. Once answered,
 * it is not shown again: the settings keep the buttons.
 */
export function SourceSetupPrompt() {
  const { data } = useSources();
  const [types, setTypes] = useState<SourceType[]>([]);
  const [open, setOpen] = useState(false);
  const decided = useRef(false);
  useEffect(() => {
    if (decided.current || !data) return;
    if (data.setupSeen) {
      decided.current = true;
      return;
    }
    const ours = data.sources.filter((s) => setups[s.type]);
    if (ours.length === 0 || ours.some((s) => !s.checked)) return;
    decided.current = true;
    const missing = ours.filter((s) => s.fixable).map((s) => s.type);
    if (missing.length > 0) {
      setTypes(missing);
      setOpen(true);
    }
  }, [data]);

  const running = useSyncExternalStore(subscribe, () =>
    types.some((t) => phases.get(t) === "running"),
  );
  const allDone = useSyncExternalStore(subscribe, () =>
    types.every((t) => phases.get(t) === "done"),
  );

  const close = () => {
    setOpen(false);
    Sources.dismissSetup().catch((err: unknown) =>
      toast.error("没有记住这次选择", { description: errorText(err) }),
    );
  };

  return (
    <Dialog open={open} onOpenChange={(o) => !o && close()}>
      <DialogContent className="gap-0 overflow-hidden p-0 sm:max-w-xl">
        <DialogHeader className="relative gap-1.5 border-b bg-gradient-to-br from-primary/12 via-card to-card px-6 pt-6 pb-5">
          <DialogTitle className="flex items-center gap-2.5 text-lg">
            <span className="grid size-8 place-items-center rounded-lg bg-primary text-primary-foreground shadow-sm">
              <SparklesIcon className="size-4" />
            </span>
            准备安装来源
          </DialogTitle>
          <DialogDescription>
            {types.length > 1 ? "这两个来源" : "这个来源"}还缺少组件，可以在这里直接安装
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-3 px-6 py-5">
          {types.map((t, i) => (
            <SetupCard key={t} type={t} index={i} />
          ))}
        </div>
        <div className="flex items-center gap-3 border-t bg-muted/30 px-6 py-3.5">
          <span className="min-w-0 flex-1 text-xs text-muted-foreground">
            {running ? "关闭窗口后会在后台继续安装" : "以后也能在「设置 → 安装来源」里安装"}
          </span>
          <Button variant={allDone ? "default" : "ghost"} onClick={close}>
            {allDone ? <CheckCircle2Icon /> : null}
            {allDone ? "完成" : running ? "后台安装" : "以后再说"}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}

/**
 * SetupActivity is the setups running, in the title bar: a small chip that
 * covers nothing, and opens the sources' settings.
 */
export function SetupActivity({ onOpen }: { onOpen: () => void }) {
  const running = useRunningSetups();
  if (running.length === 0) return null;
  const names = running.map((t) => setups[t]?.name ?? t).join("、");
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          onClick={onOpen}
          className="anim-fade relative mr-1 flex h-7 items-center gap-1.5 overflow-hidden rounded-full bg-primary/10 pr-3 pl-2 text-xs font-medium text-primary ring-1 ring-primary/25 transition-colors ring-inset outline-none hover:bg-primary/16 focus-visible:ring-2 focus-visible:ring-ring/50"
        >
          <span className="shimmer pointer-events-none absolute inset-0" aria-hidden />
          <LoaderCircleIcon className="relative size-3.5 animate-spin" />
          <span className="relative max-w-48 truncate">正在安装 {names}</span>
        </button>
      </TooltipTrigger>
      <TooltipContent>可以继续使用，装好后会通知你；点击查看安装来源</TooltipContent>
    </Tooltip>
  );
}
