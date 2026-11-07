// Adding apps in one click, to the target chosen next to the search: no
// dialog, a toast that says where they went and undoes it. A GitHub app
// needs its detection rule seen, so it opens the dialog, the target chosen.

import type { AddCandidate } from "@/components/dialogs/add-dialog";
import { describeTarget, useAddTarget } from "@/lib/add-target";
import { errorText, useShelf } from "@/lib/api";
import { Shelf } from "@/mygo";
import { useActions } from "@/state/actions";
import { useNav } from "@/state/nav";
import { useEffect, useEffectEvent, useState } from "react";
import { toast } from "sonner";

/** KeyedCandidate is a package to add, by the key of its row. */
export interface KeyedCandidate {
  key: string;
  cand: AddCandidate;
}

export function useQuickAdd(isInShelf: (key: string) => boolean) {
  const actions = useActions();
  const nav = useNav();
  const { data: shelf } = useShelf();
  const [target] = useAddTarget();
  /** adding: the rows being added, not to add twice. */
  const [adding, setAdding] = useState<ReadonlySet<string>>(new Set());

  const release = (ks: string[]) =>
    setAdding((s) =>
      ks.some((k) => s.has(k)) ? new Set([...s].filter((k) => !ks.includes(k))) : s,
    );

  // A row added stays adding until the catalog marks it, then it is done.
  const settle = useEffectEvent(() => release([...adding].filter(isInShelf)));
  useEffect(() => settle(), [shelf]);

  const add = async (items: KeyedCandidate[], install: boolean) => {
    const todo = items.filter((i) => !adding.has(i.key));
    if (todo.length === 0) return;
    if (todo.some((i) => i.cand.source.type === "github")) {
      actions.add(
        todo.map((i) => i.cand),
        { install, target },
      );
      return;
    }
    const keys = todo.map((i) => i.key);
    setAdding((s) => new Set([...s, ...keys]));
    try {
      const res = await Shelf.addPackages({
        items: todo.map(({ cand: c }) => ({ name: c.name, source: c.source, detect: c.detect })),
        categoryId: target.categoryId,
        tagIds: target.tagIds,
      });
      const failed = new Set(res.failed.map((f) => f.name));
      release(todo.filter((i) => failed.has(i.cand.name)).map((i) => i.key));
      if (res.failed.length > 0)
        toast.error(res.added.length > 0 ? `${res.failed.length} 个软件没有加入` : "没有加入清单", {
          description: res.failed.map((f) => `${f.name}：${f.error}`).join("\n"),
          duration: 10000,
        });
      const added = res.added;
      if (added.length === 0) return;
      toast.success(
        added.length === 1 && todo.length === 1
          ? `已加入「${todo[0]?.cand.name ?? ""}」`
          : `已加入 ${added.length} 个软件`,
        {
          description: describeTarget(target, shelf),
          duration: 6000,
          action: { label: "去清单查看", onClick: () => nav.showInShelf(added) },
          cancel: {
            label: "撤销",
            onClick: () => {
              Shelf.deleteApps(added)
                .then(() => {
                  release(keys);
                  toast("已撤销加入");
                })
                .catch((err: unknown) => toast.error("撤销失败", { description: errorText(err) }));
            },
          },
        },
      );
      if (install) actions.preview(added, "install");
    } catch (err) {
      release(keys);
      toast.error("加入清单失败", { description: errorText(err) });
    }
  };

  return { adding, add, hint: `加入到${describeTarget(target, shelf)}` };
}
