// Choosing a category and tags, existing or new, typed where they are
// chosen.

import { Button } from "@/components/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { useShelf } from "@/lib/api";
import { cn } from "@/lib/utils";
import {
  CheckIcon,
  ChevronsUpDownIcon,
  FolderIcon,
  FolderXIcon,
  HashIcon,
  PlusIcon,
  XIcon,
} from "lucide-react";
import { useState, type ReactNode } from "react";

/** A category chosen: an existing one by ID, "" for none, or a new one. */
export type CategoryChoice = { id: string } | { newName: string };

/** A choice of tags: existing ones by ID, and new ones by name. */
export interface TagChoice {
  ids: string[];
  newNames: string[];
}

export const noCategory: CategoryChoice = { id: "" };
export const noTags: TagChoice = { ids: [], newNames: [] };

/** CategoryPicker chooses a category, or none, or types a new one. */
export function CategoryPicker({
  value,
  onChange,
  className,
  children,
}: {
  value: CategoryChoice;
  onChange: (v: CategoryChoice) => void;
  className?: string;
  /** children replace the trigger. */
  children?: ReactNode;
}) {
  const { data } = useShelf();
  const cats = data?.categories ?? [];
  const [open, setOpen] = useState(false);
  const [q, setQ] = useState("");
  const typed = q.trim();
  const exists = cats.some((c) => c.name.toLowerCase() === typed.toLowerCase());
  const current =
    "newName" in value ? value.newName : (cats.find((c) => c.id === value.id)?.name ?? "");
  const choose = (v: CategoryChoice) => {
    onChange(v);
    setOpen(false);
    setQ("");
  };
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        {children ?? (
          <Button variant="outline" className={cn("w-full justify-between font-normal", className)}>
            <span className="flex min-w-0 items-center gap-2">
              <FolderIcon className="text-muted-foreground" />
              <span className={cn("truncate", !current && "text-muted-foreground")}>
                {current ? ("newName" in value ? `新分类：${current}` : current) : "不分类"}
              </span>
            </span>
            <ChevronsUpDownIcon className="text-muted-foreground" />
          </Button>
        )}
      </PopoverTrigger>
      <PopoverContent className="w-64 p-0" align="start">
        <Command>
          <CommandInput placeholder="搜索或新建分类" value={q} onValueChange={setQ} />
          <CommandList>
            <CommandEmpty>没有分类</CommandEmpty>
            <CommandGroup>
              <CommandItem value="不分类 __none" onSelect={() => choose({ id: "" })}>
                <FolderXIcon className="text-muted-foreground" />
                不分类
                {"id" in value && value.id === "" && <CheckIcon className="ml-auto" />}
              </CommandItem>
              {cats.map((c) => (
                <CommandItem
                  key={c.id}
                  value={`${c.name} ${c.id}`}
                  onSelect={() => choose({ id: c.id })}
                >
                  <FolderIcon className="text-muted-foreground" />
                  <span className="truncate">{c.name}</span>
                  <span className="ml-auto text-xs text-muted-foreground tabular-nums">
                    {c.count}
                  </span>
                  {"id" in value && value.id === c.id && <CheckIcon />}
                </CommandItem>
              ))}
            </CommandGroup>
            {typed && !exists && (
              <CommandGroup>
                <CommandItem value={`${typed} __new`} onSelect={() => choose({ newName: typed })}>
                  <PlusIcon className="text-primary" />
                  新建分类「{typed}」
                </CommandItem>
              </CommandGroup>
            )}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

/** TagPicker chooses tags, existing or typed. */
export function TagPicker({
  value,
  onChange,
  className,
}: {
  value: TagChoice;
  onChange: (v: TagChoice) => void;
  className?: string;
}) {
  const { data } = useShelf();
  const tags = data?.tags ?? [];
  const [open, setOpen] = useState(false);
  const [q, setQ] = useState("");
  const typed = q.trim();
  const exists =
    tags.some((t) => t.name.toLowerCase() === typed.toLowerCase()) ||
    value.newNames.some((n) => n.toLowerCase() === typed.toLowerCase());
  const toggleID = (id: string) =>
    onChange({
      ...value,
      ids: value.ids.includes(id) ? value.ids.filter((x) => x !== id) : [...value.ids, id],
    });
  const chosen = [
    ...value.ids.map((id) => ({
      key: id,
      name: tags.find((t) => t.id === id)?.name ?? id,
      remove: () => toggleID(id),
    })),
    ...value.newNames.map((n) => ({
      key: `new:${n}`,
      name: n,
      remove: () => onChange({ ...value, newNames: value.newNames.filter((x) => x !== n) }),
    })),
  ];
  return (
    <div className={cn("flex flex-wrap items-center gap-1.5", className)}>
      {chosen.map((t) => (
        <span
          key={t.key}
          className="anim-fade inline-flex h-7 items-center gap-1 rounded-md border bg-secondary/60 pr-1 pl-2 text-xs font-medium"
        >
          <HashIcon className="size-3 text-muted-foreground" />
          {t.name}
          <button
            type="button"
            className="rounded p-0.5 text-muted-foreground hover:bg-hover hover:text-foreground"
            onClick={t.remove}
            aria-label={`去掉标签 ${t.name}`}
          >
            <XIcon className="size-3" />
          </button>
        </span>
      ))}
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button
            variant="outline"
            size="sm"
            className="h-7 border-dashed font-normal text-muted-foreground"
          >
            <PlusIcon />
            标签
          </Button>
        </PopoverTrigger>
        <PopoverContent className="w-60 p-0" align="start">
          <Command>
            <CommandInput placeholder="搜索或新建标签" value={q} onValueChange={setQ} />
            <CommandList>
              <CommandEmpty>没有标签</CommandEmpty>
              {tags.length > 0 && (
                <CommandGroup>
                  {tags.map((t) => (
                    <CommandItem
                      key={t.id}
                      value={`${t.name} ${t.id}`}
                      onSelect={() => toggleID(t.id)}
                    >
                      <HashIcon className="text-muted-foreground" />
                      <span className="truncate">{t.name}</span>
                      {value.ids.includes(t.id) && <CheckIcon className="ml-auto" />}
                    </CommandItem>
                  ))}
                </CommandGroup>
              )}
              {typed && !exists && (
                <CommandGroup>
                  <CommandItem
                    value={`${typed} __new`}
                    onSelect={() => {
                      onChange({ ...value, newNames: [...value.newNames, typed] });
                      setQ("");
                    }}
                  >
                    <PlusIcon className="text-primary" />
                    新建标签「{typed}」
                  </CommandItem>
                </CommandGroup>
              )}
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>
    </div>
  );
}
