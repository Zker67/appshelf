// The guide: what each page does, how installing stays safe, the keys.

import { DiscoverIcon, InstalledIcon, ShelfIcon, UninstallIcon } from "@/components/nav-icons";
import { Pill } from "@/components/status-badge";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Kbd } from "@/components/ui/kbd";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  ArrowUpCircleIcon,
  BanIcon,
  CheckCircle2Icon,
  CheckIcon,
  CircleHelpIcon,
  CircleIcon,
  FileUpIcon,
  FolderTreeIcon,
  HardDriveIcon,
  KeyboardIcon,
  LayoutDashboardIcon,
  ListChecksIcon,
  NotebookPenIcon,
  PencilLineIcon,
  PlusIcon,
  RefreshCwIcon,
  RocketIcon,
  ScanSearchIcon,
  SearchCheckIcon,
  SearchIcon,
  ShieldCheckIcon,
  Trash2Icon,
  TriangleAlertIcon,
  type LucideIcon,
} from "lucide-react";
import type { ComponentType, ReactNode, SVGProps } from "react";

const tabs: Array<{
  value: string;
  label: string;
  icon: ComponentType<SVGProps<SVGSVGElement>>;
  body: () => ReactNode;
}> = [
  { value: "start", label: "快速上手", icon: RocketIcon, body: Start },
  { value: "installed", label: "本机已装", icon: InstalledIcon, body: InstalledHelp },
  { value: "shelf", label: "我的清单", icon: ShelfIcon, body: ShelfHelp },
  { value: "discover", label: "发现", icon: DiscoverIcon, body: DiscoverHelp },
  { value: "uninstall", label: "卸载", icon: UninstallIcon, body: UninstallHelp },
  { value: "keys", label: "快捷键", icon: KeyboardIcon, body: Keys },
];

export function HelpDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex h-[min(640px,calc(100vh-6rem))] flex-col gap-0 p-0 sm:max-w-4xl">
        <DialogHeader className="border-b px-5 py-3.5">
          <DialogTitle className="flex items-center gap-2 text-lg">
            <CircleHelpIcon className="size-5 text-primary" />
            使用说明
          </DialogTitle>
        </DialogHeader>
        <Tabs
          defaultValue="start"
          orientation="vertical"
          className="flex min-h-0 flex-1 flex-row gap-0"
        >
          <TabsList className="h-full! w-44 shrink-0 flex-col items-stretch justify-start gap-0.5 rounded-none border-r bg-sidebar p-2">
            {tabs.map(({ value, label, icon: Icon }) => (
              <TabsTrigger
                key={value}
                value={value}
                className="group/tab h-9 flex-none justify-start gap-2 px-3 data-active:bg-card data-active:shadow-sm"
              >
                <Icon className="size-4 transition-colors group-data-active/tab:text-primary" />
                {label}
              </TabsTrigger>
            ))}
          </TabsList>
          <ScrollArea className="min-h-0 flex-1 bg-background">
            <div className="px-6 py-5">
              {tabs.map(({ value, body: Body }) => (
                <TabsContent key={value} value={value} className="flex flex-col gap-6">
                  <Body />
                </TabsContent>
              ))}
            </div>
          </ScrollArea>
        </Tabs>
      </DialogContent>
    </Dialog>
  );
}

/** Ui names a button or a page as it shows in the window. */
function Ui({ children }: { children: ReactNode }) {
  return (
    <span className="mx-0.5 inline-flex h-5 items-center rounded-md border bg-card px-1.5 align-[1px] text-xs font-medium text-foreground shadow-xs">
      {children}
    </span>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-3">
      <h3 className="text-[13px] font-semibold text-muted-foreground">{title}</h3>
      {children}
    </section>
  );
}

/** rise plays each card in a little after the one before. */
function rise(i: number) {
  return { animationDelay: `${i * 40}ms` };
}

interface Feature {
  icon: LucideIcon;
  title: string;
  text: ReactNode;
}

function Features({ items }: { items: Feature[] }) {
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      {items.map(({ icon: Icon, title, text }, i) => (
        <div
          key={title}
          style={rise(i)}
          className="anim-rise flex gap-3 rounded-xl border bg-card p-4 shadow-xs transition-colors hover:border-primary/30"
        >
          <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
            <Icon className="size-[18px]" />
          </span>
          <div className="flex min-w-0 flex-col gap-1">
            <span className="text-sm font-medium">{title}</span>
            <span className="text-[13px] leading-relaxed text-muted-foreground">{text}</span>
          </div>
        </div>
      ))}
    </div>
  );
}

