// The settings: the sources and GitHub's quotas, the updates, and
// AppShelf itself.

import logo from "@/assets/logo.png";
import { CopyButton, Hint } from "@/components/common";
import { TokenRow } from "@/components/github-token";
import { SourceIcon } from "@/components/source-badge";
import { needsSetup, runSetup, SetupButton, setupFor } from "@/components/source-setup";
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
import {
  errorText,
  keys,
  queryClient,
  useAbout,
  useShelf,
  useSources,
  useUpdateSettings,
} from "@/lib/api";
import { sourceLabel } from "@/lib/shelf";
import { checkSelf, installSelf, openReleases, useSelfUpdate } from "@/lib/self-update";
import { formatDateTime } from "@/lib/text";
import {
  About,
  Shelf,
  Updates,
  type CheckInterval,
  type RateInfo,
  type SelfCheckMode,
  type SourceInfo,
  type UpdateSettings,
} from "@/mygo";
import {
  ArrowUpCircleIcon,
  BellRingIcon,
  CheckCircle2Icon,
  CircleDashedIcon,
  DownloadCloudIcon,
  DownloadIcon,
  ExternalLinkIcon,
  FolderOpenIcon,
  GaugeIcon,
  InfoIcon,
  LoaderCircleIcon,
  MonitorDownIcon,
  PanelBottomIcon,
  PlugIcon,
  PowerIcon,
  RefreshCwIcon,
  SearchIcon,
  SettingsIcon,
  TimerIcon,
  TriangleAlertIcon,
} from "lucide-react";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { useState, type ReactNode } from "react";
import { toast } from "sonner";

export type SettingsTab = "sources" | "updates" | "about";

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
              <TabsTrigger value="updates" className="px-3">
                <ArrowUpCircleIcon />
                更新
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
            <TabsContent value="updates" className="anim-fade px-5 py-4">
              <UpdatesTab />
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
  hint,
  children,
  actions,
}: {
  title: string;
  icon: typeof InfoIcon;
  /** What to know of the whole section, on hover. */
  hint?: ReactNode;
  children: ReactNode;
  actions?: ReactNode;
}) {
  return (
    <section className="flex flex-col gap-2.5">
      <div className="flex items-center gap-2">
        <Icon className="size-4 text-muted-foreground" />
        <h3 className="text-sm font-semibold">{title}</h3>
        {hint && <Hint>{hint}</Hint>}
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
            <SourceRow key={s.type} info={s} refreshing={refreshing} />
          ))}
        </div>
      </Section>
      <Section title="GitHub 请求额度" icon={GaugeIcon}>
        <div className="flex flex-col divide-y rounded-xl border bg-card">
          <TokenRow from={data?.githubTokenFrom ?? ""} />
          <RateRow icon={PlugIcon} label="检查与安装" rate={data?.githubRate} />
          <RateRow icon={SearchIcon} label="搜索" rate={data?.githubSearchRate} />
        </div>
      </Section>
    </div>
  );
}

