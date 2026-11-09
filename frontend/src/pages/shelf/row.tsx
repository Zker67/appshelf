// A row of the catalog: the app at the left, its source below its name,
// a mark when it has a note, its groups and state at the right, a quick
// install or update on hover; its menu on right click.

import { AppIcon } from "@/components/app-icon";
import { copyText } from "@/components/common";
import { SourceBadge } from "@/components/source-badge";
import { StatusBadge } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  ContextMenu,
  ContextMenuCheckboxItem,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuRadioGroup,
  ContextMenuRadioItem,
  ContextMenuSeparator,
  ContextMenuSub,
  ContextMenuSubContent,
  ContextMenuSubTrigger,
  ContextMenuTrigger,
} from "@/components/ui/context-menu";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { noteSummary } from "@/lib/markdown";
import { sourceID } from "@/lib/shelf";
import { cn } from "@/lib/utils";
import type { App, CategoryInfo, ShelfApp, TagInfo } from "@/mygo";
import {
  ArrowUpCircleIcon,
  CopyIcon,
  DownloadIcon,
  FolderIcon,
  HashIcon,
  LoaderCircleIcon,
  MinusIcon,
  NotebookPenIcon,
  PanelRightIcon,
  PencilIcon,
  Trash2Icon,
} from "lucide-react";
import type { MouseEvent } from "react";
import { assignCategory, assignTag } from "./ops";

export interface RowActions {
  open: (id: string) => void;
  clickRow: (id: string, e: MouseEvent) => void;
  check: (id: string, on: boolean, shift: boolean) => void;
  /** targets are the apps a menu acts on: the selection, when the row is in it. */
  targets: (id: string) => ShelfApp[];
  install: (ids: string[]) => void;
  upgrade: (ids: string[]) => void;
  edit: (app: App) => void;
  /** note writes the app's own note. */
  note: (item: ShelfApp) => void;
  remove: (apps: ShelfApp[]) => void;
}

export function ShelfRow({
  item,
  categories,
  tags,
  checked,
  focused,
  selecting,
  act,
}: {
  item: ShelfApp;
  categories: Map<string, CategoryInfo>;
  tags: Map<string, TagInfo>;
  checked: boolean;
  focused: boolean;
  selecting: boolean;
  act: RowActions;
}) {
  const { app, status } = item;
  const quick = item.pending
    ? null
    : status?.state === "updateAvailable"
      ? "upgrade"
      : status?.state === "notInstalled"
        ? "install"
        : null;
  const cat = app.categoryId ? categories.get(app.categoryId) : undefined;
  const tagList = (app.tagIds ?? []).map((t) => tags.get(t)).filter((t): t is TagInfo => !!t);
  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>
        <div
          data-row={app.id}
          onClick={(e) => act.clickRow(app.id, e)}
          data-checked={checked}
          className={cn(
            "row-lazy list-row group/row relative flex h-[60px] items-center gap-3 pr-3 pl-3 select-none",
            focused && "row-focus",
          )}
        >
          <Checkbox
            checked={checked}
            aria-label={`选择 ${app.name}`}
            onClick={(e) => {
              e.preventDefault();
              e.stopPropagation();
              act.check(app.id, !checked, e.shiftKey);
            }}
            className={cn(
              "transition-opacity",
              checked || selecting
                ? "opacity-100"
                : "opacity-0 group-hover/row:opacity-100 focus-visible:opacity-100",
            )}
          />
          <AppIcon path={item.icon} name={app.name} size={36} />
          <div className="min-w-0 flex-1">
            <div className="flex min-w-0 items-baseline gap-2">
              <span className="truncate text-sm font-medium">{app.name}</span>
              {app.note && (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <NotebookPenIcon
                      aria-label="有备注"
                      className="size-3.5 shrink-0 self-center text-primary/70"
                    />
                  </TooltipTrigger>
                  <TooltipContent className="max-w-72">
                    {noteSummary(app.note) || "有备注"}
                  </TooltipContent>
                </Tooltip>
              )}
              {(status?.version || status?.state === "updateAvailable") && (
                <span className="shrink-0 text-xs text-muted-foreground tabular-nums">
                  {status.version || "?"}
                  {status.state === "updateAvailable" && status.latest && (
                    <span className="text-primary"> → {status.latest}</span>
                  )}
                </span>
              )}
            </div>
            <div className="mt-1 flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground">
              <SourceBadge type={app.source.type} />
              <span className="truncate font-mono">{sourceID(app.source)}</span>
            </div>
          </div>
          <div className="hidden max-w-[38%] shrink items-center justify-end gap-1 @3xl:flex">
            {cat && (
              <span className="inline-flex h-5 min-w-0 items-center gap-1 rounded-md bg-secondary px-1.5 text-[11px] text-secondary-foreground">
                <FolderIcon className="size-3 shrink-0 opacity-70" />
                <span className="truncate">{cat.name}</span>
              </span>
            )}
            {tagList.slice(0, 2).map((t) => (
              <span
                key={t.id}
                className="inline-flex h-5 min-w-0 items-center gap-0.5 rounded-md border border-border/80 px-1.5 text-[11px] text-muted-foreground"
              >
                <HashIcon className="size-2.5 shrink-0" />
                <span className="truncate">{t.name}</span>
              </span>
            ))}
            {tagList.length > 2 && (
              <span className="text-[11px] text-muted-foreground">+{tagList.length - 2}</span>
            )}
          </div>
          <div className="flex w-[118px] shrink-0 justify-end">
            {item.pending ? (
              <span className="inline-flex h-5 items-center gap-1 rounded-full bg-primary/12 px-2 text-[11px] font-medium text-primary ring-1 ring-primary/25 ring-inset">
                <LoaderCircleIcon className="size-3 animate-spin" />
                任务中
              </span>
            ) : (
              <StatusBadge status={status} checking={item.checking} hold={item.hold} />
            )}
          </div>
          <div className="flex w-[64px] shrink-0 justify-end">
            {quick && (
              <Button
                size="xs"
                variant={quick === "upgrade" ? "default" : "secondary"}
                className="opacity-0 transition-opacity group-hover/row:opacity-100 focus-visible:opacity-100"
                onClick={(e) => {
                  e.stopPropagation();
                  if (quick === "upgrade") act.upgrade([app.id]);
                  else act.install([app.id]);
                }}
              >
                {quick === "upgrade" ? <ArrowUpCircleIcon /> : <DownloadIcon />}
                {quick === "upgrade" ? "更新" : "安装"}
              </Button>
            )}
          </div>
        </div>
      </ContextMenuTrigger>
      <RowMenu item={item} categories={categories} tags={tags} act={act} />
    </ContextMenu>
  );
}

