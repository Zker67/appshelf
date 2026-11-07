// A note's Markdown shown as a page: GitHub's flavor (tables, task lists,
// strikethrough, links typed bare), no raw HTML. Links open in the
// browser; ticking a task gives back where its item starts in the source.

import { CopyButton } from "@/components/common";
import { openURL } from "@/components/package-info";
import { cn } from "@/lib/utils";
import { CheckIcon } from "lucide-react";
import { createContext, memo, use, type ReactNode } from "react";
import Markdown, { type Components, type ExtraProps } from "react-markdown";
import remarkGfm from "remark-gfm";

type HNode = NonNullable<ExtraProps["node"]>;
type HChild = HNode["children"][number];

function textOf(nodes: HChild[]): string {
  return nodes
    .map((n) => (n.type === "text" ? n.value : n.type === "element" ? textOf(n.children) : ""))
    .join("");
}

const isWeb = (url: string | undefined): url is string => !!url && /^https?:\/\//i.test(url);

/** TaskAt is where the task item being drawn starts in the source. */
const TaskAt = createContext<number | null>(null);
/** Toggle ticks the task whose item starts at the offset; null: read only. */
const Toggle = createContext<((offset: number) => void) | null>(null);

function TaskBox({ checked }: { checked: boolean }) {
  const offset = use(TaskAt);
  const toggle = use(Toggle);
  const can = toggle !== null && offset !== null;
  return (
    <button
      type="button"
      role="checkbox"
      aria-checked={checked}
      aria-label={checked ? "标为未完成" : "标为完成"}
      disabled={!can}
      onClick={(e) => {
        e.stopPropagation();
        if (can) toggle(offset);
      }}
      className={cn(
        "absolute top-[3px] left-0 inline-flex size-4 items-center justify-center rounded-[5px] border transition-[background-color,border-color,transform] duration-200 ease-spring",
        checked
          ? "border-primary bg-primary text-primary-foreground"
          : "border-foreground/25 bg-card",
        can && "hover:border-primary active:scale-90",
      )}
    >
      <CheckIcon
        strokeWidth={3}
        className={cn(
          "size-3 transition-[opacity,transform] duration-200 ease-spring",
          checked ? "scale-100 opacity-100" : "scale-50 opacity-0",
        )}
      />
    </button>
  );
}

