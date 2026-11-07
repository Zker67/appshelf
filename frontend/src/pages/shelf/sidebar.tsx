// The catalog's side column: all apps, the states, the categories and the
// tags to filter by, managed where they are listed; importing and exporting
// at the bottom.

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { SettingsFooter } from "@/components/side-nav";
import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { errorText } from "@/lib/api";
import { hasGroupFilter, type ShelfFilter, type StatusFilter } from "@/lib/shelf";
import { toggle } from "@/lib/selection";
import { cn } from "@/lib/utils";
import { Shelf, type ShelfSnapshot } from "@/mygo";
import type { CatalogFiles } from "./catalog-files";
import {
  ArrowUpCircleIcon,
  CheckCircle2Icon,
  CircleIcon,
  FileDownIcon,
  FileUpIcon,
  FolderIcon,
  FolderXIcon,
  HashIcon,
  LayoutGridIcon,
  LoaderCircleIcon,
  PencilIcon,
  PlusIcon,
  Trash2Icon,
  TriangleAlertIcon,
  type LucideIcon,
} from "lucide-react";
import { useState, type ReactNode } from "react";
import { toast } from "sonner";

const statusItems: Array<{
  value: Exclude<StatusFilter, "all">;
  label: string;
  icon: LucideIcon;
  tone: string;
}> = [
  { value: "update", label: "可更新", icon: ArrowUpCircleIcon, tone: "text-primary" },
  { value: "installed", label: "已安装", icon: CheckCircle2Icon, tone: "text-success" },
  { value: "notInstalled", label: "未安装", icon: CircleIcon, tone: "text-muted-foreground" },
  { value: "problem", label: "无法确认", icon: TriangleAlertIcon, tone: "text-warning" },
];

export function ShelfSidebar({
  shelf,
  filter,
  onFilter,
  counts,
  files,
}: {
  shelf: ShelfSnapshot | undefined;
  filter: ShelfFilter;
  onFilter: (f: ShelfFilter) => void;
  counts: Record<StatusFilter, number>;
  files: CatalogFiles;
}) {
  const cats = shelf?.categories ?? [];
  const tags = shelf?.tags ?? [];
  const all = !hasGroupFilter(filter) && filter.status === "all";
  return (
    <div className="flex h-full min-h-0 flex-col bg-sidebar">
      <ScrollArea className="min-h-0 flex-1">
        <div className="flex flex-col gap-4 p-2.5">
          <NavRow
            icon={LayoutGridIcon}
            label="全部软件"
            count={shelf?.apps.length ?? 0}
            active={all}
            onClick={() =>
              onFilter({ ...filter, categories: new Set(), tags: new Set(), status: "all" })
            }
          />
          <Group title="状态">
            {statusItems
              .filter((s) => counts[s.value] > 0 || filter.status === s.value)
              .map((s) => (
                <NavRow
                  key={s.value}
                  icon={s.icon}
                  iconClass={s.tone}
                  label={s.label}
                  count={counts[s.value]}
                  active={filter.status === s.value}
                  onClick={() =>
                    onFilter({ ...filter, status: filter.status === s.value ? "all" : s.value })
                  }
                />
              ))}
          </Group>
          <NamesGroup
            kind="category"
            items={cats}
            selected={filter.categories}
            onToggle={(id) => onFilter({ ...filter, categories: toggle(filter.categories, id) })}
            extra={
              shelf && shelf.uncategorized > 0 && cats.length > 0 ? (
                <NavRow
                  icon={FolderXIcon}
                  label="未分类"
                  count={shelf.uncategorized}
                  active={filter.categories.has("")}
                  multi
                  onClick={() => onFilter({ ...filter, categories: toggle(filter.categories, "") })}
                />
              ) : null
            }
          />
          <NamesGroup
            kind="tag"
            items={tags}
            selected={filter.tags}
            onToggle={(id) => onFilter({ ...filter, tags: toggle(filter.tags, id) })}
          />
        </div>
      </ScrollArea>
      <CatalogButtons files={files} />
      <SettingsFooter />
    </div>
  );
}