function Start() {
  const steps: Feature[] = [
    {
      icon: SearchIcon,
      title: "搜索软件",
      text: (
        <>
          在<Ui>发现</Ui>输入软件名，winget 和 scoop 的结果会一起列出。
        </>
      ),
    },
    {
      icon: PlusIcon,
      title: "加入清单",
      text: (
        <>
          先在搜索框旁的<Ui>加入到</Ui>选好分类和标签，之后点<Ui>加入</Ui>就直接放进去。
        </>
      ),
    },
    {
      icon: ListChecksIcon,
      title: "安装和更新",
      text: (
        <>
          在<Ui>我的清单</Ui>勾选软件，点<Ui>安装</Ui>或<Ui>更新</Ui>。
        </>
      ),
    },
    {
      icon: FileUpIcon,
      title: "换电脑",
      text: <>导出清单，在新电脑上导入，再一次装好。</>,
    },
  ];
  return (
    <>
      <Section title="四步开始">
        <ol className="grid gap-3 sm:grid-cols-2">
          {steps.map(({ icon: Icon, title, text }, i) => (
            <li
              key={title}
              style={rise(i)}
              className="anim-rise relative flex flex-col gap-2 rounded-xl border bg-card p-4 shadow-xs"
            >
              <span className="flex items-center gap-2.5">
                <span className="flex size-6 items-center justify-center rounded-full bg-primary text-xs font-semibold text-primary-foreground tabular-nums">
                  {i + 1}
                </span>
                <span className="text-sm font-medium">{title}</span>
                <Icon className="ml-auto size-4 text-muted-foreground" />
              </span>
              <span className="text-[13px] leading-relaxed text-muted-foreground">{text}</span>
            </li>
          ))}
        </ol>
      </Section>
      <div
        style={rise(4)}
        className="anim-rise flex gap-3.5 rounded-xl border border-success/25 bg-success/[0.06] p-4"
      >
        <ShieldCheckIcon className="mt-0.5 size-5 shrink-0 text-success" />
        <div className="flex flex-col gap-2">
          <span className="text-sm font-medium">放心安装</span>
          <ul className="flex flex-col gap-1.5 text-[13px] text-foreground/85">
            {[
              "每次安装前先预览，确认后才开始",
              "已经装过的软件会自动跳过",
              "状态不确定的软件不会安装",
            ].map((t) => (
              <li key={t} className="flex items-center gap-2">
                <CheckIcon className="size-3.5 shrink-0 text-success" />
                {t}
              </li>
            ))}
          </ul>
        </div>
      </div>
    </>
  );
}

function InstalledHelp() {
  return (
    <Features
      items={[
        {
          icon: LayoutDashboardIcon,
          title: "概览",
          text: "程序总数、可更新数量和来源分布，点任意一块查看对应的列表。",
        },
        {
          icon: ArrowUpCircleIcon,
          title: "直接更新",
          text: "来自 winget 或 scoop 的程序，有新版本时可以直接更新。",
        },
        {
          icon: PlusIcon,
          title: "加入清单",
          text: "把常用的程序加入清单，换电脑时一起装回来。",
        },
        {
          icon: ScanSearchIcon,
          title: "查找来源",
          text: (
            <>
              不知道从哪装的程序，点<Ui>查找来源</Ui>到发现里找。
            </>
          ),
        },
      ]}
    />
  );
}

function UninstallHelp() {
  return (
    <Features
      items={[
        {
          icon: HardDriveIcon,
          title: "按大小和时间挑",
          text: "左侧按占用空间、安装时间和来源组合筛选，默认最占地方的在前，也可以按最早安装排序。",
        },
        {
          icon: ShieldCheckIcon,
          title: "先看再卸",
          text: (
            <>
              点<Ui>卸载</Ui>
              先看卸载方式、命令和发布者签名，确认后才执行；卸完再查一次，还在就记为失败。
            </>
          ),
        },
        {
          icon: SearchCheckIcon,
          title: "清理残留",
          text: (
            <>
              卸载成功后在<Ui>最近卸载</Ui>里点<Ui>查找残留</Ui>
              ，逐项确认后清理：文件进回收站，注册表先备份。
            </>
          ),
        },
      ]}
    />
  );
}

