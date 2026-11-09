// The details of an update: from which version to which, how it is
// handled, what changed in each version between, then what the package's
// source says of it.

import { AppIcon } from "@/components/app-icon";
import { CopyButton } from "@/components/common";
import { DetailPanel } from "@/components/detail-panel";
import { PackageInfo, type Fact } from "@/components/package-info";
import { ReleaseNotesView } from "@/components/program-info";
import { SourceBadge } from "@/components/source-badge";
import { Pill } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import { UpdateMenu, modeIcons } from "@/components/update-menu";
import { sourceID } from "@/lib/shelf";
import { holdText, modeLabels } from "@/lib/updates";
import type { UpdateItem } from "@/mygo";
import {
  AppWindowIcon,
  ArrowUpCircleIcon,
  BellOffIcon,
  BoxIcon,
  ClockIcon,
  ScrollTextIcon,
  TriangleAlertIcon,
  ZapOffIcon,
} from "lucide-react";

export function UpdateDetail({
  item: i,
  onClose,
  onUpdate,
}: {
  item: UpdateItem;
  onClose: () => void;
  onUpdate: () => void;
}) {
  const id = sourceID(i.source);
  const ModeIcon = modeIcons[i.mode];
  const hold = holdText(i.hold, { skip: i.skip, until: i.until, mode: i.mode });
  const facts: Fact[] = [];
  if (i.version) {
    facts.push({ label: "已装版本", icon: BoxIcon, value: i.version, title: i.version });
  }
  facts.push({ label: "更新方式", icon: ModeIcon, value: modeLabels[i.mode] });
  if (!i.canAuto && i.autoNote) {
    facts.push({
      label: "不能自动更新",
      icon: ZapOffIcon,
      value: i.autoNote,
      title: i.autoNote,
      wide: true,
    });
  }

  const lead = (i.major || i.running || hold || i.note) && (
    <div className="flex flex-col gap-1.5 text-xs text-muted-foreground">
      {i.major && (
        <span className="flex items-center gap-1.5 text-warning">
          <TriangleAlertIcon className="size-3.5" />
          大版本升级，可能有不兼容的改动，建议先看更新日志
        </span>
      )}
      {i.running && (
        <span className="flex items-center gap-1.5 text-warning">
          <AppWindowIcon className="size-3.5" />
          它正在运行，更新时可能会被关闭
        </span>
      )}
      {hold && (
        <span className="flex items-center gap-1.5">
          <BellOffIcon className="size-3.5" />
          {hold}
        </span>
      )}
      {i.note && <span className="break-words">{i.note}</span>}
    </div>
  );

  return (
    <DetailPanel
      key={i.key}
      onClose={onClose}
      icon={<AppIcon path={i.icon} name={i.name} size={40} />}
      title={i.name}
      subtitle={
        <>
          <span className="truncate font-mono select-text">{id}</span>
          <CopyButton text={id} what="来源 ID" />
        </>
      }
      badges={
        <>
          <SourceBadge type={i.source.type} />
          {i.major && <Pill tone="warning">大版本</Pill>}
          {i.running && (
            <Pill tone="warning">
              <AppWindowIcon />
              正在运行
            </Pill>
          )}
          {i.pending && (
            <Pill tone="muted">
              <ClockIcon />
              在任务中
            </Pill>
          )}
        </>
      }
      actions={
        <>
          <Button size="sm" onClick={onUpdate} disabled={i.pending}>
            <ArrowUpCircleIcon />
            更新到 {i.latest}
          </Button>
          <UpdateMenu
            target={{
              key: i.key,
              name: i.name,
              mode: i.mode,
              hold: i.hold,
              latest: i.latest,
              canAuto: i.canAuto,
              autoNote: i.autoNote,
            }}
          />
        </>
      }
    >
      <PackageInfo
        pkgRef={{ type: i.source.type, id }}
        latest={i.latest}
        facts={facts}
        first={[
          {
            value: "notes",
            label: "更新日志",
            icon: ScrollTextIcon,
            content: (
              <ReleaseNotesView
                pkgRef={{ type: i.source.type, id }}
                from={i.version}
                to={i.latest}
                lead={lead || undefined}
              />
            ),
          },
        ]}
      />
    </DetailPanel>
  );
}
