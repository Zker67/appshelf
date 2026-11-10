// Tells the user, once a run, that a removal's uninstaller waits for them: the window shows
// it here, a system notification when the window is away.

import { useEffect, useRef } from "react";
import { toast } from "sonner";

import { errorText } from "@/lib/api";
import { newWaits } from "@/lib/tasks";
import { Tasks, type TaskEntry } from "@/mygo";

export function useWaitToasts(items: readonly TaskEntry[] | undefined) {
  const told = useRef(new Set<string>());
  useEffect(() => {
    for (const t of newWaits(items ?? [], told.current)) {
      toast.warning(`「${t.name}」的卸载程序在等你操作`, {
        description: t.prompt || "请在卸载程序的窗口里完成操作",
        duration: 15000,
        action: t.window
          ? {
              label: "显示窗口",
              onClick: () => {
                Tasks.control(t.id, "showWindow").catch((err: unknown) =>
                  toast.error("显示窗口失败", { description: errorText(err) }),
                );
              },
            }
          : undefined,
      });
    }
  }, [items]);
}
