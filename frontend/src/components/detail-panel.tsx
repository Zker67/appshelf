// The details column of a page: the item's icon and name, its actions,
// then what is known of it, scrolling on its own.

import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { XIcon } from "lucide-react";
import type { ReactNode } from "react";

export function DetailPanel({
  icon,
  title,
  badges,
  actions,
  onClose,
  children,
}: {
  icon: ReactNode;
  title: string;
  badges?: ReactNode;
  actions?: ReactNode;
  onClose: () => void;
  children: ReactNode;
}) {
  return (
    <div className="flex h-full min-h-0 flex-col bg-surface">
      <div className="shrink-0 border-b bg-linear-to-b from-primary/[0.08] via-primary/[0.03] to-transparent dark:from-primary/[0.14] dark:via-primary/[0.05]">
        <div className="anim-panel flex items-center gap-3.5 px-5 pt-5 pb-4">
          <div className="anim-rise shrink-0 rounded-[28%] bg-card p-1.5 shadow-soft ring-1 ring-border/70 [animation-delay:60ms]">
            {icon}
          </div>
          <div className="min-w-0 flex-1">
            <div className="text-[17px] leading-snug font-semibold tracking-tight break-words select-text">
              {title}
            </div>
            {badges && <div className="mt-1.5 flex flex-wrap items-center gap-1.5">{badges}</div>}
          </div>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon-sm"
                className="-mt-1 -mr-2 self-start text-muted-foreground transition-transform hover:rotate-90"
                onClick={onClose}
                aria-label="关闭详情"
              >
                <XIcon />
              </Button>
            </TooltipTrigger>
            <TooltipContent>关闭详情</TooltipContent>
          </Tooltip>
        </div>
        {actions && (
          <div className="anim-panel flex flex-wrap items-center gap-2 px-5 pb-4 [animation-delay:50ms]">
            {actions}
          </div>
        )}
      </div>
      <ScrollArea className="min-h-0 flex-1">
        <div className="anim-panel px-5 py-4 [animation-delay:100ms]">{children}</div>
      </ScrollArea>
    </div>
  );
}
