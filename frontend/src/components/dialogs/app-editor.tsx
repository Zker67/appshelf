// Editing an app of the catalog, or adding one by hand. Adding starts from
// one box: a name searches winget and scoop, a GitHub link is the
// repository; one result is chosen, then named and filed. What a beginner
// need not touch (the installer's pattern and arguments, the detection
// rule) waits under 高级设置, filled in where it can be.

import { AppIcon } from "@/components/app-icon";
import {
  CategoryPicker,
  TagPicker,
  type CategoryChoice,
  type TagChoice,
} from "@/components/pickers";
import {
  RuleSuggestions,
  suggestionLabel,
  useRuleSuggestions,
} from "@/components/rule-suggestions";
import { GitHubMark, SourceBadge } from "@/components/source-badge";
import { Pill } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
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
import { Skeleton } from "@/components/ui/skeleton";
import { errorText } from "@/lib/api";
import { iconPath, sourceID, type SourceKind } from "@/lib/shelf";
import { fold } from "@/lib/text";
import { cn } from "@/lib/utils";
import {
  Discover,
  Shelf,
  Sources,
  type App,
  type SearchBatch,
  type SearchHit,
  type Source,
} from "@/mygo";
import { useNav } from "@/state/nav";
import { useQuery } from "@tanstack/react-query";
import {
  AppWindowIcon,
  CheckIcon,
  ChevronDownIcon,
  FileArchiveIcon,
  FolderIcon,
  HashIcon,
  LoaderCircleIcon,
  MonitorCheckIcon,
  PackageCheckIcon,
  PencilIcon,
  PlusIcon,
  RefreshCwIcon,
  SearchIcon,
  SearchXIcon,
  SlidersHorizontalIcon,
  TerminalIcon,
  Trash2Icon,
  TriangleAlertIcon,
} from "lucide-react";
import { Channel } from "mygo-runtime";
import { useEffect, useRef, useState, type ComponentType, type ReactNode } from "react";
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

type SetFields = (p: Partial<Fields>) => void;

const repoShape = /^[\w.-]+\/[\w.-]+$/;

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

/** chosenOf: whether the fields name an exact package. */
function chosenOf(f: Fields): boolean {
  switch (f.type) {
    case "winget":
      return !!f.wingetId.trim();
    case "scoop":
      return !!f.bucket.trim() && !!f.scoopName.trim();
    case "github":
      return repoShape.test(f.repo.trim());
  }
}

/**
 * githubRepo reads a repository from what was typed: a link to it, or
 * owner/name. A link may go deeper (releases, a file); only the repository
 * counts.
 */
