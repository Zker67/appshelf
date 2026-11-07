// Where apps are added, next to a page's search: what is chosen, opened to
// choose another category and tags. A new category or tag is made as it is
// chosen, so that the target is kept by ID.

import {
  CategoryPicker,
  TagPicker,
  type CategoryChoice,
  type TagChoice,
} from "@/components/pickers";
import { Button } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { targetNames, useAddTarget } from "@/lib/add-target";
import { errorText, useShelf } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Shelf } from "@/mygo";
import { ChevronDownIcon, FolderIcon, FolderXIcon, HashIcon, InboxIcon } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

export function AddTargetChip({ className }: { className?: string }) {
  const { data: shelf } = useShelf();
  const [target, setTarget] = useAddTarget();
  const [open, setOpen] = useState(false);
  const { category, tags } = targetNames(target, shelf);

  const chooseCategory = async (v: CategoryChoice) => {
    if ("id" in v) {
      setTarget({ ...target, categoryId: v.id });
      return;
    }
    try {
      const made = await Shelf.addCategory(v.newName);
      setTarget({ ...target, categoryId: made.id });
    } catch (err) {
      toast.error("新建分类失败", { description: errorText(err) });
    }
  };

  const chooseTags = async (v: TagChoice) => {
    try {
      const made = await Promise.all(v.newNames.map((n) => Shelf.addTag(n)));
      setTarget({ ...target, tagIds: [...v.ids, ...made.map((t) => t.id)] });
    } catch (err) {
      toast.error("新建标签失败", { description: errorText(err) });
    }
  };

  const CatIcon = category ? FolderIcon : FolderXIcon;
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <Tooltip>
        <TooltipTrigger asChild>
          <PopoverTrigger asChild>
            <Button
              variant="outline"
              size="sm"
              className={cn("max-w-56 min-w-0 shrink justify-start font-normal", className)}
            >
              <span className="text-muted-foreground">加入到</span>
              <CatIcon className={category ? "text-primary" : "text-muted-foreground"} />
              <span className={cn("truncate", category ? "font-medium" : "text-muted-foreground")}>
                {category || "不分类"}
              </span>
              {tags.length > 0 && (
                <span className="flex shrink-0 items-center gap-0.5 rounded bg-secondary px-1 text-[11px] text-muted-foreground">
                  <HashIcon className="size-3" />
                  {tags.length === 1 ? tags[0] : `${tags.length} 个标签`}
                </span>
              )}
              <ChevronDownIcon className="ml-auto text-muted-foreground" />
            </Button>
          </PopoverTrigger>
        </TooltipTrigger>
        <TooltipContent>新加入的软件放到这里</TooltipContent>
      </Tooltip>
      <PopoverContent align="start" className="flex w-72 flex-col gap-3 p-3">
        <div className="flex items-center gap-2 text-sm font-medium">
          <InboxIcon className="size-4 text-primary" />
          新加入的软件放到
        </div>
        <div className="flex flex-col gap-1.5">
          <span className="text-xs text-muted-foreground">分类</span>
          <CategoryPicker
            value={{ id: target.categoryId }}
            onChange={(v) => void chooseCategory(v)}
            className="h-8"
          />
        </div>
        <div className="flex flex-col gap-1.5">
          <span className="text-xs text-muted-foreground">标签</span>
          <TagPicker
            value={{ ids: target.tagIds, newNames: [] }}
            onChange={(v) => void chooseTags(v)}
          />
        </div>
        <p className="border-t pt-2.5 text-xs leading-relaxed text-muted-foreground">
          在发现和本机已装点「加入」时直接放到这里。GitHub 软件需要先确认检测规则。
        </p>
      </PopoverContent>
    </Popover>
  );
}
