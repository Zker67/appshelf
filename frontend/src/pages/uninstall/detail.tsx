// The details of a program weighed for removal: the room it takes,
// measured, with where it is to open, and the directories that may hold
// its data; then how it would be removed.

import { AppIcon } from "@/components/app-icon";
import { DetailPanel, DetailTabs } from "@/components/detail-panel";
import { FactGrid, type Fact } from "@/components/package-info";
import { RemovalView, SpaceView, openLocation } from "@/components/program-info";
import { SourceBadge } from "@/components/source-badge";
import { Pill } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import { ageText, localDate, useBand } from "@/lib/uninstall";
import type { InstalledEntry, TaskEntry } from "@/mygo";
import {
  BoxIcon,
  CalendarIcon,
  CheckIcon,
  ClockIcon,
  FolderOpenIcon,
  HardDriveIcon,
  HistoryIcon,
  LoaderCircleIcon,
  PackageXIcon,
  WrenchIcon,
} from "lucide-react";
import { useState } from "react";

export function UninstallDetail({
  entry: e,
  task,
  onClose,
  onUninstall,
}: {
  entry: InstalledEntry;
  /** task is its removal, waiting or running. */
  task?: TaskEntry;
  onClose: () => void;
  onUninstall: () => void;
}) {
  const [now] = useState(() => new Date());
  const stale = useBand(e.lastUsed, now) === "stale";
  const facts: Fact[] = [
    {
      label: "上次打开",
      icon: HistoryIcon,
      value: e.lastUsed ? `${ageText(e.lastUsed, now)} · ${localDate(e.lastUsed)}` : "没有记录",
      tone: stale ? "warning" : undefined,
    },
  ];
  if (e.installed) facts.push({ label: "安装日期", icon: CalendarIcon, value: e.installed });
  if (e.version) {
    facts.push({ label: "版本", icon: BoxIcon, value: e.version, title: e.version });
  }
  return (
    <DetailPanel
      key={e.key}
      onClose={onClose}
      icon={<AppIcon path={e.icon} name={e.name} size={40} />}
      title={e.name}
      subtitle={
        e.publisher ? <span className="truncate select-text">{e.publisher}</span> : undefined
      }
      badges={
        <>
          <SourceBadge type={e.kind} />
          {e.inShelf.length > 0 && (
            <Pill tone="success">
              <CheckIcon />
              在清单里
            </Pill>
          )}
          {task && (
            <Pill tone={task.state === "running" ? "primary" : "muted"}>
              {task.state === "running" ? (
                <LoaderCircleIcon className="animate-spin" />
              ) : (
                <ClockIcon />
              )}
              {task.state === "running" ? "卸载中" : "等待卸载"}
            </Pill>
          )}
        </>
      }
      actions={
        <>
          <Button size="sm" variant="destructive" disabled={!!task} onClick={onUninstall}>
            <PackageXIcon />
            卸载
          </Button>
          <Button size="sm" variant="outline" onClick={() => openLocation(e.key)}>
            <FolderOpenIcon />
            打开位置
          </Button>
        </>
      }
    >
      <DetailTabs
        tabs={[
          {
            value: "space",
            label: "空间",
            icon: HardDriveIcon,
            content: (
              <div className="flex flex-col gap-3">
                <FactGrid facts={facts} />
                <SpaceView entryKey={e.key} />
              </div>
            ),
          },
          {
            value: "removal",
            label: "卸载方式",
            icon: WrenchIcon,
            content: <RemovalView entryKey={e.key} />,
          },
        ]}
      />
    </DetailPanel>
  );
}
