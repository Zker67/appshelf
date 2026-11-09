// What a removed program may have left behind, each find with the reasons
// for and against it; the user checks what to clean. Files go to the
// Recycle Bin, registry keys are saved first.

import { Pill } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
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
import { cn } from "@/lib/utils";
import {
  Tasks,
  type LeftoverItem,
  type LeftoverKind,
  type LeftoverLevel,
  type LeftoverResult,
} from "@/mygo";
import { useQuery } from "@tanstack/react-query";
import {
  BrushCleaningIcon,
  CheckCircle2Icon,
  ChevronRightIcon,
  DatabaseIcon,
  FolderIcon,
  FolderOpenIcon,
  LinkIcon,
  LoaderCircleIcon,
  PowerIcon,
  SearchCheckIcon,
  XCircleIcon,
  type LucideIcon,
} from "lucide-react";
import { useMemo, useState } from "react";
import { toast } from "sonner";

export interface LeftoversRequest {
  /** taskId is the removal that succeeded. */
  taskId: number;
  name: string;
  seq: number;
  open: boolean;
}

const kinds: Record<LeftoverKind, { icon: LucideIcon; label: string }> = {
  dir: { icon: FolderIcon, label: "目录" },
  shortcut: { icon: LinkIcon, label: "快捷方式" },
  regkey: { icon: DatabaseIcon, label: "注册表项" },
  regvalue: { icon: PowerIcon, label: "开机启动项" },
};

const levels: Array<{ value: LeftoverLevel; title: string; hint: string }> = [
  { value: "high", title: "很可能是残留", hint: "有明确的关联，默认勾选" },
  { value: "medium", title: "可能是残留", hint: "名称相符，请确认后再勾选" },
  { value: "low", title: "不确定", hint: "有不像残留的迹象，一般不要清理" },
];

