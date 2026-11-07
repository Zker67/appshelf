// A list's selection, the row whose details show, and the page's keys.

import {
  useEffect,
  useEffectEvent,
  useMemo,
  useRef,
  useState,
  type MouseEvent,
  type RefObject,
} from "react";
import { prune, selectRange, toggle } from "./selection";

export interface List {
  /** selected are the rows checked, of those listed. */
  selected: ReadonlySet<string>;
  /** focus is the row whose details show. */
  focus: string | null;
  open: (id: string) => void;
  /** closeDetails hides the details until a row is opened again. */
  closeDetails: () => void;
  /** clickRow opens a row; with Ctrl it checks it, with Shift a range. */
  clickRow: (id: string, e: MouseEvent) => void;
  check: (id: string, on: boolean, shift: boolean) => void;
  setSelected: (ids: Iterable<string>) => void;
  selectAll: (on: boolean) => void;
  /** move opens the row above or below the open one. */
  move: (delta: number) => string | null;
}

/**
 * useList keeps a list's selection and open row. With autoOpen, the first
 * row shows its details until the user closes them.
 */
export function useList(order: readonly string[], autoOpen: boolean): List {
  const [rawSelected, setRawSelected] = useState<ReadonlySet<string>>(() => new Set());
  const [rawFocus, setRawFocus] = useState<string | null>(null);
  const [closed, setClosed] = useState(!autoOpen);
  const anchor = useRef<string | null>(null);
  const selected = useMemo(() => prune(rawSelected, order), [rawSelected, order]);
  const focus =
    rawFocus !== null && order.includes(rawFocus) ? rawFocus : closed ? null : (order[0] ?? null);

  const open = (id: string) => {
    setRawFocus(id);
    setClosed(false);
    anchor.current = id;
  };
  return {
    selected,
    focus,
    open,
    closeDetails: () => {
      setRawFocus(null);
      setClosed(true);
    },
    clickRow: (id, e) => {
      if (e.shiftKey) {
        setRawSelected((s) => selectRange(order, s, anchor.current ?? focus, id, true));
        anchor.current = id;
      } else if (e.ctrlKey || e.metaKey) {
        setRawSelected((s) => toggle(s, id));
        anchor.current = id;
      } else {
        open(id);
      }
    },
    check: (id, on, shift) => {
      setRawSelected((s) =>
        shift ? selectRange(order, s, anchor.current, id, on) : toggle(s, id, on),
      );
      anchor.current = id;
    },
    setSelected: (ids) => setRawSelected(new Set(ids)),
    selectAll: (on) => setRawSelected(on ? new Set(order) : new Set()),
    move: (delta) => {
      if (order.length === 0) return null;
      const i = focus === null ? -1 : order.indexOf(focus);
      const next = order[Math.max(0, Math.min(order.length - 1, i < 0 ? 0 : i + delta))] ?? null;
      if (next !== null) open(next);
      return next;
    },
  };
}

/** isTyping reports whether keys go to a text field. */
function isTyping(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  return target.isContentEditable || target.matches("input, textarea, select");
}

/** overlayOpen reports whether a dialog or menu has the keys. */
function overlayOpen(): boolean {
  return (
    document.querySelector(
      '[role="dialog"][data-state="open"], [role="alertdialog"][data-state="open"], [role="menu"]',
    ) !== null
  );
}

export interface PageKeys {
  focusSearch?: () => void;
  selectAll?: () => void;
  /** escape returns whether it did something. */
  escape?: () => boolean;
  move?: (delta: number) => void;
  space?: () => void;
  /** enter acts on the row focused; it returns whether it did something. */
  enter?: () => boolean;
}

/**
 * usePageKeys handles a page's keys: Ctrl+F and / focus its search, Ctrl+A
 * selects its rows, Esc clears, ↑ ↓ move, Space checks, Enter acts on the
 * row focused (adds it, or opens it in the catalog). A page kept but
 * hidden has no effects, so only the page shown listens.
 */
export function usePageKeys(keys: PageKeys): void {
  const onKey = useEffectEvent((e: KeyboardEvent) => {
    if (e.defaultPrevented || overlayOpen()) return;
    const ctrl = e.ctrlKey || e.metaKey;
    if (ctrl && e.key.toLowerCase() === "f") {
      e.preventDefault();
      keys.focusSearch?.();
      return;
    }
    if (isTyping(e.target)) return;
    const onControl =
      e.target instanceof HTMLElement && e.target.closest("button, [role='checkbox'], a") !== null;
    if (e.key === "/" && !ctrl) {
      e.preventDefault();
      keys.focusSearch?.();
    } else if (ctrl && e.key.toLowerCase() === "a" && keys.selectAll) {
      e.preventDefault();
      keys.selectAll();
    } else if (e.key === "Escape" && keys.escape?.()) {
      e.preventDefault();
    } else if ((e.key === "ArrowDown" || e.key === "ArrowUp") && keys.move && !ctrl) {
      e.preventDefault();
      keys.move(e.key === "ArrowDown" ? 1 : -1);
    } else if (e.key === " " && keys.space && !onControl) {
      e.preventDefault();
      keys.space();
    } else if (e.key === "Enter" && !ctrl && !onControl && keys.enter?.()) {
      e.preventDefault();
    }
  });
  useEffect(() => {
    const listener = (e: KeyboardEvent) => onKey(e);
    window.addEventListener("keydown", listener);
    return () => window.removeEventListener("keydown", listener);
  }, []);
}

/** revealRow scrolls a row of a list into view. */
export function revealRow(container: RefObject<HTMLElement | null>, id: string | null): void {
  if (!id || !container.current) return;
  const el = container.current.querySelector(`[data-row="${CSS.escape(id)}"]`);
  el?.scrollIntoView({ block: "nearest" });
}
