// Editing an app of the catalog, or adding one by hand: its name, category
// and tags, its exact source, and the rule that finds it installed.

import { AppIcon } from "@/components/app-icon";
import {
  CategoryPicker,
  TagPicker,
  type CategoryChoice,
  type TagChoice,
} from "@/components/pickers";
import { RuleSuggestions } from "@/components/rule-suggestions";
import { GitHubMark, SourceBadge, SourceIcon } from "@/components/source-badge";
import { Pill } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "@/components/ui/input-group";
import { ScrollArea } from "@/components/ui/scroll-area";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { errorText, useSources } from "@/lib/api";
import { iconPath, type SourceKind } from "@/lib/shelf";
import { cn } from "@/lib/utils";
import { Shelf, Sources, type App, type SearchHit, type Source } from "@/mygo";
import { useNav } from "@/state/nav";
import {
  AppWindowIcon,
  CheckIcon,
  FileArchiveIcon,
  FolderIcon,
  HashIcon,
  LoaderCircleIcon,
  PackageCheckIcon,
  PencilIcon,
  PlusIcon,
  ScanSearchIcon,
  SearchIcon,
  TerminalIcon,
  Trash2Icon,
  TriangleAlertIcon,
} from "lucide-react";
import { useRef, useState, type ComponentType, type ReactNode } from "react";
import { toast } from "sonner";

export interface EditorRequest {
  app?: App;
  draft?: Partial<App>;
  seq: number;
  open: boolean;
}

interface Fields {
  name: string;
  category: CategoryChoice;
  tags: TagChoice;
  type: SourceKind;
  wingetId: string;
  bucket: string;
  scoopName: string;
  bucketUrl: string;
  repo: string;
  pattern: string;
  args: string;
  ruleName: string;
  ruleExe: string;
}

function fieldsOf(a: Partial<App> | undefined): Fields {
  const s: Partial<Source> = a?.source ?? {};
  const type = (["winget", "scoop", "github"] as const).find((t) => t === s.type) ?? "winget";
  return {
    name: a?.name ?? "",
    category: { id: a?.categoryId ?? "" },
    tags: { ids: a?.tagIds ?? [], newNames: [] },
    type,
    wingetId: s.id ?? "",
    bucket: s.bucket ?? "",
    scoopName: s.name ?? "",
    bucketUrl: s.bucketUrl ?? "",
    repo: s.repo ?? "",
    pattern: s.assetPattern ?? "",
    args: s.installerArgs ?? "",
    ruleName: a?.detect?.displayName ?? "",
    ruleExe: a?.detect?.exePath ?? "",
  };
}

function sourceOf(f: Fields): Source {
  switch (f.type) {
    case "winget":
      return { type: "winget", id: f.wingetId.trim() };
    case "scoop":
      return {
        type: "scoop",
        bucket: f.bucket.trim(),
        name: f.scoopName.trim(),
        ...(f.bucketUrl.trim() ? { bucketUrl: f.bucketUrl.trim() } : {}),
      };
    case "github":
      return {
        type: "github",
        repo: f.repo.trim(),
        ...(f.pattern.trim() ? { assetPattern: f.pattern.trim() } : {}),
        ...(f.args.trim() ? { installerArgs: f.args.trim() } : {}),
      };
  }
}

/** problem is what keeps the fields from being saved, or "". */
function problem(f: Fields): string {
  if (!f.name.trim()) return "请填写软件名称";
  switch (f.type) {
    case "winget":
      if (!f.wingetId.trim()) return "请先搜索，并从结果中选择一个 winget 软件包";
      break;
    case "scoop":
      if (!f.bucket.trim() || !f.scoopName.trim()) return "请先搜索，并从结果中选择一个 scoop 软件";
      break;
    case "github":
      if (!/^[\w.-]+\/[\w.-]+$/.test(f.repo.trim())) return "请填写 GitHub 仓库，格式为 owner/name";
      if (!f.ruleName.trim() && !f.ruleExe.trim())
        return "GitHub 软件需要检测规则：填写名称或可执行文件路径";
      break;
  }
  return "";
}

