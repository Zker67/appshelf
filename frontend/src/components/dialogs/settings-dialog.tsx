// The settings: the sources and GitHub's quotas, and AppShelf itself.

import logo from "@/assets/logo.png";
import { CopyButton } from "@/components/common";
import { SourceIcon } from "@/components/source-badge";
import { Pill } from "@/components/status-badge";
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
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { errorText, useAbout, useShelf, useSources } from "@/lib/api";
import { sourceLabel } from "@/lib/shelf";
import { formatDateTime } from "@/lib/text";
import { About, Shelf, Sources, type RateInfo, type SourceInfo } from "@/mygo";
import {
  CheckCircle2Icon,
  CircleDashedIcon,
  DownloadCloudIcon,
  FolderOpenIcon,
  GaugeIcon,
  InfoIcon,
  KeyRoundIcon,
  LoaderCircleIcon,
  PlugIcon,
  RefreshCwIcon,
  SearchIcon,
  SettingsIcon,
  TriangleAlertIcon,
  WrenchIcon,
} from "lucide-react";
import { useState, type ReactNode } from "react";
import { toast } from "sonner";

export type SettingsTab = "sources" | "about";

export function SettingsDialog({
  open,
  tab,
  onOpenChange,
}: {
  open: boolean;
  tab: SettingsTab;
  onOpenChange: (open: boolean) => void;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex h-[min(600px,calc(100vh-6rem))] flex-col gap-0 p-0 sm:max-w-2xl">
        <Tabs key={tab} defaultValue={tab} className="flex min-h-0 flex-1 flex-col gap-0">
          <DialogHeader className="flex-row items-center gap-3 border-b px-5 py-3.5 pr-12">
            <DialogTitle className="flex items-center gap-2 text-lg">
              <SettingsIcon className="size-5 text-primary" />
              设置
            </DialogTitle>
            <TabsList className="ml-2">
              <TabsTrigger value="sources" className="px-3">
                <PlugIcon />
                安装来源
              </TabsTrigger>
              <TabsTrigger value="about" className="px-3">
                <InfoIcon />
                关于
              </TabsTrigger>
            </TabsList>
          </DialogHeader>
          <ScrollArea className="min-h-0 flex-1">
            <TabsContent value="sources" className="anim-fade px-5 py-4">
              <SourcesTab />
            </TabsContent>
            <TabsContent value="about" className="anim-fade px-5 py-4">
              <AboutTab />
            </TabsContent>
          </ScrollArea>
        </Tabs>
      </DialogContent>
    </Dialog>
  );
}

function Section({
  title,
  icon: Icon,
  children,
  actions,
}: {
  title: string;
  icon: typeof InfoIcon;
  children: ReactNode;
  actions?: ReactNode;
}) {
  return (
    <section className="flex flex-col gap-2.5">
      <div className="flex items-center gap-2">
        <Icon className="size-4 text-muted-foreground" />
        <h3 className="text-sm font-semibold">{title}</h3>
        <div className="ml-auto flex items-center gap-1.5">{actions}</div>
      </div>
      {children}
    </section>
  );
}

function SourcesTab() {
  const { data } = useSources();
  const { data: shelf } = useShelf();
  const refreshing = shelf?.refreshing ?? false;
  return (
    <div className="flex flex-col gap-6">
      <Section
        title="来源状态"
        icon={PlugIcon}
        actions={
          <Button
            variant="outline"
            size="sm"
            disabled={refreshing}
            onClick={() =>
              Shelf.refresh().catch((err: unknown) =>
                toast.error("检查失败", { description: errorText(err) }),
              )
            }
          >
            <RefreshCwIcon className={refreshing ? "animate-spin" : undefined} />
            重新检查
          </Button>
        }
      >
        <div className="flex flex-col divide-y rounded-xl border bg-card">
          {(data?.sources ?? []).map((s) => (
            <SourceRow
              key={s.type}
              info={s}
              refreshing={refreshing}
              canFix={data?.canInstallModule ?? false}
            />
          ))}
        </div>
      </Section>
      <Section title="GitHub 请求额度" icon={GaugeIcon}>
        <div className="flex flex-col divide-y rounded-xl border bg-card">
          <div className="flex items-center gap-3 px-4 py-3">
            <KeyRoundIcon className="size-4 text-muted-foreground" />
            <span className="w-24 shrink-0 text-sm">token</span>
            {data?.githubToken ? (
              <Pill tone="success">
                <CheckCircle2Icon />
                已使用
              </Pill>
            ) : (
              <>
                <Pill tone="muted">
                  <CircleDashedIcon />
                  未设置
                </Pill>
                <span className="min-w-0 text-xs text-muted-foreground">
                  设置环境变量 <code className="font-mono">GH_TOKEN</code> 或{" "}
                  <code className="font-mono">GITHUB_TOKEN</code> 后重新打开 AppShelf
                </span>
              </>
            )}
          </div>
          <RateRow icon={PlugIcon} label="检查与安装" rate={data?.githubRate} />
          <RateRow icon={SearchIcon} label="搜索" rate={data?.githubSearchRate} />
        </div>
      </Section>
    </div>
  );
}

