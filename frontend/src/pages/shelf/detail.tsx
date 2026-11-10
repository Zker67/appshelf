// The details of an app of the catalog: what AppShelf knows of it first,
// its groups changed in place, then what its source says; how it installs
// and is found in a tab of its own, the user's note in another.

import { AppIcon } from "@/components/app-icon";
import { CopyButton, InfoCard, InfoRow } from "@/components/common";
import { DetailPanel } from "@/components/detail-panel";
import { PackageInfo, SourceLink, type Fact } from "@/components/package-info";
import { CategoryPicker } from "@/components/pickers";
import { SourceBadge } from "@/components/source-badge";
import { StatusBadge } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { sourceID } from "@/lib/shelf";
import type { CategoryInfo, ShelfApp, TagInfo } from "@/mygo";
import {
  ArrowUpCircleIcon,
  BoxIcon,
  ChevronsUpDownIcon,
  DownloadIcon,
  FileArchiveIcon,
  FolderIcon,
  FolderSearchIcon,
  HashIcon,
  InfoIcon,
  LinkIcon,
  NotebookPenIcon,
  PencilIcon,
  PlusIcon,
  RegexIcon,
  ScanSearchIcon,
  SlidersHorizontalIcon,
  TagsIcon,
  TerminalIcon,
  Trash2Icon,
  type LucideIcon,
} from "lucide-react";
import type { ReactNode } from "react";
import { NoteTab } from "./note";
import { assignCategory } from "./ops";
import { TagMenu } from "./tag-menu";

function Mono({ children }: { children: ReactNode }) {
  return <span className="font-mono text-[13px] break-all">{children}</span>;
}

export function ShelfDetail({
  item,
  categories,
  tags,
  onClose,
  onInstall,
  onUpgrade,
  onEdit,
  onDelete,
}: {
  item: ShelfApp;
  categories: Map<string, CategoryInfo>;
  tags: Map<string, TagInfo>;
  onClose: () => void;
  onInstall: () => void;
  onUpgrade: () => void;
  onEdit: () => void;
  onDelete: () => void;
}) {
  const { app, status } = item;
  const src = app.source;
  const id = sourceID(src);
  const names = new Map([...categories.values()].map((c) => [c.id, c.name]));
  const cat = app.categoryId ? categories.get(app.categoryId) : undefined;
  const tagList = (app.tagIds ?? []).map((t) => tags.get(t)).filter((t): t is TagInfo => !!t);
  const named = [{ id: app.id, name: app.name }];
  const facts: Fact[] = [];
  if (status?.version) {
    facts.push({
      label: "已装版本",
      icon: BoxIcon,
      value: status.version,
      title: status.version,
    });
  }
  if (status?.asset) {
    facts.push({
      label: "将安装的文件",
      icon: FileArchiveIcon,
      value: status.asset,
      title: status.asset,
      mono: true,
      wide: true,
    });
  }
  // What decides how it installs and how it is found: seldom looked at.
  const rules = [
    { label: "检测名称", icon: ScanSearchIcon, value: app.detect?.displayName },
    { label: "检测路径", icon: FolderSearchIcon, value: app.detect?.exePath },
    { label: "安装包匹配", icon: RegexIcon, value: src.assetPattern },
    { label: "安装参数", icon: TerminalIcon, value: src.installerArgs },
    { label: "bucket 地址", icon: LinkIcon, value: src.bucketUrl },
  ].filter((r): r is { label: string; icon: LucideIcon; value: string } => !!r.value);
  return (
    <DetailPanel
      key={app.id}
      onClose={onClose}
      icon={<AppIcon path={item.icon} name={app.name} size={40} />}
      title={app.name}
      subtitle={
        <>
          <span className="truncate font-mono select-text">{id}</span>
          <CopyButton text={id} what="来源 ID" />
          <SourceLink pkgRef={{ type: src.type, id }} />
        </>
      }
      badges={
        <>
          <StatusBadge status={status} checking={item.checking} hold={item.hold} />
          <SourceBadge type={src.type} />
        </>
      }
      actions={
        <>
          {status?.state === "updateAvailable" && (
            <Button size="sm" onClick={onUpgrade} disabled={item.pending}>
              <ArrowUpCircleIcon />
              更新到 {status.latest}
            </Button>
          )}
          {status?.state === "notInstalled" && (
            <Button size="sm" onClick={onInstall} disabled={item.pending}>
              <DownloadIcon />
              安装
            </Button>
          )}
          <Button size="sm" variant="outline" onClick={onEdit}>
            <PencilIcon />
            编辑
          </Button>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                size="icon-sm"
                variant="ghost"
                className="ml-auto text-muted-foreground hover:text-destructive"
                onClick={onDelete}
                aria-label="从清单删除"
              >
                <Trash2Icon />
              </Button>
            </TooltipTrigger>
            <TooltipContent>从清单删除</TooltipContent>
          </Tooltip>
        </>
      }
    >
      <PackageInfo
        pkgRef={{ type: src.type, id }}
        latest={status?.latest}
        facts={facts}
        overview={
          <InfoCard>
            <InfoRow label="分类" icon={FolderIcon}>
              <CategoryPicker
                value={{ id: app.categoryId ?? "" }}
                onChange={(c) => void assignCategory(named, c, names)}
              >
                <button
                  type="button"
                  className="-mx-1.5 inline-flex max-w-full items-center gap-1.5 rounded-md px-1.5 py-0.5 transition-colors hover:bg-muted"
                >
                  <FolderIcon className="size-3.5 shrink-0 text-muted-foreground" />
                  <span className="truncate">{cat?.name ?? "未分类"}</span>
                  <ChevronsUpDownIcon className="size-3 shrink-0 text-muted-foreground" />
                </button>
              </CategoryPicker>
            </InfoRow>
            <InfoRow label="标签" icon={TagsIcon}>
              <div className="flex flex-wrap items-center gap-1">
                {tagList.map((t) => (
                  <span
                    key={t.id}
                    className="inline-flex h-6 items-center gap-0.5 rounded-md border px-1.5 text-xs"
                  >
                    <HashIcon className="size-3 text-muted-foreground" />
                    {t.name}
                  </span>
                ))}
                <TagMenu apps={[app]} align="start">
                  <button
                    type="button"
                    className="inline-flex h-6 items-center gap-0.5 rounded-md border border-dashed px-1.5 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                  >
                    <PlusIcon className="size-3" />
                    标签
                  </button>
                </TagMenu>
              </div>
            </InfoRow>
            {status?.note && (
              <InfoRow label="说明" icon={InfoIcon}>
                {status.note}
              </InfoRow>
            )}
          </InfoCard>
        }
        tabs={[
          ...(rules.length > 0
            ? [
                {
                  value: "rules",
                  label: "安装规则",
                  icon: SlidersHorizontalIcon,
                  content: (
                    <InfoCard>
                      {rules.map(({ label, icon, value }) => (
                        <InfoRow key={label} label={label} icon={icon}>
                          <Mono>{value}</Mono>
                        </InfoRow>
                      ))}
                    </InfoCard>
                  ),
                },
              ]
            : []),
          { value: "note", label: "备注", icon: NotebookPenIcon, content: <NoteTab item={item} /> },
        ]}
      />
    </DetailPanel>
  );
}
