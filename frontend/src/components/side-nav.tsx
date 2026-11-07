// The left side of a page: groups of filter rows, and help and settings at
// the bottom, so that they sit in the window's lower left on every page.

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { useActions } from "@/state/actions";
import { CircleHelpIcon, SettingsIcon } from "lucide-react";
import type { ComponentType, ReactNode } from "react";

export function SideGroup({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-0.5">
      <div className="flex h-7 items-center pl-2.5 text-[11px] font-semibold tracking-wide text-muted-foreground">
        {title}
      </div>
      {children}
    </div>
  );
}

export function SideRow({
  icon: Icon,
  iconClass,
  label,
  count,
  trailing,
  active,
  onClick,
}: {
  icon: ComponentType<{ className?: string }>;
  iconClass?: string;
  label: string;
  count?: number;
  /** trailing shows in place of the count, as a search still running. */
  trailing?: ReactNode;
  active: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      aria-pressed={active}
      onClick={onClick}
      className={cn(
        "side-row relative flex h-8 items-center gap-2 rounded-lg pr-2 pl-2.5 text-left text-[13px] transition-colors duration-150 outline-none focus-visible:ring-2 focus-visible:ring-ring/50",
        active
          ? "bg-linear-to-r from-primary/14 to-primary/[0.03] font-medium text-foreground ring-1 ring-primary/10 ring-inset dark:from-primary/24 dark:to-primary/[0.05]"
          : "text-foreground/80 hover:bg-foreground/[0.05] hover:text-foreground",
      )}
    >
      <Icon
        className={cn(
          "size-4 shrink-0 transition-colors",
          active ? "text-primary" : (iconClass ?? "text-muted-foreground"),
        )}
      />
      <span className="min-w-0 truncate">{label}</span>
      <span
        className={cn(
          "ml-auto flex items-center text-xs tabular-nums transition-colors",
          active && trailing === undefined && count !== undefined
            ? "min-w-5 justify-center rounded-full bg-linear-to-r from-brand-from to-brand-to px-1.5 leading-[18px] font-medium text-brand-foreground shadow-brand"
            : active
              ? "text-primary"
              : "text-muted-foreground",
        )}
      >
        {trailing ?? count}
      </span>
    </button>
  );
}

/** SettingsFooter is the bottom of a page's left side: help, then settings. */
export function SettingsFooter() {
  const actions = useActions();
  return (
    <div className="flex shrink-0 flex-col gap-0.5 border-t p-2.5">
      <Button
        variant="ghost"
        size="sm"
        className="w-full justify-start gap-2 text-muted-foreground hover:text-foreground"
        onClick={actions.openHelp}
      >
        <CircleHelpIcon />
        使用说明
      </Button>
      <Button
        variant="ghost"
        size="sm"
        className="w-full justify-start gap-2 text-muted-foreground hover:text-foreground"
        onClick={() => actions.openSettings()}
      >
        <SettingsIcon />
        设置
      </Button>
    </div>
  );
}
