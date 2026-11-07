// Adding packages found by searching, or programs installed, to the
// catalog: their names, a category and tags, and for GitHub apps the rule
// that finds them installed.

import { AppIcon } from "@/components/app-icon";
import {
  CategoryPicker,
  TagPicker,
  noCategory,
  noTags,
  type CategoryChoice,
  type TagChoice,
} from "@/components/pickers";
import { SourceBadge } from "@/components/source-badge";
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
import {
  RuleSuggestions,
  suggestionLabel,
  useRuleSuggestions,
} from "@/components/rule-suggestions";
import { InputGroup, InputGroupAddon, InputGroupInput } from "@/components/ui/input-group";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { errorText } from "@/lib/api";
import { sourceID } from "@/lib/shelf";
import { Shelf, type AddItem, type Detect, type Source } from "@/mygo";
import { useNav } from "@/state/nav";
import {
  DownloadIcon,
  FolderIcon,
  HashIcon,
  LoaderCircleIcon,
  MonitorCheckIcon,
  PlusIcon,
  ScanSearchIcon,
  XIcon,
} from "lucide-react";
import { useEffect, useState } from "react";
import { toast } from "sonner";

/** AddCandidate is a package to add, as a search or the installed list has it. */
export interface AddCandidate {
  name: string;
  source: Source;
  icon: string;
  /** detect is a rule known already: of the installed program this is the source of. */
  detect?: Detect;
  /** from is the name of the installed program the rule finds. */
  from?: string;
}

export interface AddDialogRequest {
  items: AddCandidate[];
  install: boolean;
  seq: number;
  open: boolean;
}

interface Draft {
  key: string;
  cand: AddCandidate;
  name: string;
  rule: string;
  /** touched: the user wrote the rule, suggestions no longer replace it. */
  touched: boolean;
}

const needsRule = (d: Draft) => d.cand.source.type === "github";
const showsRule = (d: Draft) => needsRule(d) || !!d.cand.detect?.displayName;

function draftsOf(items: AddCandidate[]): Draft[] {
  return items.map((c, i) => ({
    key: `${i}:${c.source.type}:${sourceID(c.source)}`,
    cand: c,
    name: c.name,
    rule: c.detect?.displayName ?? "",
    touched: !!c.detect?.displayName,
  }));
}

function detectOf(d: Draft): Detect | undefined {
  const rule = d.rule.trim();
  const exe = d.cand.detect?.exePath;
  if (!rule && !exe) return undefined;
  return { ...(rule ? { displayName: rule } : {}), ...(exe ? { exePath: exe } : {}) };
}

