// The guide: what each page does, how installing stays safe, the keys.

import { DiscoverIcon, InstalledIcon, ShelfIcon } from "@/components/nav-icons";
import { Pill } from "@/components/status-badge";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Kbd } from "@/components/ui/kbd";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  ArrowUpCircleIcon,
  BanIcon,
  CheckCircle2Icon,
  CircleHelpIcon,
  CircleIcon,
  KeyboardIcon,
  RocketIcon,
  TriangleAlertIcon,
} from "lucide-react";
import type { ReactNode } from "react";

const tabs = [
  { value: "start", label: "快速上手", icon: RocketIcon },
  { value: "shelf", label: "我的清单", icon: ShelfIcon },
  { value: "discover", label: "发现与添加", icon: DiscoverIcon },
  { value: "installed", label: "本机已装", icon: InstalledIcon },
  { value: "keys", label: "快捷键", icon: KeyboardIcon },
] as const;

export function HelpDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex h-[min(640px,calc(100vh-6rem))] flex-col gap-0 p-0 sm:max-w-3xl">
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
          <TabsList className="h-full! w-44 shrink-0 flex-col items-stretch justify-start gap-0.5 rounded-none border-r bg-muted/40 p-2">
            {tabs.map(({ value, label, icon: Icon }) => (
              <TabsTrigger
                key={value}
                value={value}
                className="h-9 flex-none justify-start gap-2 px-3 data-active:bg-card"
              >
                <Icon className="size-4" />
                {label}
              </TabsTrigger>
            ))}
          </TabsList>
          <ScrollArea className="min-h-0 flex-1">
            <div className="px-6 py-5">
              <TabsContent value="start" className="anim-fade">
                <Start />
              </TabsContent>
              <TabsContent value="shelf" className="anim-fade">
                <ShelfHelp />
              </TabsContent>
              <TabsContent value="discover" className="anim-fade">
                <DiscoverHelp />
              </TabsContent>
              <TabsContent value="installed" className="anim-fade">
                <InstalledHelp />
              </TabsContent>
              <TabsContent value="keys" className="anim-fade">
                <Keys />
              </TabsContent>
            </div>
          </ScrollArea>
        </Tabs>
      </DialogContent>
    </Dialog>
  );
}

function H({ children }: { children: ReactNode }) {
  return <h3 className="mt-6 mb-2.5 text-sm font-semibold first:mt-0">{children}</h3>;
}

function List({ children }: { children: ReactNode }) {
  return (
    <ul className="flex list-disc flex-col gap-1.5 pl-5 text-sm leading-relaxed text-foreground/90 marker:text-primary/60">
      {children}
    </ul>
  );
}

function B({ children }: { children: ReactNode }) {
  return <span className="font-medium text-foreground">{children}</span>;
}

function Steps({ items }: { items: ReactNode[] }) {
  return (
    <ol className="flex flex-col gap-3">
      {items.map((it, i) => (
        <li key={i} className="flex gap-3 text-sm leading-relaxed">
          <span className="mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-full bg-primary/12 text-xs font-semibold text-primary tabular-nums">
            {i + 1}
          </span>
          <span className="text-foreground/90">{it}</span>
        </li>
      ))}
    </ol>
  );
}

function Start() {
  return (
    <>
      <H>五步用起来</H>
      <Steps
        items={[
          <>
            在 <B>发现</B> 输入软件名，winget 和 scoop 一起出结果；都没有时点 <B>搜 GitHub</B>。
          </>,
          <>
            选中结果点 <B>加入清单</B>，顺手选好分类和标签；点 <B>加入并安装</B>{" "}
            会直接进入安装预览。
          </>,
          <>
            在 <B>我的清单</B> 左侧选分类或标签，勾选软件后点 <B>安装</B> 或 <B>更新</B>
            ，任务按顺序一个一个执行。
          </>,
          <>
            <B>本机已装</B>{" "}
            列出这台电脑上已有的程序，有更新时可以直接更新，也可以加入清单以后换电脑装回。
          </>,
          <>
            在清单左下角 <B>导出</B> 清单文件，到另一台电脑 <B>导入</B>，再按分类或标签批量安装。
          </>,
        ]}
      />
      <H>不会误装</H>
      <List>
        <li>每次安装前都会先预览：哪些会装、哪些跳过，以及跳过的原因。</li>
        <li>已经装过的软件不会再装，用别的方式装过的也一样（检测规则命中即算已安装）。</li>
        <li>读不到状态、无法确认时一律跳过，开始前还会再核对一次。</li>
      </List>
    </>
  );
}