function Group({
  title,
  action,
  children,
}: {
  title: string;
  action?: ReactNode;
  children: ReactNode;
}) {
  return (
    <div className="flex flex-col gap-0.5">
      <div className="flex h-7 items-center justify-between pr-1 pl-2.5">
        <span className="text-[11px] font-semibold tracking-wide text-muted-foreground">
          {title}
        </span>
        {action}
      </div>
      {children}
    </div>
  );
}

function NavRow({
  icon: Icon,
  iconClass,
  label,
  count,
  active,
  multi,
  onClick,
  tools,
}: {
  icon: LucideIcon;
  iconClass?: string;
  label: string;
  count: number;
  active: boolean;
  /** multi: one of several that can be chosen together. */
  multi?: boolean;
  onClick: () => void;
  tools?: ReactNode;
}) {
  return (
    <div
      role="button"
      tabIndex={0}
      aria-pressed={active}
      onClick={onClick}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onClick();
        }
      }}
      className={cn(
        "group/nav relative flex h-8 items-center gap-2 rounded-lg pr-2 pl-2.5 text-[13px] transition-colors duration-150 outline-none select-none focus-visible:ring-2 focus-visible:ring-ring/50",
        active
          ? "bg-primary/10 font-medium text-foreground dark:bg-primary/15"
          : "text-foreground/80 hover:bg-foreground/[0.05] hover:text-foreground",
      )}
    >
      {active && (
        <span className="anim-fade absolute top-1.5 bottom-1.5 left-0 w-[3px] rounded-full bg-primary" />
      )}
      <Icon
        className={cn(
          "size-4 shrink-0 transition-colors",
          active ? "text-primary" : (iconClass ?? "text-muted-foreground"),
        )}
      />
      <span className="min-w-0 truncate">{label}</span>
      {tools && (
        <span className="flex shrink-0 items-center opacity-0 transition-opacity group-hover/nav:opacity-100 group-focus-within/nav:opacity-100">
          {tools}
        </span>
      )}
      <span
        className={cn(
          "ml-auto shrink-0 text-xs tabular-nums",
          active ? "text-primary" : "text-muted-foreground",
        )}
      >
        {count}
      </span>
      {multi && active && <span className="sr-only">已选</span>}
    </div>
  );
}