function ShelfHelp() {
  const states: Array<{
    tone: "primary" | "success" | "muted" | "warning" | "danger";
    icon: LucideIcon;
    label: string;
    text: string;
  }> = [
    { tone: "primary", icon: ArrowUpCircleIcon, label: "可更新", text: "有新版本" },
    { tone: "success", icon: CheckCircle2Icon, label: "已安装", text: "这台电脑上已经有了" },
    { tone: "muted", icon: CircleIcon, label: "未安装", text: "可以安装" },
    {
      tone: "warning",
      icon: TriangleAlertIcon,
      label: "无法确认",
      text: "暂时读不到状态，先不处理",
    },
    { tone: "danger", icon: BanIcon, label: "不可用", text: "本机没有这个来源，比如没装 scoop" },
  ];
  return (
    <>
      <Section title="状态">
        <div className="flex flex-col divide-y rounded-xl border bg-card shadow-xs">
          {states.map(({ tone, icon: Icon, label, text }, i) => (
            <div
              key={label}
              style={rise(i)}
              className="anim-rise flex items-center gap-4 px-4 py-2.5"
            >
              <span className="w-24 shrink-0">
                <Pill tone={tone}>
                  <Icon />
                  {label}
                </Pill>
              </span>
              <span className="text-[13px] text-muted-foreground">{text}</span>
            </div>
          ))}
        </div>
      </Section>
      <Section title="常用操作">
        <Features
          items={[
            {
              icon: FolderTreeIcon,
              title: "分类和标签",
              text: "左侧可以多选筛选，名称旁边能重命名和删除。",
            },
            {
              icon: NotebookPenIcon,
              title: "备注",
              text: (
                <>
                  详情的<Ui>备注</Ui>页签或右键<Ui>写备注</Ui>，用 Markdown
                  记下设置、激活方式等，边写边预览、自动保存，搜索也能搜到。
                </>
              ),
            },
            {
              icon: ListChecksIcon,
              title: "批量操作",
              text: "勾选多个软件，底部会出现安装、更新、设分类等按钮。",
            },
            {
              icon: RefreshCwIcon,
              title: "检查更新",
              text: (
                <>
                  点<Ui>检查更新</Ui>刷新所有软件的状态。AppShelf 自己的更新在<Ui>设置</Ui>
                  里。
                </>
              ),
            },
            {
              icon: Trash2Icon,
              title: "删除",
              text: "只是移出清单，不会卸载软件，删错了可以撤销。",
            },
          ]}
        />
      </Section>
    </>
  );
}

function DiscoverHelp() {
  return (
    <Features
      items={[
        {
          icon: SearchIcon,
          title: "搜索",
          text: "输入软件名就会搜索 winget 和 scoop，已在清单、已安装的会标出来。",
        },
        {
          icon: ScanSearchIcon,
          title: "搜 GitHub",
          text: (
            <>
              点<Ui>搜 GitHub</Ui>或按 <Kbd>Enter</Kbd>，再搜一遍 GitHub。
            </>
          ),
        },
        {
          icon: PencilLineIcon,
          title: "手动添加",
          text: "搜软件名或粘贴 GitHub 仓库链接，选中就能加入。",
        },
        {
          icon: ShieldCheckIcon,
          title: "检测规则",
          text: "用来判断软件是否已安装。加入 GitHub 软件时会自动推荐。",
        },
      ]}
    />
  );
}

function Keys() {
  const rows: Array<[ReactNode, string]> = [
    [
      <>
        <Kbd>Ctrl</Kbd>
        <Kbd>F</Kbd>
      </>,
      "搜索",
    ],
    [<Kbd key="esc">Esc</Kbd>, "清空搜索、取消选择"],
    [
      <>
        <Kbd>Ctrl</Kbd>
        <Kbd>A</Kbd>
      </>,
      "全选",
    ],
    [
      <>
        <Kbd>Shift</Kbd>
        <span className="text-xs text-muted-foreground">+ 点击</span>
      </>,
      "连续选择",
    ],
    [
      <>
        <Kbd>Ctrl</Kbd>
        <span className="text-xs text-muted-foreground">+ 点击</span>
      </>,
      "多选",
    ],
    [
      <>
        <Kbd>↑</Kbd>
        <Kbd>↓</Kbd>
      </>,
      "上下移动",
    ],
    [<Kbd key="space">空格</Kbd>, "勾选当前项"],
    [<Kbd key="enter">Enter</Kbd>, "加入当前项，已在清单的去清单查看"],
    [<Kbd key="right">右键</Kbd>, "更多操作"],
  ];
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      {rows.map(([k, v], i) => (
        <div
          key={v}
          style={rise(i)}
          className="anim-rise flex h-12 items-center justify-between gap-4 rounded-xl border bg-card px-4 shadow-xs"
        >
          <span className="text-[13px]">{v}</span>
          <span className="flex items-center gap-1">{k}</span>
        </div>
      ))}
    </div>
  );
}