function ShelfHelp() {
  return (
    <>
      <H>状态</H>
      <div className="grid grid-cols-[auto_1fr] items-center gap-x-4 gap-y-2.5 text-sm">
        <Pill tone="primary">
          <ArrowUpCircleIcon />
          可更新
        </Pill>
        <span>来源有新版本，可以在清单或本机已装页预览后更新。</span>
        <Pill tone="success">
          <CheckCircle2Icon />
          已安装
        </Pill>
        <span>
          通过这个来源装的，或者检测规则找到了它（显示为「已安装（来源外）」，不会通过 AppShelf
          更新）。
        </span>
        <Pill tone="muted">
          <CircleIcon />
          未安装
        </Pill>
        <span>可以安装。</span>
        <Pill tone="warning">
          <TriangleAlertIcon />
          无法确认
        </Pill>
        <span>读取状态出错，安装和更新都会跳过它，悬停可以看原因。</span>
        <Pill tone="danger">
          <BanIcon />
          不可用
        </Pill>
        <span>本机缺少这个来源，例如没有装 scoop。</span>
      </div>
      <H>筛选与批量</H>
      <List>
        <li>左侧的分类和标签可以多选，显示属于其中任意一个的软件；状态筛选再叠加在上面。</li>
        <li>分类、标签旁的按钮可以重命名和删除；删除分类不会删除软件，它们变为未分类。</li>
        <li>勾选多个软件后，底部会出现批量操作：安装、更新、设分类、加标签、删除。</li>
        <li>删除只把软件移出清单，不会卸载；删除后可以在提示里撤销。</li>
      </List>
      <H>检查更新</H>
      <List>
        <li>
          清单工具条右侧的 <B>检查更新</B> 会重新检查所有来源和软件的状态，再刷新本机已装列表。
        </li>
        <li>
          AppShelf 自己的更新在 <B>设置 → 关于</B> 里检查。
        </li>
      </List>
    </>
  );
}

function DiscoverHelp() {
  return (
    <>
      <H>搜索</H>
      <List>
        <li>
          输入后 winget 和 scoop 同时搜索，结果按名称接近程度排序，标出「已在清单」和「本机已装」。
        </li>
        <li>
          GitHub 搜索没有 token 时每分钟只有 10 次，所以只在点 <B>搜 GitHub</B> 或按{" "}
          <Kbd>Enter</Kbd> 时搜索。
        </li>
        <li>点一条结果在右侧查看详情：简介、主页、许可证、版本和发行说明。</li>
      </List>
      <H>手动添加</H>
      <List>
        <li>
          搜不到时点 <B>手动添加</B>：winget 和 scoop 在弹窗里搜索并选择软件包，GitHub 填写仓库
          owner/name。
        </li>
        <li>
          GitHub 软件可以写安装包匹配规则（例如
          x64\.msi$）和安装参数；点「检查最新版本」确认会装哪个文件。
        </li>
      </List>
      <H>检测规则</H>
      <List>
        <li>用来识别本机已有的软件，避免重复安装；GitHub 软件必须有，winget 和 scoop 软件可选。</li>
        <li>
          <B>名称</B>：「已安装的应用」里的显示名称，正则表达式，不区分大小写，例如{" "}
          <code className="rounded bg-muted px-1 font-mono text-xs">^Visual Studio Code</code>。
        </li>
        <li>
          <B>可执行文件路径</B>：可以使用{" "}
          <code className="rounded bg-muted px-1 font-mono text-xs">%ProgramFiles%</code>{" "}
          这类环境变量；两项任一命中就算已安装。
        </li>
        <li>加入 GitHub 软件时，会根据本机已装的程序和仓库名给出建议规则。</li>
      </List>
    </>
  );
}

function InstalledHelp() {
  return (
    <>
      <H>概览</H>
      <List>
        <li>打开这一页先看概览：程序总数、来源分布、可更新、清单覆盖和无来源的程序。</li>
        <li>
          点概览里的任意一块，进入对应筛选的列表；「全部更新」「全部加入清单」都会先预览再执行。
        </li>
      </List>
      <H>列表从哪来</H>
      <List>
        <li>winget 认识的程序、scoop 装的软件、注册表里的其他程序，以及应用商店应用。</li>
        <li>有 winget 或 scoop 来源且标记「可更新」的程序，可以直接点「更新」，无需先加入清单。</li>
        <li>勾选多个程序后可批量更新；确认预览后逐项执行，更新不会自动将程序加入清单。</li>
        <li>已在清单里的程序会标出来；可以只看还没加入清单的。</li>
      </List>
      <H>加入清单</H>
      <List>
        <li>
          有 winget 或 scoop 来源的程序可以直接 <B>加入清单</B>，多选后批量加入。
        </li>
        <li>
          没有来源的程序点 <B>查找来源</B>
          ：到发现页按名称搜索，加入时自动带上找到它的检测规则，以后不会重复安装。
        </li>
      </List>
    </>
  );
}

function Keys() {
  const rows: Array<[ReactNode, string]> = [
    [
      <>
        <Kbd>Ctrl</Kbd> <Kbd>F</Kbd> 或 <Kbd>/</Kbd>
      </>,
      "聚焦当前页的搜索框",
    ],
    [<Kbd key="esc">Esc</Kbd>, "清空搜索、取消选择或关闭弹窗"],
    [
      <>
        <Kbd>Ctrl</Kbd> <Kbd>A</Kbd>
      </>,
      "选中列表里显示的全部软件",
    ],
    [
      <>
        <Kbd>Shift</Kbd> + 点击
      </>,
      "连续选中一段",
    ],
    [
      <>
        <Kbd>Ctrl</Kbd> + 点击
      </>,
      "加选或取消选中一项",
    ],
    [<Kbd key="up">↑ ↓</Kbd>, "在列表里移动"],
    [<Kbd key="space">空格</Kbd>, "勾选或取消勾选当前项"],
    [<Kbd key="enter">Enter</Kbd>, "发现页：同时搜索 GitHub"],
    ["右键", "列表项的更多操作"],
  ];
  return (
    <div className="flex flex-col divide-y rounded-xl border bg-card">
      {rows.map(([k, v], i) => (
        <div key={i} className="flex items-center gap-4 px-4 py-2.5 text-sm">
          <span className="flex w-40 shrink-0 items-center gap-1 text-muted-foreground">{k}</span>
          <span>{v}</span>
        </div>
      ))}
    </div>
  );
}