const components: Components = {
  h1: ({ children }) => (
    <h1 className="mt-5 mb-2 text-lg font-semibold tracking-tight first:mt-0">{children}</h1>
  ),
  h2: ({ children }) => (
    <h2 className="mt-5 mb-2 border-b border-border/60 pb-1 text-base font-semibold tracking-tight first:mt-0">
      {children}
    </h2>
  ),
  h3: ({ children }) => (
    <h3 className="mt-4 mb-1.5 text-[14px] font-semibold first:mt-0">{children}</h3>
  ),
  h4: ({ children }) => (
    <h4 className="mt-3 mb-1 text-[13px] font-semibold text-muted-foreground first:mt-0">
      {children}
    </h4>
  ),
  h5: ({ children }) => <h5 className="mt-3 mb-1 text-[13px] font-semibold">{children}</h5>,
  h6: ({ children }) => (
    <h6 className="mt-3 mb-1 text-xs font-semibold text-muted-foreground">{children}</h6>
  ),
  p: ({ children }) => <p className="my-2 first:mt-0 last:mb-0">{children}</p>,
  a: ({ href, children }) =>
    isWeb(href) ? (
      <a
        href={href}
        title={href}
        onClick={(e) => {
          e.preventDefault();
          e.stopPropagation();
          openURL(href);
        }}
        className="font-medium text-primary underline decoration-primary/30 underline-offset-[3px] transition-colors hover:decoration-primary"
      >
        {children}
      </a>
    ) : (
      <span className="text-primary">{children}</span>
    ),
  strong: ({ children }) => <strong className="font-semibold">{children}</strong>,
  del: ({ children }) => <del className="text-muted-foreground">{children}</del>,
  blockquote: ({ children }) => (
    <blockquote className="my-2.5 rounded-r-lg border-l-[3px] border-primary/40 bg-primary/[0.04] py-1.5 pr-3 pl-3 text-muted-foreground">
      {children}
    </blockquote>
  ),
  hr: () => <hr className="my-4 border-border/70" />,
  ul: ({ children, className }) => (
    <ul
      className={cn(
        "my-2 space-y-1 pl-5",
        className?.includes("contains-task-list")
          ? "list-none pl-0.5"
          : "list-disc marker:text-primary/60",
      )}
    >
      {children}
    </ul>
  ),
  ol: ({ children, start }) => (
    <ol start={start} className="my-2 list-decimal space-y-1 pl-5 marker:text-muted-foreground">
      {children}
    </ol>
  ),
  li: ({ children, className, node }) => {
    const task = className?.includes("task-list-item");
    const offset = node?.position?.start.offset;
    return (
      <TaskAt value={task && offset !== undefined ? offset : null}>
        <li className={cn("pl-0.5", task && "relative list-none pl-6")}>{children}</li>
      </TaskAt>
    );
  },
  input: ({ type, checked }) => (type === "checkbox" ? <TaskBox checked={!!checked} /> : null),
  code: ({ children, className }) => {
    // Inside pre, the block's own look; alone, a chip.
    if (className?.startsWith("language-")) return <code>{children}</code>;
    return (
      <code className="rounded-[5px] bg-foreground/[0.06] px-[5px] py-px font-mono text-[0.9em] dark:bg-white/[0.08]">
        {children}
      </code>
    );
  },
  pre: ({ node, children }) => {
    const code = node ? textOf(node.children).replace(/\n$/, "") : "";
    const lang = /language-(\S+)/.exec(
      String(
        (node?.children[0] as { properties?: { className?: string[] } } | undefined)?.properties
          ?.className?.[0] ?? "",
      ),
    )?.[1];
    return (
      <div className="group/code relative my-2.5 overflow-hidden rounded-lg bg-foreground/[0.045] ring-1 ring-border/60 dark:bg-white/[0.04]">
        {lang && (
          <div className="border-b border-border/50 px-3 py-1 font-mono text-[11px] text-muted-foreground">
            {lang}
          </div>
        )}
        <pre className="overflow-x-auto px-3 py-2.5 font-mono text-[12.5px] leading-relaxed [&_code]:bg-transparent [&_code]:p-0">
          {children}
        </pre>
        <CopyButton
          text={code}
          what="代码"
          className="absolute top-1 right-1 bg-card/80 opacity-0 backdrop-blur transition-opacity group-hover/code:opacity-100 focus-visible:opacity-100"
        />
      </div>
    );
  },
  table: ({ children }) => (
    <div className="my-2.5 overflow-x-auto rounded-lg ring-1 ring-border/70">
      <table className="w-full border-collapse text-[13px]">{children}</table>
    </div>
  ),
  thead: ({ children }) => <thead className="bg-foreground/[0.035]">{children}</thead>,
  tr: ({ children }) => <tr className="border-b border-border/60 last:border-0">{children}</tr>,
  th: ({ children, style }) => (
    <th style={style} className="px-2.5 py-1.5 text-left font-semibold">
      {children}
    </th>
  ),
  td: ({ children, style }) => (
    <td style={style} className="px-2.5 py-1.5 align-top">
      {children}
    </td>
  ),
  img: ({ src, alt }) =>
    typeof src === "string" && isWeb(src) ? (
      <img
        src={src}
        alt={alt ?? ""}
        loading="lazy"
        className="my-2 max-h-80 max-w-full rounded-lg ring-1 ring-border/60"
      />
    ) : (
      <span className="text-muted-foreground">{alt}</span>
    ),
};

const remarkPlugins = [remarkGfm];

/**
 * MarkdownView draws a note. With onToggleTask its task boxes can be
 * ticked: it gets where the task's item starts in source.
 */
export const MarkdownView = memo(function MarkdownView({
  source,
  onToggleTask,
  className,
  empty,
}: {
  source: string;
  onToggleTask?: (offset: number) => void;
  className?: string;
  /** What shows for an empty note. */
  empty?: ReactNode;
}) {
  if (source.trim() === "" && empty) return <>{empty}</>;
  return (
    <Toggle value={onToggleTask ?? null}>
      <div
        className={cn(
          "text-[13.5px] leading-relaxed break-words text-foreground/90 select-text",
          className,
        )}
      >
        <Markdown remarkPlugins={remarkPlugins} components={components}>
          {source}
        </Markdown>
      </div>
    </Toggle>
  );
});
