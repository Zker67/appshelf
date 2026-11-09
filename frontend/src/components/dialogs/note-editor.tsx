// Writing an app's own note: Markdown at the left, the page it makes at
// the right as it is typed. A toolbar and the usual keys mark the text;
// the note saves itself a moment after typing stops, and on closing.

import { AppIcon } from "@/components/app-icon";
import { MarkdownView } from "@/components/markdown";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog";
import { Kbd } from "@/components/ui/kbd";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { errorText } from "@/lib/api";
import {
  continueList,
  indentLines,
  insertBlock,
  link,
  maxNoteLen,
  noteLength,
  toggleInline,
  toggleLines,
  toggleTask,
  type Edit,
} from "@/lib/markdown";
import { cn } from "@/lib/utils";
import { Shelf } from "@/mygo";
import {
  BoldIcon,
  CheckIcon,
  CloudAlertIcon,
  CodeIcon,
  Columns2Icon,
  EyeIcon,
  HeadingIcon,
  ItalicIcon,
  LinkIcon,
  ListChecksIcon,
  ListIcon,
  ListOrderedIcon,
  LoaderCircleIcon,
  MinusIcon,
  NotebookPenIcon,
  PenLineIcon,
  QuoteIcon,
  SquareCodeIcon,
  StrikethroughIcon,
  type LucideIcon,
} from "lucide-react";
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { toast } from "sonner";

export interface NoteRequest {
  id: string;
  name: string;
  icon: string;
  note: string;
  seq: number;
  open: boolean;
}

type Mode = "write" | "split" | "preview";
type SaveState = "saved" | "dirty" | "saving" | "failed";

const modeKey = "note.mode";
const saveDelay = 600;

function storedMode(): Mode {
  const v = localStorage.getItem(modeKey);
  return v === "write" || v === "preview" ? v : "split";
}

const modes: { value: Mode; label: string; icon: LucideIcon }[] = [
  { value: "write", label: "编辑", icon: PenLineIcon },
  { value: "split", label: "分栏", icon: Columns2Icon },
  { value: "preview", label: "预览", icon: EyeIcon },
];

type Tool = { label: string; icon: LucideIcon; keys?: string[]; run: (e: Edit) => Edit } | "|";

const tools: Tool[] = [
  { label: "标题", icon: HeadingIcon, run: (e) => toggleLines(e, "heading") },
  { label: "粗体", icon: BoldIcon, keys: ["Ctrl", "B"], run: (e) => toggleInline(e, "bold") },
  { label: "斜体", icon: ItalicIcon, keys: ["Ctrl", "I"], run: (e) => toggleInline(e, "italic") },
  { label: "删除线", icon: StrikethroughIcon, run: (e) => toggleInline(e, "strike") },
  { label: "行内代码", icon: CodeIcon, keys: ["Ctrl", "E"], run: (e) => toggleInline(e, "code") },
  "|",
  { label: "链接", icon: LinkIcon, keys: ["Ctrl", "K"], run: link },
  "|",
  { label: "列表", icon: ListIcon, run: (e) => toggleLines(e, "bullet") },
  { label: "编号列表", icon: ListOrderedIcon, run: (e) => toggleLines(e, "ordered") },
  { label: "任务清单", icon: ListChecksIcon, run: (e) => toggleLines(e, "task") },
  { label: "引用", icon: QuoteIcon, run: (e) => toggleLines(e, "quote") },
  "|",
  { label: "代码块", icon: SquareCodeIcon, run: (e) => insertBlock(e, "code") },
  { label: "分隔线", icon: MinusIcon, run: (e) => insertBlock(e, "rule") },
];

const shortcuts: Record<string, (e: Edit) => Edit> = {
  b: (e) => toggleInline(e, "bold"),
  i: (e) => toggleInline(e, "italic"),
  e: (e) => toggleInline(e, "code"),
  k: link,
};

