// The details of an app of the catalog: what AppShelf knows of it first,
// its groups changed in place, then what its source says.

import { AppIcon } from "@/components/app-icon";
import { CopyButton, InfoRow } from "@/components/common";
import { DetailPanel } from "@/components/detail-panel";
import { PackageInfo } from "@/components/package-info";
import { CategoryPicker } from "@/components/pickers";
import { SourceBadge } from "@/components/source-badge";
import { StatusBadge } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { sourceID } from "@/lib/shelf";
import type { CategoryInfo, ShelfApp, TagInfo } from "@/mygo";
import {
  ArrowUpCircleIcon,
  ChevronsUpDownIcon,
  DownloadIcon,
  FolderIcon,
  HashIcon,
  PencilIcon,
  PlusIcon,
  Trash2Icon,
} from "lucide-react";
import type { ReactNode } from "react";
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
  return (
    <DetailPanel
      key={app.id}
      onClose={onClose}
      icon={<AppIcon path={item.icon} name={app.name} size={40} />}
      title={app.name}
      badges={
        <>
          <StatusBadge status={status} checking={item.checking} />
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
      <div className="flex flex-col gap-5">
        <div className="flex flex-col">
          <InfoRow label="来源">
            <span className="flex min-w-0 items-center gap-1">
              <Mono>{id}</Mono>
              <CopyButton text={id} what="来源 ID" />
            </span>
          </InfoRow>
          <InfoRow label="分类">
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
          <InfoRow label="标签">
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
          {status?.version && <InfoRow label="已装版本">{status.version}</InfoRow>}
          {status?.latest && status.latest !== status.version && (
            <InfoRow label="最新版本">{status.latest}</InfoRow>
          )}
          {status?.asset && (
            <InfoRow label="安装包">
              <Mono>{status.asset}</Mono>
            </InfoRow>
          )}
          {status?.note && <InfoRow label="说明">{status.note}</InfoRow>}
          {app.detect?.displayName && (
            <InfoRow label="检测名称">
              <Mono>{app.detect.displayName}</Mono>
            </InfoRow>
          )}
          {app.detect?.exePath && (
            <InfoRow label="检测路径">
              <Mono>{app.detect.exePath}</Mono>
            </InfoRow>
          )}
          {src.assetPattern && (
            <InfoRow label="安装包匹配">
              <Mono>{src.assetPattern}</Mono>
            </InfoRow>
          )}
          {src.installerArgs && (
            <InfoRow label="安装参数">
              <Mono>{src.installerArgs}</Mono>
            </InfoRow>
          )}
          {src.bucketUrl && (
            <InfoRow label="bucket 地址">
              <Mono>{src.bucketUrl}</Mono>
            </InfoRow>
          )}
        </div>
        <div className="h-px bg-border" />
        <PackageInfo pkgRef={{ type: src.type, id }} />
      </div>
    </DetailPanel>
  );
}