function githubRepo(text: string): string | null {
  const t = text.trim();
  const link =
    /^(?:https?:\/\/)?(?:www\.)?github\.com\/([\w.-]+)\/([\w.-]+?)(?:\.git)?(?:[/?#].*)?$/i.exec(t);
  if (link) return `${link[1]}/${link[2]}`;
  return repoShape.test(t) ? t : null;
}

/** searchable: two letters at least, or a Chinese character. */
function searchable(q: string): boolean {
  return fold(q).length >= 2 || /\p{Script=Han}/u.test(q);
}

/** problem is what keeps the fields from being saved, or "". */
function problem(f: Fields): string {
  if (!chosenOf(f)) return "请先搜索并选择一个软件";
  if (!f.name.trim()) return "请填写软件名称";
  if (f.type === "github" && !f.ruleName.trim() && !f.ruleExe.trim())
    return "GitHub 软件需要检测规则，请在高级设置里填写";
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
  const [picking, setPicking] = useState(() => !chosenOf(f));
  const [advanced, setAdvanced] = useState(false);
  const [busy, setBusy] = useState(false);
  const [tried, setTried] = useState(false);
  /** ruleTouched: the user wrote the rule, suggestions no longer replace it. */
  const [ruleTouched, setRuleTouched] = useState(editing);
  const set: SetFields = (patch) => setF((x) => ({ ...x, ...patch }));
  const source = sourceOf(f);
  const chosen = chosenOf(f);
  const issue = problem(f);
  // winget and scoop packages keep their source when edited; a GitHub app
  // may move to another repository.
  const locked = editing && f.type !== "github";

  const repo = f.type === "github" ? f.repo.trim() : "";
  const suggestions = useRuleSuggestions(repo);
  const first = suggestions.data?.[0];
  useEffect(() => {
    if (first && !ruleTouched && !f.ruleName.trim()) set({ ruleName: first.rule });
    // Only a new first suggestion fills the rule.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [first?.rule, repo]);
  const found = suggestions.data?.find((s) => s.from && s.rule === f.ruleName.trim());

  const pick = (p: Picked) => {
    if (editing) set({ repo: p.repo ?? f.repo });
    else
      set({
        name: p.name,
        type: p.type,
        wingetId: p.wingetId ?? "",
        bucket: p.bucket ?? "",
        scoopName: p.scoopName ?? "",
        bucketUrl: "",
        repo: p.repo ?? "",
        pattern: "",
        args: "",
        ruleName: "",
        ruleExe: "",
      });
    if (!editing) setRuleTouched(false);
    setPicking(false);
  };

  const save = async () => {
    setTried(true);
    if (issue) {
      if (f.type === "github" && chosen && f.name.trim()) setAdvanced(true);
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

  const showPicker = picking || !chosen;

  return (
    <Dialog open={request?.open ?? false} onOpenChange={(o) => !o && onClose()}>
      <DialogContent
        className={cn(
          "flex flex-col gap-0 p-0 sm:max-w-2xl",
          // Results come and go while searching: the picker keeps one height.
          showPicker ? "h-[min(640px,calc(100vh-5rem))]" : "max-h-[min(720px,calc(100vh-5rem))]",
        )}
      >
        <DialogHeader className="border-b px-5 pt-5 pb-4">
          <DialogTitle className="flex items-center gap-2.5 text-lg">
            {editing ? (
              <PencilIcon className="size-5 text-primary" />
            ) : (
              <PlusIcon className="size-5 text-primary" />
            )}
            {editing ? "编辑软件" : "手动添加软件"}
          </DialogTitle>
          <DialogDescription>
            {editing
              ? "修改名称、分类和标签；安装细节在高级设置里"
              : "搜索软件名，或粘贴 GitHub 仓库链接，选中后即可加入清单"}
          </DialogDescription>
        </DialogHeader>
        <ScrollArea
          className="flex min-h-0 flex-1 flex-col"
          viewportClassName="h-auto min-h-0 flex-1"
        >
          <div className="flex flex-col gap-5 px-5 py-4">
            {showPicker ? (
              <Picker
                initial={editing ? f.repo : f.name}
                githubOnly={editing}
                canCancel={chosen}
                onCancel={() => setPicking(false)}
                onPick={pick}
              />
            ) : (
              <>
                <ChosenCard
                  f={f}
                  set={set}
                  source={source}
                  tried={tried}
                  found={found ? suggestionLabel(found) : undefined}
                  onChange={locked ? undefined : () => setPicking(true)}
                />
                <div className="anim-rise grid grid-cols-[minmax(0,15rem)_minmax(0,1fr)] gap-x-5 gap-y-1.5">
                  <Label icon={FolderIcon}>分类</Label>
                  <Label icon={HashIcon}>标签</Label>
                  <CategoryPicker value={f.category} onChange={(category) => set({ category })} />
                  <TagPicker
                    value={f.tags}
                    onChange={(tags) => set({ tags })}
                    className="min-h-8"
                  />
                </div>
                <Advanced
                  f={f}
                  set={set}
                  open={advanced}
                  onOpenChange={setAdvanced}
                  tried={tried}
                  onRuleTyped={() => setRuleTouched(true)}
                />
              </>
            )}
          </div>
        </ScrollArea>
        <DialogFooter className="mx-0 mb-0 items-center rounded-b-xl px-5 py-3.5">
          {editing && (
            <Button variant="destructive" className="mr-auto" onClick={remove}>
              <Trash2Icon />
              删除
            </Button>
          )}
          {!editing && !chosen && (
            <span className="mr-auto text-xs text-muted-foreground">先从搜索结果里选一个软件</span>
          )}
          <Button variant="outline" onClick={onClose}>
            取消
          </Button>
          <Button onClick={save} disabled={busy || !chosen || showPicker}>
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

/** Picked is a package chosen in the picker. */
interface Picked {
  name: string;
  type: SourceKind;
  wingetId?: string;
  bucket?: string;
  scoopName?: string;
  repo?: string;
}

function pickedOf(h: SearchHit): Picked {
  switch (h.type) {
    case "scoop": {
      const [bucket, ...rest] = h.id.split("/");
      return { name: h.name, type: "scoop", bucket: bucket ?? "", scoopName: rest.join("/") };
    }
    case "github":
      return { name: h.name, type: "github", repo: h.id };
    default:
      return { name: h.name, type: "winget", wingetId: h.id };
  }
}

function pickedRepo(repo: string): Picked {
  return { name: repo.split("/")[1] ?? repo, type: "github", repo };
}

interface PickerState {
  query: string;
  state: "idle" | "loading" | "done";
  hits: SearchHit[];
  /** notes are the sources that could not search, and why. */
  notes: string[];
  github: "idle" | "loading" | "done";
}

/**
 * Picker is the one box adding starts from: a name searches winget and
 * scoop as it is typed, a GitHub link or owner/name is offered as is, and
 * GitHub is searched when asked.
 */
function Picker({
  initial,
  githubOnly,
  canCancel,
  onCancel,
  onPick,
}: {
  initial: string;
  githubOnly: boolean;
  canCancel: boolean;
  onCancel: () => void;
  onPick: (p: Picked) => void;
}) {
  const [query, setQuery] = useState(initial);
  const [res, setRes] = useState<PickerState>({
    query: "",
    state: "idle",
    hits: [],
    notes: [],
    github: "idle",
  });
  const gen = useRef(0);
  const repo = githubRepo(query);
  const isLink = isLinkText(query);

  const search = (q: string) => {
    const my = ++gen.current;
    const text = q.trim();
    if (githubOnly || isLinkText(text) || !searchable(text)) {
      setRes({ query: text, state: "idle", hits: [], notes: [], github: "idle" });
      return;
    }
    setRes({ query: text, state: "loading", hits: [], notes: [], github: "idle" });
    const channel = new Channel<SearchBatch>((batch) => {
      if (my !== gen.current) return;
      setRes((r) => ({
        ...r,
        hits: [...r.hits.filter((h) => h.type !== batch.type), ...batch.hits],
        notes: batch.error ? [...r.notes, `${batch.type}：${batch.error}`] : r.notes,
      }));
    });
    Discover.search(text, channel)
      .catch((err: unknown) => {
        if (my === gen.current) toast.error("搜索失败", { description: errorText(err) });
      })
      .finally(() => {
        if (my === gen.current) setRes((r) => ({ ...r, state: "done" }));
      });
  };

  const searchGitHub = () => {
    const my = gen.current;
    const text = res.query;
    setRes((r) => ({ ...r, github: "loading" }));
    Discover.searchGitHub(text)
      .then((out) => {
        if (my !== gen.current) return;
        setRes((r) => ({
          ...r,
          github: "done",
          hits: [...r.hits.filter((h) => h.type !== "github"), ...out.hits],
        }));
      })
      .catch((err: unknown) => {
        if (my !== gen.current) return;
        setRes((r) => ({ ...r, github: "idle" }));
        toast.error("搜索 GitHub 失败", { description: errorText(err) });
      });
  };

  // Typing searches after a pause; the first search is at once.
  const first = useRef(true);
  useEffect(() => {
    if (first.current) {
      first.current = false;
      search(query);
      return;
    }
    const t = window.setTimeout(() => search(query), 350);
    return () => window.clearTimeout(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [query]);

  const searching = res.state === "loading";

  return (
    <div className="flex flex-col gap-3">
      <InputGroup className="h-11 rounded-xl bg-background shadow-xs">
        <InputGroupAddon className="pl-3.5">
          {searching ? (
            <LoaderCircleIcon className="size-4.5 animate-spin text-primary" />
          ) : repo ? (
            <GitHubMark className="size-4.5" />
          ) : (
            <SearchIcon className="size-4.5" />
          )}
        </InputGroupAddon>
        <InputGroupInput
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && repo) onPick(pickedRepo(repo));
          }}
          placeholder={
            githubOnly ? "粘贴 GitHub 仓库链接，或输入 owner/name" : "软件名，或 GitHub 仓库链接"
          }
          aria-label="搜索软件"
          spellCheck={false}
          autoFocus
          className="text-base"
        />
        {canCancel && (
          <InputGroupAddon align="inline-end">
            <InputGroupButton size="xs" onClick={onCancel}>
              取消更换
            </InputGroupButton>
          </InputGroupAddon>
        )}
      </InputGroup>

      {repo && (
        <button
          type="button"
          onClick={() => onPick(pickedRepo(repo))}
          className="anim-rise group flex items-center gap-3 rounded-xl border border-primary/30 bg-primary/[0.06] px-3.5 py-3 text-left transition-colors hover:border-primary/50 hover:bg-primary/10"
        >
          <AppIcon
            path={iconPath({ type: "github", repo })}
            name={repo.split("/")[1] ?? repo}
            size={36}
          />
          <div className="min-w-0 flex-1">
            <div className="truncate text-sm font-medium">使用 GitHub 仓库</div>
            <div className="truncate font-mono text-xs text-muted-foreground">{repo}</div>
          </div>
          <span className="flex shrink-0 items-center gap-1 text-xs font-medium text-primary">
            选择
            <CheckIcon className="size-3.5 transition-transform group-hover:scale-110" />
          </span>
        </button>
      )}

      {githubOnly || isLink ? (
        !repo && (
          <Hint icon={GitHubMark}>
            例如 <span className="font-mono">https://github.com/microsoft/PowerToys</span>
          </Hint>
        )
      ) : !searchable(query) ? (
        !repo && (
          <div className="flex flex-col items-center gap-2 py-8 text-center">
            <div className="flex size-12 items-center justify-center rounded-2xl bg-primary/10 text-primary">
              <SearchIcon className="size-6" />
            </div>
            <div className="text-sm font-medium">输入软件名开始搜索</div>
            <div className="text-xs text-muted-foreground">
              会同时搜索 winget 和 scoop；也可以直接粘贴 GitHub 仓库链接
            </div>
          </div>
        )
      ) : (
        <Results res={res} onPick={(h) => onPick(pickedOf(h))} onSearchGitHub={searchGitHub} />
      )}
    </div>
  );
}

function isLinkText(text: string): boolean {
  return /github\.com/i.test(text) || /^https?:\/\//i.test(text);
}

function Hint({
  icon: Icon,
  children,
}: {
  icon: ComponentType<{ className?: string }>;
  children: ReactNode;
}) {
  return (
    <div className="flex items-center gap-2 px-1 text-xs text-muted-foreground">
      <Icon className="size-3.5 shrink-0" />
      <span className="min-w-0">{children}</span>
    </div>
  );
}

function Results({
  res,
  onPick,
  onSearchGitHub,
}: {
  res: PickerState;
  onPick: (h: SearchHit) => void;
  onSearchGitHub: () => void;
}) {
  const searching = res.state === "loading";
  const none = res.hits.length === 0;
  return (
    <div className="flex flex-col gap-2">
      {none && searching ? (
        <div className="flex flex-col gap-1.5">
          {Array.from({ length: 4 }, (_, i) => (
            <div
              key={i}
              className="flex h-14 items-center gap-3 rounded-xl border border-border/60 px-3"
              style={{ opacity: 1 - i * 0.2 }}
            >
              <Skeleton className="size-8 rounded-[26%]" />
              <div className="flex flex-1 flex-col gap-1.5">
                <Skeleton className="h-3 w-40" />
                <Skeleton className="h-2.5 w-64" />
              </div>
            </div>
          ))}
        </div>
      ) : none ? (
        <div className="flex flex-col items-center gap-2 py-6 text-center">
          <SearchXIcon className="size-7 text-muted-foreground" />
          <div className="text-sm font-medium">winget 和 scoop 里没有找到「{res.query}」</div>
          <div className="text-xs text-muted-foreground">
            换个叫法再试，或在 GitHub 上搜索，也可以直接粘贴仓库链接
          </div>
        </div>
      ) : (
        <div className="list-stagger flex flex-col gap-1.5">
          {res.hits.map((h) => (
            <HitOption key={`${h.type}:${h.id}`} hit={h} onPick={() => onPick(h)} />
          ))}
        </div>
      )}
      {!searching && res.github !== "done" && (
        <div className="flex justify-center">
          <Button
            variant="outline"
            size="sm"
            className="rounded-full"
            disabled={res.github === "loading"}
            onClick={onSearchGitHub}
          >
            {res.github === "loading" ? (
              <LoaderCircleIcon className="animate-spin" />
            ) : (
              <GitHubMark className="size-3.5" />
            )}
            {none ? "在 GitHub 上搜索" : "没有想要的？在 GitHub 上搜索"}
          </Button>
        </div>
      )}
      {res.notes.length > 0 && <Hint icon={TriangleAlertIcon}>{res.notes.join("；")}</Hint>}
    </div>
  );
}

function HitOption({ hit: h, onPick }: { hit: SearchHit; onPick: () => void }) {
  const inShelf = !!h.inShelf;
  return (
    <button
      type="button"
      disabled={inShelf}
      onClick={onPick}
      className={cn(
        "group flex items-center gap-3 rounded-xl border border-border/70 bg-card/60 px-3 py-2.5 text-left transition-all",
        inShelf
          ? "cursor-default opacity-60"
          : "hover:-translate-y-px hover:border-primary/40 hover:bg-primary/[0.05] hover:shadow-sm",
      )}
    >
      <AppIcon path={h.icon} name={h.name} size={32} />
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 items-baseline gap-2">
          <span className="truncate text-sm font-medium">{h.name}</span>
          {h.version && (
            <span className="shrink-0 font-mono text-xs text-muted-foreground">{h.version}</span>
          )}
        </div>
        <div className="mt-0.5 flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground">
          <SourceBadge type={h.type} />
          <span className="truncate font-mono">{h.id}</span>
        </div>
      </div>
      {h.installed && (
        <Pill tone="success">
          <MonitorCheckIcon />
          本机已装
        </Pill>
      )}
      {inShelf ? (
        <Pill tone="muted">已在清单</Pill>
      ) : (
        <span className="shrink-0 text-xs font-medium text-primary opacity-0 transition-opacity group-hover:opacity-100">
          选择
        </span>
      )}
    </button>
  );
}

/** ChosenCard is the package chosen: its icon, a name to edit, its source. */
function ChosenCard({
  f,
  set,
  source,
  tried,
  found,
  onChange,
}: {
  f: Fields;
  set: SetFields;
  source: Source;
  tried: boolean;
  found?: string;
  onChange?: () => void;
}) {
  return (
    <div className="anim-rise relative overflow-hidden rounded-2xl border bg-gradient-to-br from-primary/[0.07] via-card to-card p-4 shadow-xs">
      <div className="flex items-start gap-3.5">
        <AppIcon path={iconPath(source)} name={f.name || "?"} size={52} />
        <div className="min-w-0 flex-1">
          <input
            value={f.name}
            onChange={(e) => set({ name: e.target.value })}
            placeholder="软件名称"
            aria-label="软件名称"
            aria-invalid={(tried && !f.name.trim()) || undefined}
            spellCheck={false}
            className="-mx-1.5 w-[calc(100%+0.75rem)] rounded-md bg-transparent px-1.5 py-0.5 text-base font-semibold outline-none transition-colors hover:bg-background/70 focus:bg-background focus:ring-2 focus:ring-ring/40 aria-invalid:ring-2 aria-invalid:ring-destructive/40"
          />
          <div className="mt-1 flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground">
            <SourceBadge type={source.type} />
            <span className="truncate font-mono select-text">{sourceID(source)}</span>
          </div>
        </div>
        {onChange && (
          <Button
            variant="ghost"
            size="sm"
            className="shrink-0 text-muted-foreground"
            onClick={onChange}
          >
            <RefreshCwIcon />
            更换
          </Button>
        )}
      </div>
      {(f.type === "github" || found) && (
        <div className="mt-3 flex flex-wrap items-center gap-1.5 border-t border-dashed pt-3">
          {f.type === "github" && <LatestRelease repo={f.repo.trim()} pattern={f.pattern.trim()} />}
          {found && (
            <Pill tone="success" className="max-w-full">
              <MonitorCheckIcon />
              <span className="truncate">本机已装：{found}</span>
            </Pill>
          )}
        </div>
      )}
    </div>
  );
}

function useDebounced<T>(value: T, ms: number): T {
  const [v, setV] = useState(value);
  useEffect(() => {
    const t = window.setTimeout(() => setV(value), ms);
    return () => window.clearTimeout(t);
  }, [value, ms]);
  return v;
}

/** LatestRelease reads, on its own, what installing a GitHub app would fetch. */
function LatestRelease({ repo, pattern }: { repo: string; pattern: string }) {
  const p = useDebounced(pattern, 500);
  const q = useQuery({
    queryKey: ["github-check", repo.toLowerCase(), p],
    queryFn: () => Sources.checkGitHub(repo, p),
    enabled: repoShape.test(repo),
    retry: false,
    staleTime: 60_000,
  });
  if (q.isPending)
    return (
      <Pill tone="muted">
        <LoaderCircleIcon className="animate-spin" />
        正在读取最新版本
      </Pill>
    );
  if (q.isError)
    return (
      <Pill tone="warning" className="max-w-full">
        <TriangleAlertIcon />
        <span className="truncate">没找到可安装的文件：{errorText(q.error)}</span>
      </Pill>
    );
  return (
    <Pill tone="primary" className="max-w-full">
      <FileArchiveIcon />
      <span className="truncate">
        最新 {q.data.tag} · 将安装 {q.data.asset}
      </span>
    </Pill>
  );
}

/** Advanced holds what most never touch, folded away. */
function Advanced({
  f,
  set,
  open,
  onOpenChange,
  tried,
  onRuleTyped,
}: {
  f: Fields;
  set: SetFields;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  tried: boolean;
  onRuleTyped: () => void;
}) {
  const github = f.type === "github";
  const missing = github && tried && !f.ruleName.trim() && !f.ruleExe.trim();
  const summary = github
    ? f.ruleName.trim() || f.ruleExe.trim()
      ? "检测规则已自动填好"
      : "需要检测规则"
    : "通常不用填";
  return (
    <section
      className={cn(
        "rounded-xl border transition-colors",
        open ? "bg-muted/30" : "bg-transparent",
        missing && "border-destructive/50",
      )}
    >
      <button
        type="button"
        onClick={() => onOpenChange(!open)}
        aria-expanded={open}
        className="flex w-full items-center gap-2 rounded-xl px-3.5 py-2.5 text-left text-sm transition-colors hover:bg-muted/50"
      >
        <SlidersHorizontalIcon className="size-4 text-muted-foreground" />
        <span className="font-medium">高级设置</span>
        <span className={cn("text-xs", missing ? "text-destructive" : "text-muted-foreground")}>
          {summary}
        </span>
        <ChevronDownIcon
          className={cn(
            "ml-auto size-4 text-muted-foreground transition-transform duration-200",
            open && "rotate-180",
          )}
        />
      </button>
      {open && (
        <div className="anim-rise flex flex-col gap-3.5 border-t px-3.5 pt-3 pb-3.5">
          {github && (
            <div className="grid grid-cols-2 gap-3">
              <Field icon={FileArchiveIcon} label="安装包匹配" hint="可选，正则表达式">
                <Input
                  value={f.pattern}
                  onChange={(e) => set({ pattern: e.target.value })}
                  placeholder="例如 x64\.msi$"
                  spellCheck={false}
                  className="bg-background font-mono text-xs"
                />
              </Field>
              <Field icon={TerminalIcon} label="安装参数" hint="可选">
                <Input
                  value={f.args}
                  onChange={(e) => set({ args: e.target.value })}
                  placeholder="留空：.msi 用 /passive"
                  spellCheck={false}
                  className="bg-background font-mono text-xs"
                />
              </Field>
            </div>
          )}
          {f.type === "scoop" && (
            <Field icon={PackageCheckIcon} label="bucket 地址" hint="仅自定义 bucket 需要">
              <Input
                value={f.bucketUrl}
                onChange={(e) => set({ bucketUrl: e.target.value })}
                placeholder="https://github.com/…"
                spellCheck={false}
                className="bg-background font-mono text-xs"
              />
            </Field>
          )}
          <div className="flex flex-col gap-1">
            <div className="flex items-center gap-2 text-xs font-medium">
              检测规则
              <Pill tone={github ? "warning" : "muted"}>{github ? "必填" : "可选"}</Pill>
            </div>
            <p className="text-xs text-muted-foreground">
              {github
                ? "用来判断软件是否已经装在本机，下面两项填一项即可，已按仓库自动推荐。"
                : `用来认出不是通过 ${f.type} 安装的同一个软件，一般留空。`}
            </p>
          </div>
          <Field icon={AppWindowIcon} label="已安装应用里的名称" hint="正则表达式，不区分大小写">
            <InputGroup className="h-8 bg-background">
              <InputGroupInput
                value={f.ruleName}
                onChange={(e) => {
                  onRuleTyped();
                  set({ ruleName: e.target.value });
                }}
                placeholder="例如 ^Visual Studio Code"
                spellCheck={false}
                aria-invalid={missing || undefined}
                className="font-mono text-xs"
              />
              {github && repoShape.test(f.repo.trim()) && (
                <InputGroupAddon align="inline-end">
                  <RuleSuggestions
                    repo={f.repo.trim()}
                    onPick={(rule) => {
                      onRuleTyped();
                      set({ ruleName: rule });
                    }}
                  />
                </InputGroupAddon>
              )}
            </InputGroup>
          </Field>
          <Field
            icon={TerminalIcon}
            label="可执行文件路径"
            hint="可使用 %ProgramFiles% 这类环境变量"
          >
            <Input
              value={f.ruleExe}
              onChange={(e) => set({ ruleExe: e.target.value })}
              placeholder="例如 %LOCALAPPDATA%\Programs\App\App.exe"
              spellCheck={false}
              aria-invalid={missing || undefined}
              className="bg-background font-mono text-xs"
            />
          </Field>
        </div>
      )}
    </section>
  );
}

function Field({
  icon,
  label,
  hint,
  children,
}: {
  icon: ComponentType<{ className?: string }>;
  label: string;
  hint?: string;
  children: ReactNode;
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <Label icon={icon} hint={hint}>
        {label}
      </Label>
      {children}
    </div>
  );
}
