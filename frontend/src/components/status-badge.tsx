import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { statusView, type Tone } from "@/lib/shelf";
import type { AppStatus } from "@/mygo";
import type { ReactNode } from "react";
import {
  ArrowUpCircleIcon,
  BanIcon,
  CheckCircle2Icon,
  CircleDashedIcon,
  CircleIcon,
  LoaderCircleIcon,
  TriangleAlertIcon,
} from "lucide-react";

export const toneClasses: Record<Tone, string> = {
  primary: "bg-primary/12 text-primary ring-primary/25",
  success: "bg-success/12 text-success ring-success/25",
  warning: "bg-warning/14 text-warning ring-warning/30",
  muted: "bg-muted text-muted-foreground ring-border",
  danger: "bg-destructive/10 text-destructive ring-destructive/25",
};

/** Pill is a small rounded label of a tone. */
export function Pill({
  tone,
  className,
  children,
}: {
  tone: Tone;
  className?: string;
  children: ReactNode;
}) {
  return (
    <span
      className={cn(
        "inline-flex h-5 shrink-0 items-center gap-1 rounded-full px-2 text-[11px] font-medium whitespace-nowrap ring-1 ring-inset [&_svg]:size-3",
        toneClasses[tone],
        className,
      )}
    >
      {children}
    </span>
  );
}

/** StatusBadge shows an app's state, its note in a tooltip. */
export function StatusBadge({
  status,
  checking,
  className,
}: {
  status: AppStatus | null;
  checking: boolean;
  className?: string;
}) {
  const v = statusView(status, checking);
  const icon = checking ? (
    <LoaderCircleIcon className="animate-spin" />
  ) : !status ? (
    <CircleDashedIcon />
  ) : status.state === "updateAvailable" ? (
    <ArrowUpCircleIcon />
  ) : status.state === "installed" ? (
    <CheckCircle2Icon />
  ) : status.state === "notInstalled" ? (
    <CircleIcon />
  ) : status.state === "unavailable" ? (
    <BanIcon />
  ) : (
    <TriangleAlertIcon />
  );
  const pill = (
    <Pill tone={v.tone} className={className}>
      {icon}
      {v.label}
    </Pill>
  );
  if (!v.detail) return pill;
  return (
    <Tooltip>
      <TooltipTrigger asChild>{pill}</TooltipTrigger>
      <TooltipContent className="max-w-80">{v.detail}</TooltipContent>
    </Tooltip>
  );
}
