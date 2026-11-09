// The 备注 tab of an app's details: its note drawn as a page, its tasks
// ticked in place; writing it opens the note editor.

import { MarkdownView } from "@/components/markdown";
import { Button } from "@/components/ui/button";
import { errorText } from "@/lib/api";
import { taskCount, toggleTask } from "@/lib/markdown";
import { Shelf, type ShelfApp } from "@/mygo";
import { useActions } from "@/state/actions";
import { ListChecksIcon, NotebookPenIcon, PencilIcon, PlusIcon } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

export function NoteTab({ item }: { item: ShelfApp }) {
  const actions = useActions();
  const onEdit = () => actions.editNote(item);
  const note = item.app.note ?? "";
  // A ticked task shows at once, until the catalog comes back with it.
  const [ticked, setTicked] = useState<{ from: string; to: string } | null>(null);
  const shown = ticked && ticked.from === note ? ticked.to : note;

  if (shown.trim() === "") {
    return (
      <div className="anim-rise flex flex-col items-center gap-3 rounded-xl border border-dashed bg-card/50 px-6 py-10 text-center">
        <div className="flex size-11 items-center justify-center rounded-2xl bg-primary/10 text-primary ring-1 ring-primary/15">
          <NotebookPenIcon className="size-5" />
        </div>
        <div>
          <div className="text-sm font-medium">还没有备注</div>
          <div className="mt-1 text-xs leading-relaxed text-muted-foreground">
            记下安装后要改的设置、激活方式、常用快捷键……
            <br />
            支持 Markdown，边写边预览，只存在你的清单里
          </div>
        </div>
        <Button size="sm" onClick={onEdit}>
          <PlusIcon />
          写备注
        </Button>
      </div>
    );
  }

  const tick = (offset: number) => {
    const to = toggleTask(shown, offset);
    if (to === shown) return;
    setTicked({ from: note, to });
    Shelf.setNote(item.app.id, to).catch((err: unknown) => {
      setTicked(null);
      toast.error("备注没有保存", { description: errorText(err) });
    });
  };

  const tasks = taskCount(shown);
  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center gap-2">
        {tasks.total > 0 && (
          <span className="inline-flex h-6 items-center gap-1.5 rounded-full bg-track pr-2.5 pl-2 text-xs text-muted-foreground tabular-nums">
            <ListChecksIcon className="size-3.5" />
            {tasks.done} / {tasks.total}
            <span className="h-1 w-10 overflow-hidden rounded-full bg-track">
              <span
                className="block h-full rounded-full bg-primary transition-[width] duration-500 ease-out-soft"
                style={{ width: `${(tasks.done / tasks.total) * 100}%` }}
              />
            </span>
          </span>
        )}
        <Button
          size="xs"
          variant="ghost"
          className="ml-auto text-muted-foreground"
          onClick={onEdit}
        >
          <PencilIcon />
          编辑备注
        </Button>
      </div>
      <div className="anim-rise rounded-xl bg-card px-4 py-3.5 shadow-soft ring-1 ring-border/70">
        <MarkdownView source={shown} onToggleTask={tick} />
      </div>
    </div>
  );
}
