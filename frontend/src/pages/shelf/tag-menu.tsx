// Giving apps a tag, or taking it away: a tag all of them have is taken
// from all, any other is given to all.

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
import type { App } from "@/mygo";
import { CheckIcon, HashIcon, MinusIcon, PlusIcon } from "lucide-react";
import { useState, type ReactNode } from "react";
import { assignTag } from "./ops";

export function TagMenu({
  apps,
  children,
  align = "center",
}: {
  apps: App[];
  children: ReactNode;
  align?: "start" | "center" | "end";
}) {
  const { data } = useShelf();
  const tags = data?.tags ?? [];
  const [open, setOpen] = useState(false);
  const [q, setQ] = useState("");
  const typed = q.trim();
  const exists = tags.some((t) => t.name.toLowerCase() === typed.toLowerCase());
  const have = (id: string) => apps.filter((a) => a.tagIds?.includes(id)).length;
  const named = apps.map((a) => ({ id: a.id, name: a.name }));
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>{children}</PopoverTrigger>
      <PopoverContent className="w-60 p-0" align={align} side="top">
        <Command>
          <CommandInput placeholder="搜索或新建标签" value={q} onValueChange={setQ} />
          <CommandList>
            <CommandEmpty>没有标签</CommandEmpty>
            {tags.length > 0 && (
              <CommandGroup>
                {tags.map((t) => {
                  const n = have(t.id);
                  const all = n === apps.length && n > 0;
                  return (
                    <CommandItem
                      key={t.id}
                      value={`${t.name} ${t.id}`}
                      onSelect={() => void assignTag(named, t, !all)}
                    >
                      <HashIcon className="text-muted-foreground" />
                      <span className="truncate">{t.name}</span>
                      <span className="ml-auto flex size-4 items-center justify-center">
                        {all ? (
                          <CheckIcon className="text-primary" />
                        ) : n > 0 ? (
                          <MinusIcon className="text-muted-foreground" />
                        ) : null}
                      </span>
                    </CommandItem>
                  );
                })}
              </CommandGroup>
            )}
            {typed && !exists && (
              <CommandGroup>
                <CommandItem
                  value={`${typed} __new`}
                  onSelect={() => {
                    void assignTag(named, { newName: typed }, true);
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
  );
}
