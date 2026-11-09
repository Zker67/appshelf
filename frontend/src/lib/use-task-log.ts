// A task's output, read as it comes: from where the last read ended, again
// each time Go says the tasks changed.

import { Tasks, events } from "@/mygo";
import { useEffect, useEffectEvent, useRef, useState } from "react";
import { errorText } from "./api";

/** The most lines kept in the window, as Go keeps. */
const maxLines = 2000;

export interface TaskLogView {
  lines: string[];
  /** live is the line being written, which may still change. */
  live: string;
  /** dropped means older lines are left out. */
  dropped: boolean;
  done: boolean;
  error: string;
}

const empty: TaskLogView = { lines: [], live: "", dropped: false, done: false, error: "" };

/**
 * useTaskLog reads the output of task id while enabled. A task retried
 * keeps its id: remount the reader for each run.
 */
export function useTaskLog(id: number, enabled: boolean): TaskLogView {
  const [view, setView] = useState<TaskLogView>(empty);
  const next = useRef(0);
  const busy = useRef(false);
  const again = useRef(false);
  const ended = useRef(false);

  const read = useEffectEvent(async () => {
    if (ended.current) return;
    if (busy.current) {
      again.current = true;
      return;
    }
    busy.current = true;
    try {
      do {
        again.current = false;
        const from = next.current;
        const got = await Tasks.log(id, from);
        next.current = got.next;
        setView((v) => {
          let lines = got.start > from ? got.lines : [...v.lines, ...got.lines];
          const dropped = v.dropped || got.start > from || lines.length > maxLines;
          if (lines.length > maxLines) lines = lines.slice(lines.length - maxLines);
          return { lines, live: got.live ?? "", dropped, done: got.done, error: "" };
        });
        if (got.done) {
          ended.current = true;
          break;
        }
      } while (again.current);
    } catch (err) {
      setView((v) => ({ ...v, error: errorText(err) }));
    } finally {
      busy.current = false;
    }
  });

  useEffect(() => {
    if (!enabled) return;
    void read();
    return events.changed.on((change) => {
      if (change.kinds.includes("tasks")) void read();
    });
  }, [enabled, id]);

  return view;
}