export function LeftoversDialog({
  request,
  onClose,
}: {
  request: LeftoversRequest | null;
  onClose: () => void;
}) {
  const open = request?.open ?? false;
  const scan = useQuery({
    queryKey: ["leftovers", request?.taskId ?? 0, request?.seq ?? 0],
    queryFn: () => Tasks.findLeftovers(request?.taskId ?? 0),
    enabled: open && !!request,
    staleTime: Infinity,
    gcTime: 0,
  });
  const items = useMemo(() => scan.data?.items ?? [], [scan.data]);
  // What the user changed of the first choice: the sure finds checked.
  const [picked, setPicked] = useState<Map<string, boolean>>(new Map());
  const checked = (it: LeftoverItem) => picked.get(it.id) ?? it.checked;
  const chosen = items.filter(checked);
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<LeftoverResult | null>(null);
  const [showLow, setShowLow] = useState(false);

  const toggle = (it: LeftoverItem, on: boolean) => setPicked((m) => new Map(m).set(it.id, on));
  const clean = async () => {
    if (!scan.data) return;
    setBusy(true);
    try {
      const res = await Tasks.cleanLeftovers(
        scan.data.token,
        chosen.map((i) => i.id),
      );
      setResult(res);
      if (res.failed === 0) toast.success(`已清理 ${res.cleaned} 项`);
      else toast.warning(`已清理 ${res.cleaned} 项，${res.failed} 项未能清理`);
    } catch (err) {
      toast.error("清理失败", { description: errorText(err) });
    } finally {
      setBusy(false);
    }
  };
  const outcome = new Map((result?.outcomes ?? []).map((o) => [o.id, o]));

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="flex max-h-[min(760px,calc(100vh-6rem))] flex-col gap-0 p-0 sm:max-w-2xl">
        <DialogHeader className="border-b px-5 pt-5 pb-4">
          <DialogTitle className="flex items-center gap-2 text-lg">
            <SearchCheckIcon className="size-5 text-primary" />「{request?.name}」的残留
          </DialogTitle>
          <DialogDescription className="pt-1">
            {scan.isPending
              ? "正在查找安装目录、数据目录、注册表、快捷方式和开机启动项"
              : result
                ? `已清理 ${result.cleaned} 项${result.failed ? `，${result.failed} 项未能清理` : ""}`
                : items.length === 0
                  ? "没有找到残留"
                  : "每一项都列出了判断依据：文件和目录移到回收站，注册表先备份再删除"}
          </DialogDescription>
        </DialogHeader>
        <ScrollArea
          className="flex min-h-0 flex-1 flex-col"
          viewportClassName="h-auto min-h-0 flex-1"
        >
          <div className="flex flex-col gap-4 px-3 py-3">
            {scan.isPending ? (
              <div className="flex flex-col gap-2 px-2">
                {Array.from({ length: 4 }, (_, i) => (
                  <div key={i} className="flex items-center gap-3 py-1.5">
                    <Skeleton className="size-4 rounded" />
                    <div className="flex flex-1 flex-col gap-1.5">
                      <Skeleton className="h-3.5 w-80" />
                      <Skeleton className="h-3 w-48" />
                    </div>
                  </div>
                ))}
              </div>
            ) : scan.isError ? (
              <div className="px-3 py-8 text-center text-sm text-destructive">
                {errorText(scan.error)}
              </div>
            ) : items.length === 0 ? (
              <div className="flex flex-col items-center gap-2 px-3 py-10 text-sm text-muted-foreground">
                <CheckCircle2Icon className="size-8 text-success" />
                卸载程序已经清理干净，或残留不在常见位置
              </div>
            ) : (
              levels.map((lv) => {
                const group = items.filter((i) => i.level === lv.value);
                if (group.length === 0) return null;
                const collapsed = lv.value === "low" && !showLow;
                return (
                  <section key={lv.value} className="anim-rise">
                    <button
                      type="button"
                      className="flex w-full items-baseline gap-2 px-2 pb-1 text-left"
                      onClick={() => lv.value === "low" && setShowLow(!showLow)}
                      disabled={lv.value !== "low"}
                    >
                      {lv.value === "low" && (
                        <ChevronRightIcon
                          className={cn(
                            "size-3.5 self-center text-muted-foreground transition-transform",
                            !collapsed && "rotate-90",
                          )}
                        />
                      )}
                      <span className="text-xs font-medium text-muted-foreground">
                        {lv.title} <span className="tabular-nums">{group.length}</span>
                      </span>
                      <span className="text-[11px] text-muted-foreground/70">{lv.hint}</span>
                    </button>
                    {!collapsed && (
                      <div className="flex flex-col gap-0.5">
                        {group.map((it) => (
                          <Row
                            key={it.id}
                            item={it}
                            checked={checked(it)}
                            disabled={!!result || busy}
                            outcome={outcome.get(it.id)}
                            onCheck={(on) => toggle(it, on)}
                          />
                        ))}
                      </div>
                    )}
                  </section>
                );
              })
            )}
          </div>
        </ScrollArea>
        <DialogFooter className="mx-0 mb-0 items-center rounded-b-xl px-5 py-3.5">
          <span className="mr-auto text-xs text-muted-foreground">
            {result?.backupDir
              ? "注册表已先备份"
              : chosen.some((i) => i.kind === "regkey" || i.kind === "regvalue")
                ? "注册表会先导出备份"
                : "文件可以从回收站还原"}
          </span>
          {result?.backupDir && scan.data && (
            <Button
              variant="outline"
              onClick={() =>
                Tasks.openBackup(scan.data.token).catch((err: unknown) =>
                  toast.error("无法打开备份", { description: errorText(err) }),
                )
              }
            >
              <FolderOpenIcon />
              打开备份
            </Button>
          )}
          <Button variant={result ? "default" : "outline"} onClick={onClose}>
            {result ? "完成" : "取消"}
          </Button>
          {!result && (
            <Button onClick={clean} disabled={busy || chosen.length === 0}>
              {busy ? <LoaderCircleIcon className="animate-spin" /> : <BrushCleaningIcon />}
              清理
              {chosen.length > 0 && <span className="tabular-nums">（{chosen.length}）</span>}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function Row({
  item: it,
  checked,
  disabled,
  outcome,
  onCheck,
}: {
  item: LeftoverItem;
  checked: boolean;
  disabled: boolean;
  outcome?: { ok: boolean; error?: string };
  onCheck: (on: boolean) => void;
}) {
  const kind = kinds[it.kind];
  const Icon = kind.icon;
  return (
    <label
      className={cn(
        "flex cursor-pointer items-start gap-3 rounded-lg px-2 py-2 transition-colors hover:bg-muted/50",
        disabled && "cursor-default",
      )}
    >
      <Checkbox
        className="mt-0.5"
        checked={checked}
        disabled={disabled}
        onCheckedChange={(v) => onCheck(v === true)}
        aria-label={`清理 ${it.path}`}
      />
      <Icon className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
      <div className="min-w-0 flex-1">
        <Tooltip>
          <TooltipTrigger asChild>
            <div className="truncate font-mono text-xs select-text">
              {it.path}
              {it.value && <span className="text-primary"> · {it.value}</span>}
            </div>
          </TooltipTrigger>
          <TooltipContent className="max-w-[32rem] font-mono break-all">
            {it.path}
            {it.value && ` · ${it.value}`}
          </TooltipContent>
        </Tooltip>
        <div className="mt-1 flex flex-wrap items-center gap-1">
          <span className="text-[11px] text-muted-foreground">{kind.label}</span>
          {it.reasons.map((r) => (
            <span
              key={r.text}
              className={cn(
                "rounded px-1.5 text-[11px] leading-[18px] tabular-nums",
                r.delta > 0 ? "bg-success/10 text-success" : "bg-warning/12 text-warning",
              )}
            >
              {r.text} {r.delta > 0 ? `+${r.delta}` : r.delta}
            </span>
          ))}
        </div>
        {outcome && !outcome.ok && (
          <div className="mt-1 text-xs text-destructive">{outcome.error}</div>
        )}
      </div>
      {outcome ? (
        outcome.ok ? (
          <Pill tone="success">
            <CheckCircle2Icon />
            已清理
          </Pill>
        ) : (
          <Pill tone="danger">
            <XCircleIcon />
            未清理
          </Pill>
        )
      ) : (
        <span
          className={cn(
            "shrink-0 text-xs font-medium tabular-nums",
            it.score >= 4
              ? "text-success"
              : it.score > 0
                ? "text-muted-foreground"
                : "text-warning",
          )}
        >
          {it.score > 0 ? `+${it.score}` : it.score} 分
        </span>
      )}
    </label>
  );
}