export function NoteEditor({
  request,
  onClose,
}: {
  request: NoteRequest | null;
  onClose: () => void;
}) {
  const [text, setText] = useState(request?.note ?? "");
  const [mode, setModeState] = useState<Mode>(storedMode);
  const [save, setSave] = useState<SaveState>("saved");
  const area = useRef<HTMLTextAreaElement>(null);
  const preview = useRef<HTMLDivElement>(null);
  // What Go has, what is being sent, and the text as typed, for the timer
  // and closing to read without waiting for a render.
  const saved = useRef(request?.note ?? "");
  const sending = useRef<Promise<void> | null>(null);
  const latest = useRef(text);
  latest.current = text;
  const timer = useRef<number | undefined>(undefined);
  // A selection to put back once the text set by a tool is drawn.
  const select = useRef<{ start: number; end: number } | null>(null);

  const setMode = (m: Mode) => {
    setModeState(m);
    localStorage.setItem(modeKey, m);
    if (m !== "preview") requestAnimationFrame(() => area.current?.focus());
  };

  const flush = useCallback(async (): Promise<boolean> => {
    window.clearTimeout(timer.current);
    if (!request) return true;
    // One save at a time; what was typed meanwhile goes next.
    while (sending.current) await sending.current;
    const want = latest.current;
    if (want === saved.current) {
      setSave("saved");
      return true;
    }
    if (noteLength(want) > maxNoteLen) {
      setSave("failed");
      return false;
    }
    setSave("saving");
    let ok = true;
    const p = Shelf.setNote(request.id, want)
      .then(() => {
        // Go tidies the blank lines around it; the text as sent is saved.
        saved.current = want;
      })
      .catch((err: unknown) => {
        ok = false;
        toast.error("备注没有保存", { description: errorText(err) });
      })
      .finally(() => {
        sending.current = null;
      });
    sending.current = p;
    await p;
    if (!ok) {
      setSave("failed");
      return false;
    }
    // What was typed while saving waits for its own timer.
    setSave(latest.current === saved.current ? "saved" : "dirty");
    return true;
  }, [request]);

  const change = (next: string) => {
    setText(next);
    latest.current = next;
    setSave(next === saved.current ? "saved" : "dirty");
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => void flush(), saveDelay);
  };

  useEffect(() => () => window.clearTimeout(timer.current), []);

  const close = () => {
    if (noteLength(latest.current) > maxNoteLen) {
      toast.error(`备注超过 ${maxNoteLen} 个字，超出的修改没有保存`);
    } else void flush();
    onClose();
  };

  // A tool's edit goes in as typing would, only the part that changed, for
  // Ctrl+Z to take it back and the box to stay where it was scrolled.
  const apply = (fn: (e: Edit) => Edit | null): boolean => {
    const el = area.current;
    if (!el) return false;
    const old = el.value;
    const after = fn({ text: old, start: el.selectionStart, end: el.selectionEnd });
    if (!after) return false;
    let head = 0;
    const most = Math.min(old.length, after.text.length);
    while (head < most && old[head] === after.text[head]) head++;
    let tail = 0;
    while (
      tail < most - head &&
      old[old.length - 1 - tail] === after.text[after.text.length - 1 - tail]
    )
      tail++;
    el.focus();
    el.setSelectionRange(head, old.length - tail);
    const insert = after.text.slice(head, after.text.length - tail);
    const typed =
      insert === ""
        ? head === old.length - tail || document.execCommand("delete")
        : document.execCommand("insertText", false, insert);
    if (!typed || el.value !== after.text) change(after.text);
    select.current = { start: after.start, end: after.end };
    el.setSelectionRange(after.start, after.end);
    return true;
  };

  useLayoutEffect(() => {
    const s = select.current;
    const el = area.current;
    if (!s || !el) return;
    select.current = null;
    el.setSelectionRange(s.start, s.end);
  });

  const onKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.nativeEvent.isComposing) return;
    const mod = e.ctrlKey || e.metaKey;
    if (mod && e.key.toLowerCase() === "s") {
      e.preventDefault();
      void flush();
      return;
    }
    if (mod && !e.shiftKey && !e.altKey) {
      const fn = shortcuts[e.key.toLowerCase()];
      if (fn) {
        e.preventDefault();
        apply(fn);
      }
      return;
    }
    if (e.key === "Enter" && !e.shiftKey && !mod && !e.altKey) {
      if (apply(continueList)) e.preventDefault();
      return;
    }
    if (e.key === "Tab" && !mod && !e.altKey) {
      if (apply((x) => indentLines(x, e.shiftKey))) e.preventDefault();
    }
  };

  // Split, the page follows the text: as far down, as a share of its height.
  const follow = () => {
    const el = area.current;
    const view = preview.current;
    if (!el || !view || mode !== "split") return;
    const room = el.scrollHeight - el.clientHeight;
    view.scrollTop = room > 0 ? (el.scrollTop / room) * (view.scrollHeight - view.clientHeight) : 0;
  };

  const tick = (offset: number) => {
    const next = toggleTask(latest.current, offset);
    if (next !== latest.current) change(next);
  };

  const chars = noteLength(text);
  const over = chars > maxNoteLen;
  const open = request?.open ?? false;

  return (
    <Dialog open={open} onOpenChange={(o) => !o && close()}>
      <DialogContent
        showCloseButton={false}
        onOpenAutoFocus={(e) => {
          e.preventDefault();
          const el = area.current;
          if (!el) return;
          el.focus();
          el.setSelectionRange(el.value.length, el.value.length);
        }}
        className="flex h-[min(720px,calc(100vh-5rem))] flex-col gap-0 overflow-hidden p-0 sm:max-w-[min(1100px,calc(100vw-4rem))]"
      >
        <div className="flex shrink-0 items-center gap-3 border-b bg-linear-to-b from-primary/[0.07] to-transparent px-4 py-3">
          {request && <AppIcon path={request.icon} name={request.name} size={28} />}
          <div className="min-w-0 flex-1">
            <DialogTitle className="flex min-w-0 items-center gap-1.5 text-[15px] font-semibold">
              <span className="truncate">{request?.name}</span>
              <span className="shrink-0 text-muted-foreground">的备注</span>
            </DialogTitle>
            <DialogDescription className="text-xs">
              支持 Markdown，边写边预览，停笔后自动保存
            </DialogDescription>
          </div>
          <SaveBadge state={over ? "failed" : save} over={over} />
          <ModeSwitch mode={mode} onChange={setMode} />
        </div>

        <div
          className={cn(
            "flex shrink-0 items-center gap-0.5 border-b px-2.5 py-1.5 transition-opacity",
            mode === "preview" && "pointer-events-none opacity-40",
          )}
          aria-hidden={mode === "preview" || undefined}
        >
          {tools.map((t, i) =>
            t === "|" ? (
              <span key={i} className="mx-1 h-4 w-px bg-border" />
            ) : (
              <Tooltip key={t.label}>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    className="text-muted-foreground hover:text-foreground"
                    aria-label={t.label}
                    tabIndex={mode === "preview" ? -1 : 0}
                    onMouseDown={(e) => e.preventDefault()}
                    onClick={() => apply(t.run)}
                  >
                    <t.icon />
                  </Button>
                </TooltipTrigger>
                <TooltipContent className="flex items-center gap-1.5">
                  {t.label}
                  {t.keys && (
                    <span className="flex gap-0.5">
                      {t.keys.map((k) => (
                        <Kbd key={k}>{k}</Kbd>
                      ))}
                    </span>
                  )}
                </TooltipContent>
              </Tooltip>
            ),
          )}
          <span
            className={cn(
              "ml-auto pr-1.5 font-mono text-[11px] tabular-nums",
              over ? "text-destructive" : "text-muted-foreground",
            )}
          >
            {chars.toLocaleString()} / {maxNoteLen.toLocaleString()}
          </span>
        </div>

        <div
          className={cn("grid min-h-0 flex-1", mode === "split" ? "grid-cols-2" : "grid-cols-1")}
        >
          <div className={cn("relative min-h-0", mode === "preview" && "hidden")}>
            <textarea
              ref={area}
              value={text}
              onChange={(e) => change(e.target.value)}
              onKeyDown={onKeyDown}
              onScroll={follow}
              spellCheck={false}
              aria-label="备注 Markdown"
              placeholder={placeholder}
              className={cn(
                "size-full resize-none bg-transparent px-5 py-4 font-mono text-[13px] leading-[1.7] outline-none placeholder:text-muted-foreground/60 [scrollbar-color:var(--border)_transparent] [scrollbar-width:thin]",
                mode === "write" && "mx-auto max-w-3xl",
              )}
            />
          </div>
          <div
            className={cn(
              "min-h-0 bg-surface/60",
              mode === "write" && "hidden",
              mode === "split" && "border-l",
            )}
          >
            <ScrollArea className="h-full" viewportRef={preview}>
              <div className={cn("px-6 py-4", mode === "preview" && "mx-auto max-w-3xl")}>
                <MarkdownView source={text} onToggleTask={tick} empty={<PreviewEmpty />} />
              </div>
            </ScrollArea>
          </div>
        </div>

        <div className="flex shrink-0 items-center gap-3 border-t px-4 py-2.5 text-xs text-muted-foreground">
          <span className="flex items-center gap-1">
            <Kbd>Ctrl</Kbd>
            <Kbd>S</Kbd>
            立即保存
          </span>
          <span className="flex items-center gap-1">
            <Kbd>Tab</Kbd>
            缩进列表
          </span>
          <span className="flex items-center gap-1">
            <Kbd>Esc</Kbd>
            保存并关闭
          </span>
          <Button size="sm" className="ml-auto" onClick={close}>
            <CheckIcon />
            完成
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}

