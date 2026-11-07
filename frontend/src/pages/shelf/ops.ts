// Changing apps of the catalog, one or many: deleting with undo, a
// category, tags. Each says how it went in a toast.

import { errorText } from "@/lib/api";
import { Shelf } from "@/mygo";
import type { CategoryChoice } from "@/components/pickers";
import { toast } from "sonner";

interface Named {
  id: string;
  name: string;
}

function what(apps: Named[]): string {
  return apps.length === 1 ? `「${apps[0]?.name ?? ""}」` : `${apps.length} 个软件`;
}

/** deleteApps removes apps from the catalog, with undo in the toast. */
export async function deleteApps(apps: Named[]): Promise<boolean> {
  if (apps.length === 0) return false;
  try {
    const token = await Shelf.deleteApps(apps.map((a) => a.id));
    toast.success(`已删除${what(apps)}`, {
      description: "只从清单中删除，不会卸载软件",
      duration: 8000,
      action: {
        label: "撤销",
        onClick: () => {
          Shelf.undoDelete(token)
            .then((n) =>
              toast.success(n === apps.length ? `已恢复${what(apps)}` : `已恢复 ${n} 个软件`),
            )
            .catch((err: unknown) => toast.error("撤销失败", { description: errorText(err) }));
        },
      },
    });
    return true;
  } catch (err) {
    toast.error("删除失败", { description: errorText(err) });
    return false;
  }
}

/** assignCategory puts apps in a category, made first when it is new. */
export async function assignCategory(
  apps: Named[],
  choice: CategoryChoice,
  names: Map<string, string>,
): Promise<void> {
  try {
    const id = "newName" in choice ? (await Shelf.addCategory(choice.newName)).id : choice.id;
    await Shelf.setCategory(
      apps.map((a) => a.id),
      id,
    );
    const name = "newName" in choice ? choice.newName : names.get(id);
    toast.success(id ? `已把${what(apps)}移到「${name ?? ""}」` : `已把${what(apps)}设为未分类`);
  } catch (err) {
    toast.error("设置分类失败", { description: errorText(err) });
  }
}

/** assignTag gives apps a tag, made first when it is new, or takes it away. */
export async function assignTag(
  apps: Named[],
  tag: { id: string; name: string } | { newName: string },
  on: boolean,
): Promise<void> {
  try {
    const id = "newName" in tag ? (await Shelf.addTag(tag.newName)).id : tag.id;
    const name = "newName" in tag ? tag.newName : tag.name;
    await Shelf.setTag(
      apps.map((a) => a.id),
      id,
      on,
    );
    toast.success(on ? `已给${what(apps)}加上「${name}」` : `已从${what(apps)}去掉「${name}」`);
  } catch (err) {
    toast.error("设置标签失败", { description: errorText(err) });
  }
}
