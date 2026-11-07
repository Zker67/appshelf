// Exporting the catalog to a file, and importing one after a preview of
// what merging it changes.

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
import { errorText } from "@/lib/api";
import { Shelf, type ImportPreview } from "@/mygo";
import { useState, type ReactNode } from "react";
import { toast } from "sonner";

export interface CatalogFiles {
  busy: "import" | "export" | null;
  exportFile: () => void;
  importFile: () => void;
  /** confirm is the import's confirmation, to render once. */
  confirm: ReactNode;
}

export function useCatalogFiles(): CatalogFiles {
  const [busy, setBusy] = useState<"import" | "export" | null>(null);
  const [preview, setPreview] = useState<ImportPreview | null>(null);
  const exportFile = async () => {
    setBusy("export");
    try {
      const path = await Shelf.export();
      if (path) toast.success("已导出清单", { description: path });
    } catch (err) {
      toast.error("导出失败", { description: errorText(err) });
    } finally {
      setBusy(null);
    }
  };
  const importFile = async () => {
    setBusy("import");
    try {
      const p = await Shelf.readImport();
      if (p) setPreview(p);
    } catch (err) {
      toast.error("无法导入", { description: errorText(err) });
    } finally {
      setBusy(null);
    }
  };
  const apply = async () => {
    const p = preview;
    setPreview(null);
    if (!p) return;
    try {
      const c = await Shelf.applyImport(p.token);
      toast.success("已导入清单", {
        description: `新增 ${c.addedApps} 个软件，合并 ${c.mergedApps} 个，新增 ${c.addedCategories} 个分类、${c.addedTags} 个标签`,
      });
    } catch (err) {
      toast.error("导入失败", { description: errorText(err) });
    }
  };
  const c = preview?.counts;
  const file = preview?.file.split(/[\\/]/).pop() ?? "";
  const confirm = (
    <AlertDialog open={preview !== null} onOpenChange={(o) => !o && setPreview(null)}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>导入「{file}」？</AlertDialogTitle>
          <AlertDialogDescription>
            将新增 {c?.addedApps ?? 0} 个软件，合并 {c?.mergedApps ?? 0} 个已有的软件，新增{" "}
            {c?.addedCategories ?? 0} 个分类、{c?.addedTags ?? 0}{" "}
            个标签。已有的软件保留本机的设置，只补充分类和标签。
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>取消</AlertDialogCancel>
          <AlertDialogAction onClick={() => void apply()}>导入</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
  return {
    busy,
    exportFile: () => void exportFile(),
    importFile: () => void importFile(),
    confirm,
  };
}