function SourceRow({ info, refreshing }: { info: SourceInfo; refreshing: boolean }) {
  const [confirm, setConfirm] = useState(false);
  const setup = setupFor(info);
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
      {info.checked && info.ok && info.reason && <Hint>{info.reason}</Hint>}
      <span className="min-w-0 flex-1 text-xs break-words text-muted-foreground select-text">
        {info.checked && !info.ok && info.reason}
      </span>
      {setup && needsSetup(info) && (
        <>
          <SetupButton type={info.type} setup={setup} onStart={() => setConfirm(true)} />
          <AlertDialog open={confirm} onOpenChange={setConfirm}>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>安装 {setup.name}？</AlertDialogTitle>
                <AlertDialogDescription>
                  用于{setup.use}。{setup.how}。
                </AlertDialogDescription>
              </AlertDialogHeader>
              <code className="rounded-lg border bg-muted/50 px-2.5 py-1.5 font-mono text-xs break-all select-text">
                {setup.command}
              </code>
              <AlertDialogFooter>
                <AlertDialogCancel>取消</AlertDialogCancel>
                <AlertDialogAction onClick={() => void runSetup(info.type, setup)}>
                  <DownloadIcon />
                  安装
                </AlertDialogAction>
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

const intervals: Array<{ value: CheckInterval; label: string }> = [
  { value: "1h", label: "每小时" },
  { value: "3h", label: "每 3 小时" },
  { value: "6h", label: "每 6 小时" },
  { value: "12h", label: "每 12 小时" },
  { value: "24h", label: "每天" },
  { value: "off", label: "只在启动时" },
];

/** useSaveUpdateSettings saves a change of the update settings, shown at once. */
function useSaveUpdateSettings() {
  const { data } = useUpdateSettings();
  const save = async (patch: Partial<UpdateSettings>) => {
    if (!data) return;
    const key = [...keys.updates, "settings"];
    const next = { ...data, ...patch };
    queryClient.setQueryData(key, next);
    try {
      await Updates.setSettings(next);
    } catch (err) {
      toast.error("设置没有保存", { description: errorText(err) });
    } finally {
      await queryClient.invalidateQueries({ queryKey: key });
    }
  };
  return { data, save };
}

function UpdatesTab() {
  const { data, save } = useSaveUpdateSettings();
  if (!data) return null;
  return (
    <div className="flex flex-col gap-6">
      <Section
        title="检查更新"
        icon={RefreshCwIcon}
        hint="自动更新在每个软件的更新方式里开启。只有 scoop 和为当前用户安装的 winget 软件能自动更新，正在运行的会等它关闭；其余的只提醒。"
      >
        <div className="flex flex-col divide-y rounded-xl border bg-card">
          <SettingRow icon={TimerIcon} label="自动检查" hint="软件图书馆运行时按这个间隔检查一次">
            <Select
              value={data.checkEvery}
              onValueChange={(v) => void save({ checkEvery: v as CheckInterval })}
            >
              <SelectTrigger size="sm" className="w-32">
                <SelectValue />
              </SelectTrigger>
              <SelectContent align="end">
                {intervals.map((i) => (
                  <SelectItem key={i.value} value={i.value}>
                    {i.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </SettingRow>
          <SettingRow
            icon={BellRingIcon}
            label="发现更新时通知"
            hint="窗口不在前面时发系统通知，自动更新完成后也会告诉你"
          >
            <Switch checked={data.notify} onCheckedChange={(on) => void save({ notify: on })} />
          </SettingRow>
        </div>
      </Section>
      <Section title="后台运行" icon={PanelBottomIcon}>
        <div className="flex flex-col divide-y rounded-xl border bg-card">
          <SettingRow
            icon={PanelBottomIcon}
            label="关闭窗口后留在托盘"
            hint={
              data.canBackground
                ? "继续定期检查、执行任务；单击托盘图标打开窗口，右键查看更新和任务。关掉后，关闭窗口即退出"
                : "这个版本不支持托盘"
            }
          >
            <Switch
              checked={data.background}
              disabled={!data.canBackground}
              onCheckedChange={(on) => void save({ background: on })}
            />
          </SettingRow>
          <SettingRow
            icon={PowerIcon}
            label="开机时启动"
            hint={data.background ? "启动后留在托盘，不打开窗口" : "开机后打开软件图书馆"}
          >
            <Switch
              checked={data.openAtLogin}
              disabled={!data.canBackground}
              onCheckedChange={(on) => void save({ openAtLogin: on })}
            />
          </SettingRow>
        </div>
      </Section>
    </div>
  );
}

function SettingRow({
  icon: Icon,
  label,
  hint,
  children,
}: {
  icon: typeof InfoIcon;
  label: string;
  hint?: string;
  children: ReactNode;
}) {
  return (
    <div className="flex items-center gap-3 px-4 py-3">
      <Icon className="size-4 shrink-0 text-muted-foreground" />
      <div className="flex min-w-0 flex-1 items-center gap-1.5">
        <span className="text-sm">{label}</span>
        {hint && <Hint>{hint}</Hint>}
      </div>
      {children}
    </div>
  );
}

/** When AppShelf checks for a newer version of itself, as one choice. */
const selfChecks: Array<{ value: string; label: string; check: SelfCheckMode; every?: number }> = [
  { value: "launch", label: "启动时检查", check: "launch" },
  { value: "1", label: "每小时检查", check: "interval", every: 1 },
  { value: "6", label: "每 6 小时检查", check: "interval", every: 6 },
  { value: "24", label: "每天检查", check: "interval", every: 24 },
  { value: "off", label: "不自动检查", check: "off" },
];

function AboutTab() {
  const { data } = useAbout();
  const { data: self } = useSelfUpdate();
  const { data: settings, save } = useSaveUpdateSettings();
  const [checkError, setCheckError] = useState("");
  const [checked, setChecked] = useState(false);
  const phase = self?.phase;
  const enabled = self?.enabled ?? false;
  const found = phase === "available" || phase === "failed";
  const working = phase === "checking" || phase === "downloading" || phase === "restarting";
  const check = async () => {
    setCheckError("");
    setChecked(false);
    const err = await checkSelf();
    setCheckError(err);
    setChecked(err === "");
  };
  let status = "";
  if (self && !enabled) status = "开发版本，不能自动更新";
  else if (phase === "checking") status = "正在检查…";
  else if (phase === "downloading")
    status = `正在更新到 ${self?.available}，已下载 ${Math.floor((self?.progress ?? 0) * 100)}%`;
  else if (phase === "restarting") status = "已更新，马上重新打开";
  else if (phase === "failed") status = self?.error ?? "更新失败";
  else if (found) status = `发现新版本 ${self?.available}`;
  else if (checkError) status = checkError;
  else if (checked) status = "已经是最新版本";
  const bad = phase === "failed" || (!!checkError && !found);
  const schedule =
    settings?.selfCheck === "interval"
      ? String(settings.selfEvery)
      : (settings?.selfCheck ?? "launch");
  const open = (what: string, fn: () => Promise<void>) =>
    fn().catch((err: unknown) => toast.error(`无法打开${what}`, { description: errorText(err) }));
  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center gap-4 rounded-xl border bg-gradient-to-br from-primary/10 via-card to-card p-4">
        <img src={logo} alt="" className="size-14 drop-shadow-md" draggable={false} />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-1.5 text-base font-semibold">
            AppShelf 软件图书馆
            <Hint>
              更新包从 GitHub 下载，校验签名后替换，完成后自动重新打开，清单和设置不受影响。
            </Hint>
          </div>
          <div className="mt-1 flex min-w-0 items-center gap-2">
            <Pill tone="primary">
              {data?.version ? `v${data.version.replace(/^v/, "")}` : "开发版本"}
            </Pill>
            {status && (
              <span
                className={
                  bad
                    ? "min-w-0 truncate text-xs text-destructive"
                    : "min-w-0 truncate text-xs text-muted-foreground tabular-nums"
                }
                title={status}
              >
                {status}
              </span>
            )}
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-1.5">
          {found ? (
            <Button onClick={installSelf}>
              <DownloadIcon />
              {phase === "failed" ? "重试" : "去更新"}
            </Button>
          ) : (
            <Button variant="outline" disabled={working || !enabled} onClick={() => void check()}>
              {phase === "checking" ? (
                <LoaderCircleIcon className="animate-spin" />
              ) : (
                <RefreshCwIcon />
              )}
              检查更新
            </Button>
          )}
          <Button variant="ghost" onClick={() => openReleases()}>
            <ExternalLinkIcon />
            所有版本
          </Button>
        </div>
      </div>
      {settings && (
        <Section
          title="新版本"
          icon={MonitorDownIcon}
          hint="启动时检查：每次打开几秒后检查一次。定时检查：启动时检查一次，之后按间隔检查。发现新版本时右下角提示，点「去更新」才下载，装好后自动重新打开。"
        >
          <div className="flex flex-col divide-y rounded-xl border bg-card">
            <SettingRow
              icon={DownloadCloudIcon}
              label="自动检查新版本"
              hint={settings.canSelfUpdate ? undefined : "开发版本不能自动更新"}
            >
              <Select
                value={schedule}
                disabled={!settings.canSelfUpdate}
                onValueChange={(v) => {
                  const c = selfChecks.find((x) => x.value === v);
                  if (c) {
                    void save({ selfCheck: c.check, selfEvery: c.every ?? settings.selfEvery });
                  }
                }}
              >
                <SelectTrigger size="sm" className="w-32">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent align="end">
                  {selfChecks.map((c) => (
                    <SelectItem key={c.value} value={c.value}>
                      {c.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </SettingRow>
          </div>
        </Section>
      )}
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