function ToolButton({
  icon: Icon,
  label,
  onClick,
  danger,
}: {
  icon: LucideIcon;
  label: string;
  onClick: () => void;
  danger?: boolean;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          aria-label={label}
          onClick={(e) => {
            e.stopPropagation();
            onClick();
          }}
          className={cn(
            "flex size-6 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-foreground/10",
            danger ? "hover:text-destructive" : "hover:text-foreground",
          )}
        >
          <Icon className="size-3.5" />
        </button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

/** NameInput writes a name in place: Enter keeps it, Esc leaves. */
function NameInput({
  initial,
  placeholder,
  icon: Icon,
  onDone,
}: {
  initial: string;
  placeholder: string;
  icon: LucideIcon;
  onDone: (name: string | null) => void;
}) {
  const [value, setValue] = useState(initial);
  const finish = (keep: boolean) =>
    onDone(keep && value.trim() && value.trim() !== initial ? value.trim() : null);
  return (
    <div className="anim-fade flex h-8 items-center gap-2 rounded-lg bg-background pr-1.5 pl-2.5 ring-2 ring-ring/40">
      <Icon className="size-4 shrink-0 text-primary" />
      <input
        autoFocus
        value={value}
        placeholder={placeholder}
        onChange={(e) => setValue(e.target.value)}
        onBlur={() => finish(true)}
        onKeyDown={(e) => {
          if (e.key === "Enter") finish(true);
          if (e.key === "Escape") {
            e.stopPropagation();
            finish(false);
          }
        }}
        className="min-w-0 flex-1 bg-transparent text-[13px] outline-none placeholder:text-muted-foreground"
      />
    </div>
  );
}

function NamesGroup({
  kind,
  items,
  selected,
  onToggle,
  extra,
}: {
  kind: "category" | "tag";
  items: Array<{ id: string; name: string; count: number }>;
  selected: ReadonlySet<string>;
  onToggle: (id: string) => void;
  extra?: ReactNode;
}) {
  const word = kind === "category" ? "分类" : "标签";
  const icon = kind === "category" ? FolderIcon : HashIcon;
  const [adding, setAdding] = useState(false);
  const [renaming, setRenaming] = useState<string | null>(null);
  const [deleting, setDeleting] = useState<{ id: string; name: string; count: number } | null>(
    null,
  );

  const add = async (name: string | null) => {
    setAdding(false);
    if (!name) return;
    try {
      await (kind === "category" ? Shelf.addCategory(name) : Shelf.addTag(name));
    } catch (err) {
      toast.error(`添加${word}失败`, { description: errorText(err) });
    }
  };
  const rename = async (id: string, name: string | null) => {
    setRenaming(null);
    if (!name) return;
    try {
      await (kind === "category" ? Shelf.renameCategory(id, name) : Shelf.renameTag(id, name));
    } catch (err) {
      toast.error(`重命名${word}失败`, { description: errorText(err) });
    }
  };
  const remove = async () => {
    const d = deleting;
    setDeleting(null);
    if (!d) return;
    try {
      await (kind === "category" ? Shelf.deleteCategory(d.id) : Shelf.deleteTag(d.id));
      if (selected.has(d.id)) onToggle(d.id);
      toast.success(`已删除${word}「${d.name}」`);
    } catch (err) {
      toast.error(`删除${word}失败`, { description: errorText(err) });
    }
  };

  return (
    <Group
      title={word}
      action={<ToolButton icon={PlusIcon} label={`新建${word}`} onClick={() => setAdding(true)} />}
    >
      {items.map((it) =>
        renaming === it.id ? (
          <NameInput
            key={it.id}
            initial={it.name}
            placeholder={`${word}名称`}
            icon={icon}
            onDone={(n) => void rename(it.id, n)}
          />
        ) : (
          <NavRow
            key={it.id}
            icon={icon}
            label={it.name}
            count={it.count}
            active={selected.has(it.id)}
            multi
            onClick={() => onToggle(it.id)}
            tools={
              <>
                <ToolButton icon={PencilIcon} label="重命名" onClick={() => setRenaming(it.id)} />
                <ToolButton icon={Trash2Icon} label="删除" danger onClick={() => setDeleting(it)} />
              </>
            }
          />
        ),
      )}
      {extra}
      {adding ? (
        <NameInput
          initial=""
          placeholder={`新${word}名称`}
          icon={icon}
          onDone={(n) => void add(n)}
        />
      ) : (
        <button
          type="button"
          onClick={() => setAdding(true)}
          className="flex h-8 items-center gap-2 rounded-lg pl-2.5 text-[13px] text-muted-foreground transition-colors hover:bg-foreground/[0.05] hover:text-foreground"
        >
          <PlusIcon className="size-4" />
          新建{word}
        </button>
      )}
      <AlertDialog open={deleting !== null} onOpenChange={(o) => !o && setDeleting(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              删除{word}「{deleting?.name}」？
            </AlertDialogTitle>
            <AlertDialogDescription>
              {kind === "category"
                ? `其中的 ${deleting?.count ?? 0} 个软件变为未分类，软件本身不会删除。`
                : `从 ${deleting?.count ?? 0} 个软件上去掉这个标签，软件本身不会删除。`}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction variant="destructive" onClick={() => void remove()}>
              删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Group>
  );
}

/** CatalogButtons are importing and exporting, at the column's bottom. */
function CatalogButtons({ files }: { files: CatalogFiles }) {
  return (
    <div className="grid grid-cols-2 gap-1.5 border-t p-2.5">
      <Button variant="outline" size="sm" disabled={files.busy !== null} onClick={files.importFile}>
        {files.busy === "import" ? <LoaderCircleIcon className="animate-spin" /> : <FileDownIcon />}
        导入
      </Button>
      <Button variant="outline" size="sm" disabled={files.busy !== null} onClick={files.exportFile}>
        {files.busy === "export" ? <LoaderCircleIcon className="animate-spin" /> : <FileUpIcon />}
        导出
      </Button>
    </div>
  );
}