function SourceRow({
  info,
  refreshing,
  canFix,
}: {
  info: SourceInfo;
  refreshing: boolean;
  canFix: boolean;
}) {
  const [confirm, setConfirm] = useState(false);
  const [installing, setInstalling] = useState(false);
  const install = async () => {
    setInstalling(true);
    const id = toast.loading("正在安装 Microsoft.WinGet.Client，需要几分钟");
    try {
      await Sources.installModule();
      toast.success("已安装 winget 模块", { id });
    } catch (err) {
      toast.error("安装模块失败", { id, description: errorText(err) });
    } finally {
      setInstalling(false);
    }
  };
  return (
    <div className="flex items-center gap-3 px-4 py-3">
      <SourceIcon type={info.type} className="size-4 text-muted-foreground" />
      <span className="w-24 shrink-0 text-sm font-medium">{sourceLabel(info.type)}</span>
      {!info.checked ? (
        <Pill tone="muted">
          {refreshing ? <LoaderCircleIcon className="animate-spin" /> : <CircleDashedIcon />}
          {refreshing ? "检查中" : "尚未检查"}
        </Pill>
      ) : info.ok ? (
        <Pill tone="success">
          <CheckCircle2Icon />
          可用
        </Pill>
      ) : (
        <Pill tone="warning">
          <TriangleAlertIcon />
          不可用
        </Pill>
      )}
      <span className="min-w-0 flex-1 text-xs break-words text-muted-foreground select-text">
        {info.reason}
      </span>
      {info.type === "winget" && info.checked && !info.ok && info.fixable && canFix && (
        <>
          <Button size="sm" disabled={installing} onClick={() => setConfirm(true)}>
            {installing ? <LoaderCircleIcon className="animate-spin" /> : <WrenchIcon />}
            安装模块
          </Button>
          <AlertDialog open={confirm} onOpenChange={setConfirm}>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>安装 Microsoft.WinGet.Client？</AlertDialogTitle>
                <AlertDialogDescription>
                  AppShelf 用这个 PowerShell 模块读取 winget 的已装软件和更新。将以当前用户身份从
                  PowerShell Gallery 安装，不需要管理员权限。
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>取消</AlertDialogCancel>
                <AlertDialogAction onClick={() => void install()}>安装</AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </>
      )}
    </div>
  );
}

function RateRow({
  icon: Icon,
  label,
  rate,
}: {
  icon: typeof InfoIcon;
  label: string;
  rate?: RateInfo;
}) {
  const reset = formatDateTime(rate?.reset);
  const low = rate?.known && rate.remaining <= Math.max(1, rate.limit / 10);
  return (
    <div className="flex items-center gap-3 px-4 py-3">
      <Icon className="size-4 text-muted-foreground" />
      <span className="w-24 shrink-0 text-sm">{label}</span>
      {rate?.known ? (
        <>
          <div className="h-1.5 w-28 overflow-hidden rounded-full bg-muted">
            <div
              className={
                low
                  ? "h-full rounded-full bg-warning transition-[width]"
                  : "h-full rounded-full bg-primary transition-[width]"
              }
              style={{
                width: `${rate.limit ? Math.round((rate.remaining / rate.limit) * 100) : 0}%`,
              }}
            />
          </div>
          <span className="text-sm tabular-nums">
            剩余 {rate.remaining} / {rate.limit} 次
          </span>
          {reset && (
            <span className="ml-auto text-xs text-muted-foreground tabular-nums">{reset} 重置</span>
          )}
        </>
      ) : (
        <span className="text-xs text-muted-foreground">还没有请求过</span>
      )}
    </div>
  );
}

function AboutTab() {
  const { data } = useAbout();
  const [checking, setChecking] = useState(false);
  const check = async () => {
    setChecking(true);
    try {
      await About.checkForUpdates();
    } catch (err) {
      toast.error("检查更新失败", { description: errorText(err) });
    } finally {
      setChecking(false);
    }
  };
  const open = (what: string, fn: () => Promise<void>) =>
    fn().catch((err: unknown) => toast.error(`无法打开${what}`, { description: errorText(err) }));
  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center gap-4 rounded-xl border bg-gradient-to-br from-primary/10 via-card to-card p-4">
        <img src={logo} alt="" className="size-14 drop-shadow-md" draggable={false} />
        <div className="min-w-0 flex-1">
          <div className="text-base font-semibold">AppShelf 软件商城</div>
          <div className="mt-1 flex items-center gap-2">
            <Pill tone="primary">
              {data?.version ? `v${data.version.replace(/^v/, "")}` : "开发版本"}
            </Pill>
          </div>
        </div>
        {data?.canUpdate ? (
          <Button onClick={check} disabled={checking}>
            {checking ? <LoaderCircleIcon className="animate-spin" /> : <DownloadCloudIcon />}
            检查更新
          </Button>
        ) : (
          <span className="max-w-48 text-right text-xs text-muted-foreground">
            自更新只在用安装包安装的版本里可用
          </span>
        )}
      </div>
      <Section title="文件位置" icon={FolderOpenIcon}>
        <div className="flex flex-col divide-y rounded-xl border bg-card">
          <PathRow
            label="清单与数据"
            path={data?.dataDir ?? ""}
            onOpen={() => open("数据目录", () => About.openDataDir())}
          />
          <PathRow
            label="安装日志"
            path={data?.logsDir ?? ""}
            onOpen={() => open("日志目录", () => About.openLogs())}
          />
        </div>
      </Section>
    </div>
  );
}

function PathRow({ label, path, onOpen }: { label: string; path: string; onOpen: () => void }) {
  return (
    <div className="flex items-center gap-3 px-4 py-2.5">
      <span className="w-24 shrink-0 text-sm">{label}</span>
      <span
        className="min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground select-text"
        title={path}
      >
        {path}
      </span>
      {path && <CopyButton text={path} what="路径" />}
      <Button variant="outline" size="sm" onClick={onOpen} disabled={!path}>
        <FolderOpenIcon />
        打开
      </Button>
    </div>
  );
}
