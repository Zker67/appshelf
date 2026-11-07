// How a package is updated, in a menu: tell me, update on its own, or
// ignore; skip this version, put it off a week, or offer it again.

import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { errorText } from "@/lib/api";
import { modeLabels } from "@/lib/updates";
import { cn } from "@/lib/utils";
import { Updates, type UpdateHold, type UpdateMode } from "@/mygo";
import {
  BellIcon,
  BellOffIcon,
  ChevronDownIcon,
  Clock3Icon,
  RotateCcwIcon,
  SkipForwardIcon,
  ZapIcon,
  type LucideIcon,
} from "lucide-react";
import { toast } from "sonner";

/** What the menu changes: a package, by its source key. */
export interface UpdateTarget {
  key: string;
  name: string;
  mode: UpdateMode;
  hold?: UpdateHold;
  /** latest is the version offered, when there is one. */
  latest?: string;
  /** canAuto false disables updating on its own, for autoNote; unknown
   * leaves it to Go to say. */
  canAuto?: boolean;
  autoNote?: string;
}

export const modeIcons: Record<UpdateMode, LucideIcon> = {
  notify: BellIcon,
  auto: ZapIcon,
  ignore: BellOffIcon,
};

const modeHints: Record<UpdateMode, string> = {
  notify: "有新版本时提醒，由你决定何时更新",
  auto: "检查到更新后自动安装",
  ignore: "不再提醒它的更新",
};

/** run calls Go and says how it went, with an undo when there is one. */
async function run(fn: () => Promise<void>, done: string, undo?: () => Promise<void>) {
  try {
    await fn();
    toast.success(
      done,
      undo && { action: { label: "撤销", onClick: () => void run(undo, "已撤销") } },
    );
  } catch (err) {
    toast.error("没有改成", { description: errorText(err) });
  }
}

export function UpdateMenu({
  target: t,
  align = "end",
  className,
}: {
  target: UpdateTarget;
  align?: "start" | "end";
  className?: string;
}) {
  const Icon = modeIcons[t.mode];
  const setMode = (mode: string) => {
    const m = mode as UpdateMode;
    if (m === t.mode) return;
    const prev = t.mode;
    const done =
      m === "auto"
        ? `「${t.name}」将自动更新`
        : m === "ignore"
          ? `已忽略「${t.name}」的更新`
          : `「${t.name}」有更新时会提醒你`;
    void run(
      () => Updates.setMode(t.key, m),
      done,
      () => Updates.setMode(t.key, prev),
    );
  };
  const held = !!t.hold || t.mode === "ignore";
  return (
    <DropdownMenu>
      <Tooltip>
        <TooltipTrigger asChild>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="xs"
              onClick={(ev) => ev.stopPropagation()}
              className={cn(
                "shrink-0 gap-1 text-muted-foreground hover:text-foreground data-[state=open]:bg-foreground/[0.06] data-[state=open]:text-foreground",
                t.mode === "auto" && "text-primary hover:text-primary",
                className,
              )}
              aria-label={`更新方式：${modeLabels[t.mode]}`}
            >
              <Icon />
              {t.mode !== "notify" && modeLabels[t.mode]}
              <ChevronDownIcon className="opacity-60" />
            </Button>
          </DropdownMenuTrigger>
        </TooltipTrigger>
        <TooltipContent>更新方式：{modeLabels[t.mode]}</TooltipContent>
      </Tooltip>
      <DropdownMenuContent align={align} className="w-64" onClick={(ev) => ev.stopPropagation()}>
        <DropdownMenuLabel className="text-xs text-muted-foreground">更新方式</DropdownMenuLabel>
        <DropdownMenuRadioGroup value={t.mode} onValueChange={setMode}>
          {(["notify", "auto", "ignore"] as const).map((m) => {
            const MIcon = modeIcons[m];
            const blocked = m === "auto" && t.canAuto === false;
            return (
              <DropdownMenuRadioItem
                key={m}
                value={m}
                disabled={blocked}
                className="items-start py-1.5"
              >
                <MIcon className={cn("mt-0.5", m === "auto" && "text-primary")} />
                <span className="flex min-w-0 flex-col">
                  <span>{modeLabels[m]}</span>
                  <span className="text-xs leading-snug text-muted-foreground">
                    {blocked ? (t.autoNote ?? "这个软件不能自动更新") : modeHints[m]}
                  </span>
                </span>
              </DropdownMenuRadioItem>
            );
          })}
        </DropdownMenuRadioGroup>
        {(t.latest || held) && <DropdownMenuSeparator />}
        {t.latest && t.mode !== "ignore" && t.hold !== "skip" && (
          <DropdownMenuItem
            onSelect={() =>
              void run(
                () => Updates.skip(t.key),
                `已跳过「${t.name}」${t.latest}，有更新的版本时再提醒`,
                () => Updates.restore(t.key),
              )
            }
          >
            <SkipForwardIcon />
            跳过 {t.latest}
          </DropdownMenuItem>
        )}
        {t.latest && t.mode !== "ignore" && t.hold !== "snooze" && (
          <DropdownMenuItem
            onSelect={() =>
              void run(
                () => Updates.snooze(t.key, 7),
                `7 天内不再提醒「${t.name}」`,
                () => Updates.restore(t.key),
              )
            }
          >
            <Clock3Icon />7 天内不提醒
          </DropdownMenuItem>
        )}
        {held && (
          <DropdownMenuItem
            onSelect={() => void run(() => Updates.restore(t.key), `已恢复提醒「${t.name}」的更新`)}
          >
            <RotateCcwIcon />
            恢复提醒
          </DropdownMenuItem>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
