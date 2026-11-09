// The details of a program of this machine, the most complete of the three
// pages': what this machine knows of it, what its source says, its update
// with the release notes, the room it takes and how it is removed.

import { AppIcon } from "@/components/app-icon";
import { CopyButton } from "@/components/common";
import { DetailPanel, DetailTabs, type DetailTab } from "@/components/detail-panel";
import { FactGrid, PackageInfo, type Fact } from "@/components/package-info";
import {
  Notice,
  ReleaseNotesView,
  RemovalView,
  SpaceView,
  openLocation,
} from "@/components/program-info";
import { AddButton, InShelfButton } from "@/components/row-actions";
import { SourceBadge } from "@/components/source-badge";
import { Pill } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { UpdateMenu } from "@/components/update-menu";
import { sourceID, sourceKey } from "@/lib/shelf";
import { formatBytes } from "@/lib/text";
import { ageText, localDate } from "@/lib/uninstall";
import { holdText, modeLabels } from "@/lib/updates";
import type { InstalledEntry, UpdateItem } from "@/mygo";
import {
  ArrowUpCircleIcon,
  BellOffIcon,
  BoxIcon,
  Building2Icon,
  CalendarIcon,
  CheckIcon,
  FolderOpenIcon,
  HardDriveIcon,
  HistoryIcon,
  LayoutDashboardIcon,
  MonitorCheckIcon,
  PackageXIcon,
  ScrollTextIcon,
} from "lucide-react";
import { useState } from "react";

/** localFacts are what this machine knows of a program. */
export function localFacts(e: InstalledEntry, now: Date): Fact[] {
  const facts: Fact[] = [];
  if (e.version) {
    facts.push({ label: "已装版本", icon: BoxIcon, value: e.version, title: e.version });
  }
  if (e.publisher) {
    facts.push({ label: "发布者", icon: Building2Icon, value: e.publisher, title: e.publisher });
  }
  if (e.installed) facts.push({ label: "安装日期", icon: CalendarIcon, value: e.installed });
  facts.push({
    label: "上次打开",
    icon: HistoryIcon,
    value: e.lastUsed ? `${ageText(e.lastUsed, now)} · ${localDate(e.lastUsed)}` : "没有记录",
  });
  if (e.size) facts.push({ label: "登记大小", icon: HardDriveIcon, value: formatBytes(e.size) });
  return facts;
}

export function InstalledDetail({
  entry: e,
  update,
  adding,
  addHint,
  onClose,
  onUpdate,
  onAdd,
  onShow,
  onFind,
  onUninstall,
}: {
  entry: InstalledEntry;
  /** update is what the updates know of its package. */
  update?: UpdateItem;
  adding: boolean;
  addHint: string;
  onClose: () => void;
  onUpdate: () => void;
  onAdd: () => void;
  onShow: () => void;
  onFind: () => void;
  onUninstall: () => void;
}) {
  const [now] = useState(() => new Date());
  const src = e.source;
  const id = src ? sourceID(src) : "";
  const held = !!e.update && !!e.hold;
  const facts = localFacts(e, now);
  if (e.mode && src) {
    facts.push({ label: "更新方式", icon: ArrowUpCircleIcon, value: modeLabels[e.mode] });
  }

  const after: DetailTab[] = [];
  if (src && e.update) {
    after.push({
      value: "update",
      label: "更新",
      icon: ScrollTextIcon,
      content: (
        <ReleaseNotesView
          pkgRef={{ type: src.type, id }}
          from={e.version}
          to={e.update}
          lead={
            held ? (
              <span className="text-xs text-muted-foreground">
                {holdText(e.hold, { skip: update?.skip, until: update?.until })}
              </span>
            ) : update?.running ? (
              <span className="text-xs text-warning">它正在运行，更新时可能会被关闭</span>
            ) : undefined
          }
        />
      ),
    });
  }
  after.push(
    {
      value: "space",
      label: "空间",
      icon: HardDriveIcon,
      content: <SpaceView entryKey={e.key} />,
    },
    {
      value: "removal",
      label: "卸载",
      icon: PackageXIcon,
      content: <RemovalView entryKey={e.key} />,
    },
  );

  return (
    <DetailPanel
      key={e.key}
      onClose={onClose}
      icon={<AppIcon path={e.icon} name={e.name} size={40} />}
      title={e.name}
      subtitle={
        id ? (
          <>
            <span className="truncate font-mono select-text">{id}</span>
            <CopyButton text={id} what="来源 ID" />
          </>
        ) : e.publisher ? (
          <span className="truncate select-text">{e.publisher}</span>
        ) : undefined
      }
      badges={
        <>
          <SourceBadge type={e.kind} />
          {e.update && !held && (
            <Pill tone="primary">
              <ArrowUpCircleIcon />
              可更新 {e.update}
            </Pill>
          )}
          {held && (
            <Pill tone="muted">
              <BellOffIcon />
              {holdText(e.hold, { skip: update?.skip, until: update?.until })}
            </Pill>
          )}
          {e.inShelf.length > 0 && (
            <Pill tone="success">
              <CheckIcon />
              在清单里
            </Pill>
          )}
        </>
      }
      actions={
        <>
          {src && e.update && !held && (
            <Button size="sm" onClick={onUpdate}>
              <ArrowUpCircleIcon />
              更新到 {e.update}
            </Button>
          )}
          {e.inShelf.length > 0 ? (
            <InShelfButton onClick={onShow} />
          ) : src ? (
            <AddButton
              label="加入清单"
              adding={adding}
              hint={src.type === "github" ? "确认检测规则后加入" : addHint}
              onClick={onAdd}
            />
          ) : (
            <Button size="sm" variant="outline" onClick={onFind}>
              <MonitorCheckIcon />
              查找来源
            </Button>
          )}
          {src && (e.update || (e.mode && e.mode !== "notify")) && (
            <UpdateMenu
              target={{
                key: sourceKey(src),
                name: e.name,
                mode: e.mode ?? "notify",
                hold: e.hold,
                latest: e.update,
                canAuto: update?.canAuto,
                autoNote: update?.autoNote,
              }}
            />
          )}
          <div className="ml-auto flex items-center gap-0.5">
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  size="icon-sm"
                  variant="ghost"
                  className="text-muted-foreground"
                  aria-label="打开位置"
                  onClick={() => openLocation(e.key)}
                >
                  <FolderOpenIcon />
                </Button>
              </TooltipTrigger>
              <TooltipContent>打开安装位置</TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  size="icon-sm"
                  variant="ghost"
                  className="text-muted-foreground hover:text-destructive"
                  aria-label="卸载"
                  onClick={onUninstall}
                >
                  <PackageXIcon />
                </Button>
              </TooltipTrigger>
              <TooltipContent>卸载</TooltipContent>
            </Tooltip>
          </div>
        </>
      }
    >
      {src ? (
        <PackageInfo pkgRef={{ type: src.type, id }} latest={e.update} facts={facts} tabs={after} />
      ) : (
        <DetailTabs
          tabs={[
            {
              value: "overview",
              label: "概览",
              icon: LayoutDashboardIcon,
              content: (
                <div className="flex flex-col gap-3">
                  <FactGrid facts={facts} />
                  <Notice>
                    {e.kind === "store"
                      ? "它是应用商店的应用，更新由应用商店负责。"
                      : "它不是通过 winget 或 scoop 安装的，没有来源信息，也无法直接更新。可以「查找来源」找到它的软件包后加入清单。"}
                  </Notice>
                </div>
              ),
            },
            ...after,
          ]}
        />
      )}
    </DetailPanel>
  );
}