export function AddDialog({
  request,
  onClose,
  onInstall,
}: {
  request: AddDialogRequest | null;
  onClose: () => void;
  onInstall: (ids: string[]) => void;
}) {
  const nav = useNav();
  const [drafts, setDrafts] = useState<Draft[]>(() => draftsOf(request?.items ?? []));
  const [category, setCategory] = useState<CategoryChoice>(noCategory);
  const [tags, setTags] = useState<TagChoice>(noTags);
  const [busy, setBusy] = useState<"add" | "install" | null>(null);
  const open = request?.open ?? false;
  const install = request?.install ?? false;
  const missing = drafts.filter((d) => needsRule(d) && !d.rule.trim()).length;

  const update = (key: string, patch: Partial<Draft>) =>
    setDrafts((ds) => ds.map((d) => (d.key === key ? { ...d, ...patch } : d)));
  const remove = (key: string) => setDrafts((ds) => ds.filter((d) => d.key !== key));

  const submit = async (andInstall: boolean) => {
    setBusy(andInstall ? "install" : "add");
    try {
      const items: AddItem[] = drafts.map((d) => ({
        name: d.name.trim() || d.cand.name,
        source: d.cand.source,
        detect: detectOf(d),
      }));
      const res = await Shelf.addPackages({
        items,
        ...("newName" in category
          ? { newCategory: category.newName }
          : { categoryId: category.id }),
        tagIds: tags.ids,
        newTags: tags.newNames,
      });
      if (res.failed.length > 0) {
        toast.error(res.added.length > 0 ? `${res.failed.length} 个软件没有加入` : "没有加入清单", {
          description: res.failed.map((f) => `${f.name}：${f.error}`).join("\n"),
          duration: 10000,
        });
      }
      if (res.added.length > 0) {
        const added = res.added;
        toast.success(`已加入清单：${added.length} 个软件`, {
          action: { label: "去清单查看", onClick: () => nav.showInShelf(added) },
        });
      }
      if (andInstall && res.added.length > 0) {
        onClose();
        onInstall(res.added);
      } else if (res.failed.length === 0) {
        onClose();
      } else {
        // What could not be added stays, to fix it.
        const failed = new Set(res.failed.map((f) => f.name));
        setDrafts((ds) => ds.filter((d) => failed.has(d.name.trim() || d.cand.name)));
      }
    } catch (err) {
      toast.error("加入清单失败", { description: errorText(err) });
    } finally {
      setBusy(null);
    }
  };

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="flex max-h-[min(760px,calc(100vh-6rem))] flex-col gap-0 p-0 sm:max-w-2xl">
        <DialogHeader className="border-b px-5 pt-5 pb-4">
          <DialogTitle className="flex items-center gap-2 text-lg">
            <PlusIcon className="size-5 text-primary" />
            加入清单
          </DialogTitle>
          <DialogDescription className="flex items-center gap-1.5 pt-1">
            <Pill tone="primary">
              <HashIcon />
              {drafts.length} 个软件
            </Pill>
            {missing > 0 && (
              <Pill tone="warning">
                <ScanSearchIcon />
                {missing} 个缺少检测规则
              </Pill>
            )}
          </DialogDescription>
        </DialogHeader>
        <ScrollArea
          className="flex min-h-0 flex-1 flex-col"
          viewportClassName="h-auto min-h-0 flex-1"
        >
          <div className="flex flex-col gap-5 px-5 py-4">
            <div className="flex flex-col gap-1.5">
              {drafts.map((d) => (
                <ItemRow
                  key={d.key}
                  draft={d}
                  removable={drafts.length > 1}
                  onChange={(p) => update(d.key, p)}
                  onRemove={() => remove(d.key)}
                />
              ))}
            </div>
            <div className="grid grid-cols-[minmax(0,15rem)_minmax(0,1fr)] gap-x-5 gap-y-1.5">
              <div className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
                <FolderIcon className="size-3.5" />
                分类
              </div>
              <div className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
                <HashIcon className="size-3.5" />
                标签
              </div>
              <CategoryPicker value={category} onChange={setCategory} />
              <TagPicker value={tags} onChange={setTags} className="min-h-8" />
            </div>
          </div>
        </ScrollArea>
        <DialogFooter className="mx-0 mb-0 rounded-b-xl px-5 py-3.5">
          <Button variant="outline" onClick={onClose}>
            取消
          </Button>
          <Button
            variant={install ? "outline" : "default"}
            disabled={busy !== null || missing > 0 || drafts.length === 0}
            onClick={() => submit(false)}
          >
            {busy === "add" ? <LoaderCircleIcon className="animate-spin" /> : <PlusIcon />}
            加入清单
          </Button>
          <Button
            variant={install ? "default" : "outline"}
            disabled={busy !== null || missing > 0 || drafts.length === 0}
            onClick={() => submit(true)}
          >
            {busy === "install" ? <LoaderCircleIcon className="animate-spin" /> : <DownloadIcon />}
            加入并安装
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function ItemRow({
  draft: d,
  removable,
  onChange,
  onRemove,
}: {
  draft: Draft;
  removable: boolean;
  onChange: (p: Partial<Draft>) => void;
  onRemove: () => void;
}) {
  return (
    <div className="anim-rise rounded-xl border bg-card/60 p-2.5 shadow-xs">
      <div className="flex items-center gap-3">
        <AppIcon path={d.cand.icon} name={d.name || d.cand.name} size={36} />
        <div className="min-w-0 flex-1">
          <input
            value={d.name}
            onChange={(e) => onChange({ name: e.target.value })}
            placeholder={d.cand.name}
            aria-label="软件名称"
            spellCheck={false}
            className="-mx-1.5 w-[calc(100%+0.75rem)] rounded-md bg-transparent px-1.5 py-0.5 text-sm font-medium outline-none transition-colors hover:bg-muted focus:bg-background focus:ring-2 focus:ring-ring/40"
          />
          <div className="mt-0.5 flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground">
            <SourceBadge type={d.cand.source.type} />
            <span className="truncate font-mono select-text">{sourceID(d.cand.source)}</span>
          </div>
        </div>
        {removable && (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon-sm"
                className="text-muted-foreground"
                onClick={onRemove}
                aria-label="不加入这个"
              >
                <XIcon />
              </Button>
            </TooltipTrigger>
            <TooltipContent>不加入这个</TooltipContent>
          </Tooltip>
        )}
      </div>
      {showsRule(d) && <RuleField draft={d} onChange={onChange} />}
    </div>
  );
}

/**
 * RuleField is the rule that finds an app installed: suggested, for a
 * GitHub app, from the programs installed and the repository's name.
 */
function RuleField({
  draft: d,
  onChange,
}: {
  draft: Draft;
  onChange: (p: Partial<Draft>) => void;
}) {
  const repo = d.cand.source.type === "github" ? (d.cand.source.repo ?? "") : "";
  const suggestions = useRuleSuggestions(repo);
  const first = suggestions.data?.[0];
  useEffect(() => {
    if (first && !d.touched && !d.rule) onChange({ rule: first.rule });
    // Only a new first suggestion fills the rule.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [first?.rule]);
  const chosen = suggestions.data?.find((s) => s.rule === d.rule.trim());
  const found =
    chosen?.from ??
    (d.rule.trim() === (d.cand.detect?.displayName ?? "") ? d.cand.from : undefined);
  const invalid = needsRule(d) && !d.rule.trim();
  return (
    <div className="mt-2.5 flex flex-col gap-1.5 border-t border-dashed pt-2.5">
      <div className="flex items-center gap-2 text-xs">
        <span className="font-medium text-muted-foreground">检测规则</span>
        {found && (
          <Pill tone="success">
            <MonitorCheckIcon />
            <span className="max-w-72 truncate">
              本机：{chosen ? suggestionLabel(chosen) : found}
            </span>
          </Pill>
        )}
      </div>
      <InputGroup className="h-8 bg-background">
        <InputGroupAddon>
          <ScanSearchIcon />
        </InputGroupAddon>
        <InputGroupInput
          value={d.rule}
          onChange={(e) => onChange({ rule: e.target.value, touched: true })}
          placeholder={
            needsRule(d) ? "已安装应用里的名称，正则表达式，必填" : "已安装应用里的名称，正则表达式"
          }
          aria-invalid={invalid || undefined}
          spellCheck={false}
          className="font-mono text-xs"
        />
        {repo !== "" && (
          <InputGroupAddon align="inline-end">
            <RuleSuggestions repo={repo} onPick={(rule) => onChange({ rule, touched: true })} />
          </InputGroupAddon>
        )}
      </InputGroup>
    </div>
  );
}
