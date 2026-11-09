// The toast of a newer AppShelf at the bottom right, as Vibe Atlas has it:
// 去更新 downloads it in place, with its progress, and AppShelf starts again
// in it; 稍后 puts it off until the next launch. No window of its own.

import { installSelf, openReleases, putOff, useLater, useSelfUpdate } from "@/lib/self-update";
import { useEffect } from "react";
import { toast } from "sonner";

/** The toaster at the bottom right, of this toast only. */
export const SELF_TOASTER = "self";
const ID = "self-update";

export function SelfUpdateToast() {
  const { data: s } = useSelfUpdate();
  const later = useLater();

  useEffect(() => {
    if (!s) return;
    const v = s.available ?? "";
    const base = { id: ID, toasterId: SELF_TOASTER, duration: Infinity } as const;
    switch (s.phase) {
      case "checking":
        return; // what shows stays while it checks again
      case "available":
        if (v === later) {
          toast.dismiss(ID);
          return;
        }
        toast(`发现新版本 ${v}`, {
          ...base,
          description: `当前 ${s.version}，更新后自动重新打开`,
          action: {
            label: "去更新",
            onClick: (e) => {
              e.preventDefault(); // the toast stays, to show the progress
              installSelf();
            },
          },
          cancel: { label: "稍后", onClick: () => putOff(v) },
          closeButton: false,
          onDismiss: () => putOff(v), // swiped away too
        });
        return;
      case "downloading":
        toast.loading(`正在更新到 ${v}`, {
          ...base,
          description: `已下载 ${Math.floor(s.progress * 100)}%`,
          action: undefined,
          cancel: undefined,
          closeButton: false,
          onDismiss: undefined,
        });
        return;
      case "restarting":
        toast.loading("马上重新打开", {
          ...base,
          description: `已更新到 ${v}`,
          action: undefined,
          cancel: undefined,
          closeButton: false,
          onDismiss: undefined,
        });
        return;
      case "failed":
        if (v === later) {
          toast.dismiss(ID);
          return;
        }
        toast.error("更新失败", {
          ...base,
          description: s.error,
          action: {
            label: "重试",
            onClick: (e) => {
              e.preventDefault();
              installSelf();
            },
          },
          // To download the installer by hand; × puts it off, as 稍后 does.
          cancel: {
            label: "去 GitHub",
            onClick: () => {
              openReleases(true);
              putOff(v);
            },
          },
          closeButton: true,
          onDismiss: () => putOff(v),
        });
        return;
      default:
        toast.dismiss(ID);
    }
  }, [s, later]);

  return null;
}