const placeholder = `写点只属于你的东西，例如：

## 安装后
- [ ] 登录账号，打开设置同步
- [ ] 关掉开机自启

**激活方式**：…
常用快捷键：\`Ctrl\` + \`Shift\` + \`P\``;

function PreviewEmpty() {
  return (
    <div className="flex h-full min-h-48 flex-col items-center justify-center gap-2 text-center text-muted-foreground">
      <NotebookPenIcon className="size-7 opacity-40" />
      <div className="text-[13px]">左边写下的内容会在这里实时显示</div>
    </div>
  );
}

function SaveBadge({ state, over }: { state: SaveState; over: boolean }) {
  const view = over
    ? { icon: CloudAlertIcon, label: "超出字数", tone: "text-destructive" }
    : {
        saved: { icon: CheckIcon, label: "已保存", tone: "text-success" },
        dirty: { icon: PenLineIcon, label: "编辑中", tone: "text-muted-foreground" },
        saving: { icon: LoaderCircleIcon, label: "保存中", tone: "text-muted-foreground" },
        failed: { icon: CloudAlertIcon, label: "保存失败", tone: "text-destructive" },
      }[state];
  return (
    <span
      key={view.label}
      className={cn("anim-fade flex shrink-0 items-center gap-1 text-xs font-medium", view.tone)}
      aria-live="polite"
    >
      <view.icon className={cn("size-3.5", state === "saving" && !over && "animate-spin")} />
      {view.label}
    </span>
  );
}

function ModeSwitch({ mode, onChange }: { mode: Mode; onChange: (m: Mode) => void }) {
  const index = modes.findIndex((m) => m.value === mode);
  return (
    <div
      role="radiogroup"
      aria-label="显示方式"
      className="seg-track relative grid shrink-0 grid-cols-3 rounded-xl p-[3px]"
    >
      <div
        className="absolute top-[3px] bottom-[3px] left-[3px] w-[calc((100%-6px)/3)] rounded-[9px] seg-thumb transition-transform duration-400 ease-spring"
        style={{ transform: `translateX(${index * 100}%)` }}
      />
      {modes.map((m) => (
        <button
          key={m.value}
          type="button"
          role="radio"
          aria-checked={mode === m.value}
          onClick={() => onChange(m.value)}
          className="group/mode relative flex h-7 items-center justify-center gap-1.5 rounded-[9px] px-2.5 text-[13px] font-medium text-muted-foreground transition-colors outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/50 aria-checked:text-foreground"
        >
          <m.icon className="size-3.5 transition-colors group-aria-checked/mode:text-primary" />
          {m.label}
        </button>
      ))}
    </div>
  );
}