function RowMenu({
  item,
  categories,
  tags,
  act,
}: {
  item: ShelfApp;
  categories: Map<string, CategoryInfo>;
  tags: Map<string, TagInfo>;
  act: RowActions;
}) {
  const targets = act.targets(item.app.id);
  const ids = targets.map((t) => t.app.id);
  const named = targets.map((t) => ({ id: t.app.id, name: t.app.name }));
  const many = targets.length > 1;
  const canInstall = targets.some((t) => t.status?.state === "notInstalled");
  const canUpgrade = targets.some((t) => t.status?.state === "updateAvailable");
  const catOf = new Set(targets.map((t) => t.app.categoryId ?? ""));
  const oneCat = catOf.size === 1 ? [...catOf][0] : undefined;
  const names = new Map([...categories.values()].map((c) => [c.id, c.name]));
  return (
    <ContextMenuContent className="w-52">
      {many && (
        <div className="px-2 py-1 text-xs text-muted-foreground">已选 {targets.length} 个软件</div>
      )}
      <ContextMenuItem disabled={!canInstall} onSelect={() => act.install(ids)}>
        <DownloadIcon />
        安装
      </ContextMenuItem>
      <ContextMenuItem disabled={!canUpgrade} onSelect={() => act.upgrade(ids)}>
        <ArrowUpCircleIcon />
        更新
      </ContextMenuItem>
      <ContextMenuSeparator />
      {!many && (
        <>
          <ContextMenuItem onSelect={() => act.open(item.app.id)}>
            <PanelRightIcon />
            查看详情
          </ContextMenuItem>
          <ContextMenuItem onSelect={() => act.edit(item.app)}>
            <PencilIcon />
            编辑
          </ContextMenuItem>
          <ContextMenuItem onSelect={() => act.note(item)}>
            <NotebookPenIcon />
            {item.app.note ? "编辑备注" : "写备注"}
          </ContextMenuItem>
        </>
      )}
      <ContextMenuSub>
        <ContextMenuSubTrigger>
          <FolderIcon />
          设分类
        </ContextMenuSubTrigger>
        <ContextMenuSubContent className="w-48">
          <ScrollArea viewportClassName="max-h-78">
            <ContextMenuRadioGroup
              value={oneCat ?? "__mixed"}
              onValueChange={(v) => void assignCategory(named, { id: v }, names)}
            >
              <ContextMenuRadioItem value="">不分类</ContextMenuRadioItem>
              {[...categories.values()].map((c) => (
                <ContextMenuRadioItem key={c.id} value={c.id}>
                  {c.name}
                </ContextMenuRadioItem>
              ))}
            </ContextMenuRadioGroup>
          </ScrollArea>
        </ContextMenuSubContent>
      </ContextMenuSub>
      {tags.size > 0 && (
        <ContextMenuSub>
          <ContextMenuSubTrigger>
            <HashIcon />
            标签
          </ContextMenuSubTrigger>
          <ContextMenuSubContent className="w-48">
            <ScrollArea viewportClassName="max-h-78">
              {[...tags.values()].map((t) => {
                const n = targets.filter((a) => a.app.tagIds?.includes(t.id)).length;
                const all = n === targets.length;
                return (
                  <ContextMenuCheckboxItem
                    key={t.id}
                    checked={all}
                    onSelect={(e) => {
                      e.preventDefault();
                      void assignTag(named, t, !all);
                    }}
                  >
                    <span className="truncate">{t.name}</span>
                    {n > 0 && !all && (
                      <MinusIcon className="absolute right-2 text-muted-foreground" />
                    )}
                  </ContextMenuCheckboxItem>
                );
              })}
            </ScrollArea>
          </ContextMenuSubContent>
        </ContextMenuSub>
      )}
      {!many && (
        <ContextMenuItem onSelect={() => void copyText(sourceID(item.app.source), "来源 ID")}>
          <CopyIcon />
          复制来源 ID
        </ContextMenuItem>
      )}
      <ContextMenuSeparator />
      <ContextMenuItem variant="destructive" onSelect={() => act.remove(targets)}>
        <Trash2Icon />
        {many ? `删除 ${targets.length} 个` : "删除"}
      </ContextMenuItem>
    </ContextMenuContent>
  );
}