export function AppEditor({
  request,
  onClose,
}: {
  request: EditorRequest | null;
  onClose: () => void;
}) {
  const nav = useNav();
  const editing = !!request?.app;
  const [f, setF] = useState<Fields>(() => fieldsOf(request?.app ?? request?.draft));
  const [busy, setBusy] = useState(false);
  const [tried, setTried] = useState(false);
  const set = (patch: Partial<Fields>) => setF((x) => ({ ...x, ...patch }));
  const source = sourceOf(f);
  const issue = problem(f);

  const save = async () => {
    setTried(true);
    if (issue) {
      toast.error(issue);
      return;
    }
    if (!request) return;
    setBusy(true);
    try {
      const categoryId =
        "newName" in f.category ? (await Shelf.addCategory(f.category.newName)).id : f.category.id;
      const tagIds = [...f.tags.ids];
      for (const name of f.tags.newNames) tagIds.push((await Shelf.addTag(name)).id);
      const rule = { displayName: f.ruleName.trim(), exePath: f.ruleExe.trim() };
      const app: App = {
        id: request.app?.id ?? "",
        name: f.name.trim(),
        ...(categoryId ? { categoryId } : {}),
        tagIds,
        source,
        ...(rule.displayName || rule.exePath
          ? {
              detect: {
                ...(rule.displayName ? { displayName: rule.displayName } : {}),
                ...(rule.exePath ? { exePath: rule.exePath } : {}),
              },
            }
          : {}),
      };
      const saved = await Shelf.saveApp(app);
      if (editing) toast.success(`已保存「${saved.name}」`);
      else
        toast.success(`已加入清单：${saved.name}`, {
          action: { label: "去清单查看", onClick: () => nav.showInShelf([saved.id]) },
        });
      onClose();
    } catch (err) {
      toast.error("保存失败", { description: errorText(err) });
    } finally {
      setBusy(false);
    }
  };

  const remove = async () => {
    const app = request?.app;
    if (!app) return;
    try {
      const token = await Shelf.deleteApps([app.id]);
      onClose();
      toast.success(`已删除「${app.name}」`, {
        description: "只从清单中删除，不会卸载软件",
        action: {
          label: "撤销",
          onClick: () => {
            Shelf.undoDelete(token).catch((err: unknown) =>
              toast.error("撤销失败", { description: errorText(err) }),
            );
          },
        },
        duration: 8000,
      });
    } catch (err) {
      toast.error("删除失败", { description: errorText(err) });
    }
  };

  return (
    <Dialog open={request?.open ?? false} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="flex max-h-[min(820px,calc(100vh-5rem))] flex-col gap-0 p-0 sm:max-w-2xl">
        <DialogHeader className="border-b px-5 pt-5 pb-4">
          <DialogTitle className="flex items-center gap-2.5 text-lg">
            {editing ? (
              <PencilIcon className="size-5 text-primary" />
            ) : (
              <PlusIcon className="size-5 text-primary" />
            )}
            {editing ? "编辑软件" : "手动添加软件"}
          </DialogTitle>
        </DialogHeader>
        <ScrollArea className="min-h-0 flex-1">
          <div className="flex flex-col gap-5 px-5 py-4">
            <div className="flex items-center gap-3">
              <AppIcon path={iconPath(source)} name={f.name || "?"} size={44} />
              <Input
                value={f.name}
                onChange={(e) => set({ name: e.target.value })}
                placeholder="软件名称"
                aria-label="软件名称"
                aria-invalid={(tried && !f.name.trim()) || undefined}
                className="h-10 text-base font-medium"
                autoFocus={!editing}
              />
            </div>
            <div className="grid grid-cols-[minmax(0,15rem)_minmax(0,1fr)] gap-x-5 gap-y-1.5">
              <Label icon={FolderIcon}>分类</Label>
              <Label icon={HashIcon}>标签</Label>
              <CategoryPicker value={f.category} onChange={(category) => set({ category })} />
              <TagPicker value={f.tags} onChange={(tags) => set({ tags })} className="min-h-8" />
            </div>
            <section className="flex flex-col gap-2.5">
              <div className="flex items-center justify-between gap-3">
                <Label icon={PackageCheckIcon}>安装来源</Label>
                <ToggleGroup
                  type="single"
                  variant="outline"
                  size="sm"
                  value={f.type}
                  onValueChange={(v) => v && set({ type: v as SourceKind })}
                  disabled={editing}
                >
                  {(["winget", "scoop", "github"] as const).map((t) => (
                    <ToggleGroupItem key={t} value={t} className="gap-1.5 px-3">
                      <SourceIcon type={t} />
                      {t === "github" ? "GitHub" : t}
                    </ToggleGroupItem>
                  ))}
                </ToggleGroup>
              </div>
              {f.type === "github" ? (
                <GitHubFields f={f} set={set} tried={tried} />
              ) : (
                <PackageSearch
                  key={f.type}
                  type={f.type}
                  f={f}
                  set={set}
                  locked={editing}
                  tried={tried}
                />
              )}
            </section>
            <RuleFields f={f} set={set} tried={tried} />
          </div>
        </ScrollArea>
        <DialogFooter className="mx-0 mb-0 items-center rounded-b-xl px-5 py-3.5">
          {editing && (
            <Button variant="destructive" className="mr-auto" onClick={remove}>
              <Trash2Icon />
              删除
            </Button>
          )}
          <Button variant="outline" onClick={onClose}>
            取消
          </Button>
          <Button onClick={save} disabled={busy}>
            {busy ? <LoaderCircleIcon className="animate-spin" /> : <CheckIcon />}
            {editing ? "保存" : "加入清单"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function Label({
  icon: Icon,
  children,
  hint,
}: {
  icon: ComponentType<{ className?: string }>;
  children: ReactNode;
  hint?: string;
}) {
  return (
    <div className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
      <Icon className="size-3.5" />
      {children}
      {hint && <span className="font-normal text-muted-foreground/80">{hint}</span>}
    </div>
  );
}

/**
 * PackageSearch finds the exact package of winget or scoop: one is chosen
 * from the results rather than typed.
 */
function PackageSearch({
  type,
  f,
  set,
  locked,
  tried,
}: {
  type: "winget" | "scoop";
  f: Fields;
  set: (p: Partial<Fields>) => void;
  locked: boolean;
  tried: boolean;
}) {
  const { data: sources } = useSources();
  const info = sources?.sources.find((s) => s.type === type);
  const unavailable = info?.checked && !info.ok;
  const [query, setQuery] = useState(f.name);
  const [hits, setHits] = useState<SearchHit[] | null>(null);
  const [searching, setSearching] = useState(false);
  const gen = useRef(0);
  const chosen = type === "winget" ? f.wingetId : f.scoopName ? `${f.bucket}/${f.scoopName}` : "";

  const search = async () => {
    const q = query.trim();
    if (!q) return;
    const my = ++gen.current;
    setSearching(true);
    try {
      const res = await Sources.search(type, q);
      if (my === gen.current) setHits(res);
    } catch (err) {
      if (my === gen.current) {
        setHits(null);
        toast.error(`搜索 ${type} 失败`, { description: errorText(err) });
      }
    } finally {
      if (my === gen.current) setSearching(false);
    }
  };

  const choose = (h: SearchHit) => {
    if (type === "winget") set({ wingetId: h.id });
    else {
      const [bucket, ...rest] = h.id.split("/");
      set({ bucket: bucket ?? "", scoopName: rest.join("/") });
    }
    if (!f.name.trim()) set({ name: h.name });
  };

  return (
    <div className="flex flex-col gap-2.5">
      <div
        className={cn(
          "flex min-h-11 items-center gap-2.5 rounded-lg border px-3 py-2 text-sm",
          chosen ? "border-primary/30 bg-primary/6" : "border-dashed text-muted-foreground",
          tried && !chosen && "border-destructive/60",
        )}
      >
        {chosen ? (
          <>
            <CheckIcon className="size-4 shrink-0 text-primary" />
            <SourceBadge type={type} />
            <span className="truncate font-mono font-medium select-text">{chosen}</span>
          </>
        ) : (
          <>
            <SearchIcon className="size-4 shrink-0" />
            还没有选择软件包
          </>
        )}
      </div>
      {locked ? null : unavailable ? (
        <div className="flex items-start gap-2 rounded-lg bg-warning/10 px-3 py-2.5 text-sm text-warning">
          <TriangleAlertIcon className="mt-0.5 size-4 shrink-0" />
          <span>
            本机无法搜索 {type}：{info?.reason}
          </span>
        </div>
      ) : (
        <>
          <InputGroup className="h-9">
            <InputGroupAddon>
              <SearchIcon />
            </InputGroupAddon>
            <InputGroupInput
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && void search()}
              placeholder={`搜索 ${type}`}
              spellCheck={false}
            />
            <InputGroupAddon align="inline-end">
              <InputGroupButton
                variant="secondary"
                size="xs"
                onClick={() => void search()}
                disabled={searching || !query.trim()}
              >
                {searching && <LoaderCircleIcon className="animate-spin" />}
                搜索
              </InputGroupButton>
            </InputGroupAddon>
          </InputGroup>
          {hits !== null && (
            <div className="anim-rise max-h-64 overflow-y-auto rounded-lg border">
              {hits.length === 0 ? (
                <div className="px-3 py-6 text-center text-sm text-muted-foreground">没有找到</div>
              ) : (
                hits.map((h) => {
                  const on = h.id.toLowerCase() === chosen.toLowerCase();
                  return (
                    <button
                      key={h.id}
                      type="button"
                      onClick={() => choose(h)}
                      className={cn(
                        "flex w-full items-center gap-3 border-b px-3 py-2 text-left transition-colors last:border-b-0 hover:bg-muted/60",
                        on && "bg-primary/8",
                      )}
                    >
                      <AppIcon path={h.icon} name={h.name} size={28} />
                      <div className="min-w-0 flex-1">
                        <div className="truncate text-sm font-medium">{h.name}</div>
                        <div className="truncate font-mono text-xs text-muted-foreground">
                          {h.id}
                        </div>
                      </div>
                      {h.version && (
                        <span className="shrink-0 font-mono text-xs text-muted-foreground">
                          {h.version}
                        </span>
                      )}
                      {h.inShelf && <Pill tone="muted">已在清单</Pill>}
                      {on ? (
                        <CheckIcon className="size-4 shrink-0 text-primary" />
                      ) : (
                        <span className="size-4 shrink-0" />
                      )}
                    </button>
                  );
                })
              )}
            </div>
          )}
        </>
      )}
      {type === "scoop" && (
        <div className="flex flex-col gap-1.5">
          <Label icon={PackageCheckIcon} hint="仅自定义 bucket 需要">
            bucket 地址
          </Label>
          <Input
            value={f.bucketUrl}
            onChange={(e) => set({ bucketUrl: e.target.value })}
            placeholder="https://github.com/…"
            spellCheck={false}
            className="font-mono text-xs"
          />
        </div>
      )}
    </div>
  );
}

function GitHubFields({
  f,
  set,
  tried,
}: {
  f: Fields;
  set: (p: Partial<Fields>) => void;
  tried: boolean;
}) {
  const [checking, setChecking] = useState(false);
  const [found, setFound] = useState<{ tag: string; asset: string; key: string } | null>(null);
  const repo = f.repo.trim();
  const repoOK = /^[\w.-]+\/[\w.-]+$/.test(repo);
  const key = `${repo}\n${f.pattern.trim()}`;
  const check = async () => {
    setChecking(true);
    try {
      const res = await Sources.checkGitHub(repo, f.pattern.trim());
      setFound({ ...res, key });
    } catch (err) {
      setFound(null);
      toast.error("检查失败", { description: errorText(err) });
    } finally {
      setChecking(false);
    }
  };
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-col gap-1.5">
        <Label icon={GitHubMark}>仓库</Label>
        <Input
          value={f.repo}
          onChange={(e) => set({ repo: e.target.value })}
          placeholder="owner/name"
          spellCheck={false}
          aria-invalid={(tried && !repoOK) || undefined}
          className="font-mono"
        />
      </div>
      <div className="grid grid-cols-2 gap-3">
        <div className="flex flex-col gap-1.5">
          <Label icon={FileArchiveIcon} hint="可选，正则表达式">
            安装包匹配
          </Label>
          <Input
            value={f.pattern}
            onChange={(e) => set({ pattern: e.target.value })}
            placeholder="例如 x64\.msi$"
            spellCheck={false}
            className="font-mono text-xs"
          />
        </div>
        <div className="flex flex-col gap-1.5">
          <Label icon={TerminalIcon} hint="可选">
            安装参数
          </Label>
          <Input
            value={f.args}
            onChange={(e) => set({ args: e.target.value })}
            placeholder="留空：.msi 用 /passive，.exe 交互安装"
            spellCheck={false}
            className="font-mono text-xs"
          />
        </div>
      </div>
      <div className="flex min-h-8 items-center gap-2.5">
        <Button variant="secondary" size="sm" onClick={check} disabled={checking || !repoOK}>
          {checking ? <LoaderCircleIcon className="animate-spin" /> : <ScanSearchIcon />}
          检查最新版本
        </Button>
        {found && found.key === key && (
          <Pill tone="success" className="anim-fade max-w-[70%]">
            <FileArchiveIcon />
            <span className="truncate">
              将安装 {found.asset}（{found.tag}）
            </span>
          </Pill>
        )}
      </div>
    </div>
  );
}

function RuleFields({
  f,
  set,
  tried,
}: {
  f: Fields;
  set: (p: Partial<Fields>) => void;
  tried: boolean;
}) {
  const required = f.type === "github";
  const missing = required && tried && !f.ruleName.trim() && !f.ruleExe.trim();
  const repo = f.repo.trim();
  return (
    <section className="flex flex-col gap-2.5 rounded-xl border bg-muted/30 p-3.5">
      <div className="flex items-center gap-2">
        <Label icon={ScanSearchIcon}>检测规则</Label>
        <Pill tone={required ? "warning" : "muted"}>{required ? "必填" : "可选"}</Pill>
        <span className="text-xs text-muted-foreground">任一项命中即视为已安装</span>
      </div>
      <div className="flex flex-col gap-1.5">
        <Label icon={AppWindowIcon} hint="正则表达式，不区分大小写">
          已安装应用里的名称
        </Label>
        <InputGroup className="h-8 bg-background">
          <InputGroupInput
            value={f.ruleName}
            onChange={(e) => set({ ruleName: e.target.value })}
            placeholder="例如 ^Visual Studio Code"
            spellCheck={false}
            aria-invalid={missing || undefined}
            className="font-mono text-xs"
          />
          {f.type === "github" && /^[\w.-]+\/[\w.-]+$/.test(repo) && (
            <InputGroupAddon align="inline-end">
              <RuleSuggestions repo={repo} onPick={(rule) => set({ ruleName: rule })} />
            </InputGroupAddon>
          )}
        </InputGroup>
      </div>
      <div className="flex flex-col gap-1.5">
        <Label icon={TerminalIcon} hint="可使用 %ProgramFiles% 这类环境变量">
          可执行文件路径
        </Label>
        <Input
          value={f.ruleExe}
          onChange={(e) => set({ ruleExe: e.target.value })}
          placeholder="例如 %LOCALAPPDATA%\Programs\App\App.exe"
          spellCheck={false}
          aria-invalid={missing || undefined}
          className="bg-background font-mono text-xs"
        />
      </div>
    </section>
  );
}
