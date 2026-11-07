// Small pieces the pages share: empty states, the selection bar, the
// search box, copying.

import { Button } from "@/components/ui/button";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "@/components/ui/input-group";
import { Kbd } from "@/components/ui/kbd";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { CheckIcon, CopyIcon, SearchIcon, XIcon, type LucideIcon } from "lucide-react";
import { useEffect, useRef, useState, type ReactNode, type Ref } from "react";
import { toast } from "sonner";

/** EmptyState fills its area, centered, with what to do next. */
export function EmptyState({
  icon: Icon,
  title,
  hint,
  children,
  className,
}: {
  icon: LucideIcon;
  title: string;
  hint?: string;
  children?: ReactNode;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "anim-rise flex h-full min-h-60 flex-col items-center justify-center gap-3 p-8 text-center",
        className,
      )}
    >
      <div className="anim-float relative mb-1">
        <div className="flex size-16 items-center justify-center rounded-2xl bg-linear-to-br from-brand-from to-brand-to text-brand-foreground shadow-brand">
          <Icon className="size-7" strokeWidth={1.6} />
        </div>
        <Sparkle className="sparkle absolute -top-1.5 -right-1.5 size-3.5 text-brand-to" />
      </div>
      <div className="text-base font-semibold">{title}</div>
      {hint && <div className="max-w-sm text-sm text-muted-foreground">{hint}</div>}
      {children && (
        <div className="mt-2 flex flex-wrap items-center justify-center gap-2">{children}</div>
      )}
    </div>
  );
}

/** Sparkle is the four-pointed star of the app icon. */
function Sparkle({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 16 16" className={className} aria-hidden>
      <path
        fill="currentColor"
        d="M8 0c.5 3.9 2.1 5.5 6 6-3.9.5-5.5 2.1-6 6-.5-3.9-2.1-5.5-6-6 3.9-.5 5.5-2.1 6-6Z"
        transform="translate(0 2)"
      />
    </svg>
  );
}

/** CountUp shows a number rolling from the one it showed to the new one. */
export function CountUp({ value, duration = 700 }: { value: number; duration?: number }) {
  const [shown, setShown] = useState(0);
  const from = useRef(0);
  useEffect(() => {
    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
      from.current = value;
      setShown(value);
      return;
    }
    const start = performance.now();
    const begin = from.current;
    let frame = 0;
    const tick = (now: number) => {
      const t = Math.min(1, (now - start) / duration);
      const eased = 1 - (1 - t) ** 3;
      const next = Math.round(begin + (value - begin) * eased);
      from.current = next;
      setShown(next);
      if (t < 1) frame = requestAnimationFrame(tick);
    };
    frame = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(frame);
  }, [value, duration]);
  return <>{shown}</>;
}

/** SelectionBar floats over the bottom of a list while items are selected. */
export function SelectionBar({
  count,
  onClear,
  children,
}: {
  count: number;
  onClear: () => void;
  children: ReactNode;
}) {
  if (count === 0) return null;
  return (
    <div className="anim-bar absolute bottom-4 left-1/2 z-20 flex max-w-[calc(100%-2rem)] items-center gap-1.5 rounded-xl border bg-popover/95 p-1.5 pl-3.5 shadow-xl shadow-black/10 backdrop-blur supports-[backdrop-filter]:bg-popover/85 dark:shadow-black/40">
      <span className="mr-1.5 text-sm font-medium whitespace-nowrap">
        已选 <span className="text-primary tabular-nums">{count}</span> 项
      </span>
      <div className="flex items-center gap-1">{children}</div>
      <div className="mx-0.5 h-5 w-px bg-border" />
      <Tooltip>
        <TooltipTrigger asChild>
          <Button variant="ghost" size="icon-sm" onClick={onClear} aria-label="取消选择">
            <XIcon />
          </Button>
        </TooltipTrigger>
        <TooltipContent>取消选择</TooltipContent>
      </Tooltip>
    </div>
  );
}

/** SearchBox is a page's filter or search field, focused by Ctrl+F or /. */
export function SearchBox({
  value,
  onChange,
  onEnter,
  placeholder,
  inputRef,
  className,
  size = "default",
  busy,
}: {
  value: string;
  onChange: (v: string) => void;
  onEnter?: () => void;
  placeholder: string;
  inputRef?: Ref<HTMLInputElement>;
  className?: string;
  size?: "default" | "lg";
  busy?: boolean;
}) {
  return (
    <InputGroup
      className={cn(
        "bg-background/70 transition-shadow focus-within:shadow-sm",
        size === "lg" ? "h-10" : "h-8",
        className,
      )}
    >
      <InputGroupAddon>
        <SearchIcon className={cn(busy && "animate-pulse text-primary")} />
      </InputGroupAddon>
      <InputGroupInput
        ref={inputRef}
        value={value}
        placeholder={placeholder}
        spellCheck={false}
        autoComplete="off"
        onChange={(e) => onChange(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") onEnter?.();
          if (e.key === "Escape" && value) {
            e.stopPropagation();
            onChange("");
          }
        }}
      />
      <InputGroupAddon align="inline-end">
        {value ? (
          <InputGroupButton size="icon-xs" aria-label="清除" onClick={() => onChange("")}>
            <XIcon />
          </InputGroupButton>
        ) : (
          <Kbd className="hidden sm:inline-flex">Ctrl F</Kbd>
        )}
      </InputGroupAddon>
    </InputGroup>
  );
}

/** copyText copies text and says so. */
export async function copyText(text: string, what = "内容"): Promise<void> {
  try {
    await navigator.clipboard.writeText(text);
    toast.success(`已复制${what}`);
  } catch {
    toast.error("无法写入剪贴板");
  }
}

/** CopyButton copies a value, its icon turning to a check for a moment. */
export function CopyButton({
  text,
  what,
  className,
}: {
  text: string;
  what?: string;
  className?: string;
}) {
  const [done, setDone] = useState(false);
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size="icon-xs"
          className={cn("text-muted-foreground", className)}
          aria-label="复制"
          onClick={(e) => {
            e.stopPropagation();
            void copyText(text, what);
            setDone(true);
            window.setTimeout(() => setDone(false), 1200);
          }}
        >
          {done ? <CheckIcon className="text-success" /> : <CopyIcon />}
        </Button>
      </TooltipTrigger>
      <TooltipContent>复制</TooltipContent>
    </Tooltip>
  );
}

/** Chip is a small summary label of a toolbar. */
export function Chip({
  icon: Icon,
  children,
  tone,
  className,
}: {
  icon?: LucideIcon;
  children: ReactNode;
  tone?: "primary";
  className?: string;
}) {
  return (
    <span
      className={cn(
        "inline-flex h-7 shrink-0 items-center gap-1.5 rounded-lg border bg-card px-2.5 text-xs font-medium whitespace-nowrap text-muted-foreground shadow-xs",
        tone === "primary" && "border-primary/25 bg-primary/8 text-primary",
        className,
      )}
    >
      {Icon && <Icon className="size-3.5" />}
      {children}
    </span>
  );
}

/** Field is "label：value" of a details panel. */
export function InfoRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid grid-cols-[5.5rem_minmax(0,1fr)] items-start gap-2 py-1.5 text-sm">
      <div className="pt-px text-muted-foreground">{label}</div>
      <div className="min-w-0 break-words select-text">{children}</div>
    </div>
  );
}
