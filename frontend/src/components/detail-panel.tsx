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
      <div className="anim-panel flex items-start gap-3 px-4 pt-4 pb-3">
        <div className="anim-rise shrink-0 [animation-delay:60ms]">{icon}</div>
        <div className="min-w-0 flex-1 pt-0.5">
          <div className="text-[15px] leading-snug font-semibold break-words select-text">
            {title}
          </div>
          {badges && <div className="mt-1.5 flex flex-wrap items-center gap-1.5">{badges}</div>}
        </div>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              className="-mt-1 -mr-1 text-muted-foreground"
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
        <div className="anim-panel flex flex-wrap items-center gap-1.5 border-b px-4 pb-3 [animation-delay:50ms]">
          {actions}
        </div>
      )}
      <ScrollArea className="min-h-0 flex-1">
        <div className="anim-panel p-4 [animation-delay:100ms]">{children}</div>
      </ScrollArea>
    </div>
  );
}
