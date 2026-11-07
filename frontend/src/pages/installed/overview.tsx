// The overview of Installed: how many programs, from which sources, how
// many have updates, how many the catalog has, and how many have no source.
// Each part opens the list on what it counts.

import { AppIcon } from "@/components/app-icon";
import { CountUp } from "@/components/common";
import { SourceBadge, SourceIcon } from "@/components/source-badge";
import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import { cn } from "@/lib/utils";
import type { InstalledEntry, InstalledKind } from "@/mygo";
import {
  ArrowRightIcon,
  ArrowUpCircleIcon,
  CheckCircle2Icon,
  CircleDashedIcon,
  CircleHelpIcon,
  LayoutGridIcon,
  PlusIcon,
  type LucideIcon,
} from "lucide-react";
import type { ReactNode } from "react";

/** The list an overview part opens: a source, a state, or everything. */
export interface ListFilter {
  kind?: InstalledKind;
  state?: "update" | "notInShelf" | "inShelf" | "noSource";
}

const kindLabels: Array<{ value: InstalledKind; label: string }> = [
  { value: "winget", label: "winget" },
  { value: "scoop", label: "scoop" },
  { value: "local", label: "本机程序" },
  { value: "store", label: "应用商店" },
];

/** How many update rows the overview lists before "查看全部". */
const updateRows = 6;

export function InstalledOverview({
  entries,
  at,
  onOpen,
  onUpdate,
  onAdd,
}: {
  entries: InstalledEntry[];
  /** at is when the programs were read, formatted. */
  at: string;
  onOpen: (f: ListFilter) => void;
  onUpdate: (keys: string[]) => void;
  onAdd: (entries: InstalledEntry[]) => void;
}) {
  const total = entries.length;
  const updates = entries.filter((e) => !!e.update);
  const updatable = updates.filter((e) => e.source);
  const inShelf = entries.filter((e) => e.inShelf.length > 0).length;
  const notInShelf = entries.filter((e) => e.inShelf.length === 0);
  const addable = notInShelf.filter((e) => e.source);
  const noSource = entries.filter((e) => !e.source).length;
  const kinds = kindLabels
    .map((k) => ({ ...k, count: entries.filter((e) => e.kind === k.value).length }))
    .filter((k) => k.count > 0);
  const most = Math.max(1, ...kinds.map((k) => k.count));

  return (
    <ScrollArea className="min-h-0 flex-1">
      <div className="anim-fade mx-auto flex max-w-5xl flex-col gap-4 p-5 pb-20">
        <div className="grid grid-cols-2 gap-3 @3xl:grid-cols-4">
          <StatTile
            icon={LayoutGridIcon}
            label="全部程序"
            value={total}
            hint={at ? `读取于 ${at}` : "正在读取"}
            index={0}
            onClick={() => onOpen({})}
          />
          <StatTile
            icon={ArrowUpCircleIcon}
            brand
            label="可更新"
            value={updates.length}
            hint={updates.length === 0 ? "都是最新版本" : `其中 ${updatable.length} 个可直接更新`}
            index={1}
            onClick={() => onOpen({ state: "update" })}
          />
          <StatTile
            icon={CircleDashedIcon}
            label="未加入清单"
            value={notInShelf.length}
            hint={`其中 ${addable.length} 个有来源，可直接加入`}
            index={2}
            onClick={() => onOpen({ state: "notInShelf" })}
          />
          <StatTile
            icon={CircleHelpIcon}
            label="无来源"
            value={noSource}
            hint="没有 winget / scoop 来源，可查找来源"
            index={3}
            onClick={() => onOpen({ state: "noSource" })}
          />
        </div>

        <div className="anim-rise grid gap-3 [animation-delay:160ms] @3xl:grid-cols-2">
          <Card title="来源分布">
            <div className="flex flex-col gap-0.5">
              {kinds.map((k, i) => (
                <button
                  key={k.value}
                  type="button"
                  onClick={() => onOpen({ kind: k.value })}
                  className="group/bar grid grid-cols-[6.5rem_1fr_auto] items-center gap-3 rounded-lg px-2 py-1.5 text-left text-[13px] transition-colors hover:bg-foreground/[0.04]"
                >
                  <span className="flex min-w-0 items-center gap-2">
                    <SourceIcon type={k.value} className="size-4 shrink-0" />
                    <span className="truncate">{k.label}</span>
                  </span>
                  <span className="h-2 overflow-hidden rounded-full bg-foreground/[0.06]">
                    <span
                      className="anim-grow-x block h-full rounded-full bg-linear-to-r from-brand-from to-brand-to opacity-80 transition-opacity group-hover/bar:opacity-100"
                      style={{
                        width: `${(k.count / most) * 100}%`,
                        animationDelay: `${240 + i * 70}ms`,
                      }}
                    />
                  </span>
                  <span className="w-20 text-right text-xs text-muted-foreground tabular-nums">
                    <span className="font-medium text-foreground">{k.count}</span> 个 ·{" "}
                    {percent(k.count, total)}
                  </span>
                </button>
              ))}
            </div>
          </Card>

          <Card title="清单覆盖">
            <div className="flex items-baseline gap-2 px-2">
              <span className="text-2xl font-semibold tabular-nums">
                <CountUp value={inShelf} />
              </span>
              <span className="text-sm text-muted-foreground">
                / {total} 个已在清单 · {percent(inShelf, total)}
              </span>
            </div>
            <div className="mx-2 mt-2.5 h-2 overflow-hidden rounded-full bg-foreground/[0.06]">
              <div
                className="anim-grow-x h-full rounded-full bg-success/80 [animation-delay:300ms]"
                style={{ width: `${total ? (inShelf / total) * 100 : 0}%` }}
              />
            </div>
            <div className="mt-4 flex flex-wrap items-center gap-2 px-2 text-sm text-muted-foreground">
              <span>
                未加入清单且有来源的
                <span className="mx-1 font-medium text-foreground tabular-nums">
                  {addable.length}
                </span>
                个
              </span>
              <div className="ml-auto flex items-center gap-1.5">
                <Button variant="ghost" size="sm" onClick={() => onOpen({ state: "notInShelf" })}>
                  查看
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={addable.length === 0}
                  onClick={() => onAdd(addable)}
                >
                  <PlusIcon />
                  全部加入清单
                </Button>
              </div>
            </div>
          </Card>
        </div>

        <Card
          className="anim-rise [animation-delay:220ms]"
          title={`可更新 ${updates.length}`}
          action={
            updates.length > 0 && (
              <div className="flex items-center gap-1.5">
                <Button variant="ghost" size="sm" onClick={() => onOpen({ state: "update" })}>
                  查看全部
                  <ArrowRightIcon />
                </Button>
                <Button
                  size="sm"
                  disabled={updatable.length === 0}
                  onClick={() => onUpdate(updatable.map((e) => e.key))}
                >
                  <ArrowUpCircleIcon />
                  全部更新{updatable.length > 0 && ` ${updatable.length}`}
                </Button>
              </div>
            )
          }
        >
          {updates.length === 0 ? (
            <div className="flex items-center gap-2 px-2 py-3 text-sm text-muted-foreground">
              <CheckCircle2Icon className="size-4 text-success" />
              没有可更新的程序
            </div>
          ) : (
            <div className="flex flex-col">
              {updates.slice(0, updateRows).map((e) => (
                <div
                  key={e.key}
                  className="flex h-11 items-center gap-3 border-b border-border/60 px-2 last:border-b-0"
                >
                  <AppIcon path={e.icon} name={e.name} size={24} />
                  <span className="min-w-0 flex-1 truncate text-[13px] font-medium">{e.name}</span>
                  <SourceBadge type={e.kind} />
                  <span className="w-44 shrink-0 truncate text-right font-mono text-xs text-muted-foreground">
                    {e.version ?? "?"} → <span className="text-primary">{e.update}</span>
                  </span>
                </div>
              ))}
              {updates.length > updateRows && (
                <button
                  type="button"
                  onClick={() => onOpen({ state: "update" })}
                  className="mt-1 rounded-lg px-2 py-1.5 text-left text-xs text-muted-foreground transition-colors hover:bg-foreground/[0.04] hover:text-foreground"
                >
                  还有 {updates.length - updateRows} 个，查看全部
                </button>
              )}
            </div>
          )}
        </Card>
      </div>
    </ScrollArea>
  );
}

function percent(n: number, total: number): string {
  return total ? `${Math.round((n / total) * 100)}%` : "0%";
}

function StatTile({
  icon: Icon,
  brand,
  label,
  value,
  hint,
  index,
  onClick,
}: {
  icon: LucideIcon;
  /** brand marks the tile that matters most, in the icon's colors. */
  brand?: boolean;
  label: string;
  value: number;
  hint: string;
  /** index staggers the tiles coming in. */
  index: number;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      style={{ animationDelay: `${index * 50}ms` }}
      className="group/tile anim-rise flex flex-col gap-1.5 rounded-xl border bg-card p-4 text-left shadow-soft transition-[transform,box-shadow,border-color] duration-300 ease-[var(--ease-out-soft)] outline-none hover:-translate-y-0.5 hover:border-primary/30 hover:shadow-lift focus-visible:ring-2 focus-visible:ring-ring/50 active:translate-y-0"
    >
      <span className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
        <span
          className={cn(
            "flex size-7 items-center justify-center rounded-lg transition-transform duration-300 ease-[var(--ease-spring)] group-hover/tile:scale-110 group-hover/tile:-rotate-6",
            brand
              ? "bg-linear-to-br from-brand-from to-brand-to text-brand-foreground shadow-brand"
              : "bg-primary/10 text-primary",
          )}
        >
          <Icon className="size-4" />
        </span>
        {label}
        <ArrowRightIcon className="ml-auto size-3.5 -translate-x-1 opacity-0 transition-[opacity,translate] duration-300 group-hover/tile:translate-x-0 group-hover/tile:opacity-100" />
      </span>
      <span className="mt-1 text-[26px] leading-tight font-semibold tracking-tight tabular-nums">
        <CountUp value={value} />
      </span>
      <span className="truncate text-xs text-muted-foreground">{hint}</span>
    </button>
  );
}

function Card({
  title,
  action,
  className,
  children,
}: {
  title: string;
  action?: ReactNode;
  className?: string;
  children: ReactNode;
}) {
  return (
    <section className={cn("flex flex-col rounded-xl border bg-card p-3 shadow-soft", className)}>
      <div className="mb-2 flex h-8 items-center gap-2 px-2">
        <h3 className="text-sm font-semibold">{title}</h3>
        <div className="ml-auto">{action}</div>
      </div>
      {children}
    </section>
  );
}
