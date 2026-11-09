// The dialogs every page opens: removing, adding, editing, an app's note,
// the tasks, the settings and the guide; and installs and updates, which
// start without one.

import {
  AddDialog,
  type AddCandidate,
  type AddDialogRequest,
} from "@/components/dialogs/add-dialog";
import { AppEditor } from "@/components/dialogs/app-editor";
import { HelpDialog } from "@/components/dialogs/help-dialog";
import { LeftoversDialog, type LeftoversRequest } from "@/components/dialogs/leftovers-dialog";
import { NoteEditor, type NoteRequest } from "@/components/dialogs/note-editor";
import { SettingsDialog, type SettingsTab } from "@/components/dialogs/settings-dialog";
import { TasksSheet } from "@/components/dialogs/tasks-sheet";
import { UninstallDialog, type UninstallRequest } from "@/components/dialogs/uninstall-dialog";
import type { AddTarget } from "@/lib/add-target";
import { errorText } from "@/lib/api";
import { Installed, Tasks, Updates, type App, type ShelfApp } from "@/mygo";
import { createContext, use, useMemo, useState, type ReactNode } from "react";
import { toast } from "sonner";

interface Actions {
  /**
   * Installs and updates start at once, without a preview: Go waits for
   * the states being read, checks each again and queues those that can
   * run, and the queue checks before and after. install and upgrade take
   * apps of the catalog; updateInstalled programs of the installed list,
   * by key; update packages, by source key.
   */
  install: (ids: string[]) => void;
  upgrade: (ids: string[]) => void;
  updateInstalled: (keys: string[]) => void;
  update: (keys: string[]) => void;
  /** previewUninstall shows what removing programs would do, by key of the installed list. */
  previewUninstall: (keys: string[]) => void;
  /** findLeftovers looks for what a removal left behind. */
  findLeftovers: (taskId: number, name: string) => void;
  /** add adds packages to the catalog with a category and tags. */
  add: (items: AddCandidate[], opts?: { install?: boolean; target?: AddTarget }) => void;
  /** edit edits an app, or adds one by hand with what is known of it. */
  edit: (app?: App, draft?: Partial<App>) => void;
  /** editNote writes an app's own note. */
  editNote: (item: ShelfApp) => void;
  openTasks: () => void;
  /** toggleTasks opens the task panel, or closes it when open. */
  toggleTasks: () => void;
  openSettings: (tab?: SettingsTab) => void;
  openHelp: () => void;
}

const ActionsContext = createContext<Actions | null>(null);

let counter = 0;
const next = () => ++counter;

/**
 * startTasks starts installs or updates, saying how many and how many
 * could not; it may wait a while for the states of apps just added.
 */
async function startTasks(
  verb: "安装" | "更新",
  asked: number,
  start: () => Promise<number>,
  started: () => void,
) {
  const id = toast.loading(`正在准备${verb}`);
  try {
    const n = await start();
    toast.success(`已开始${verb} ${n} 个软件`, {
      id,
      description: asked > n ? `另有 ${asked - n} 个现在不能${verb}，已跳过` : undefined,
    });
    started();
  } catch (err) {
    toast.error(`无法开始${verb}`, { id, description: errorText(err) });
  }
}

export function ActionsProvider({ children }: { children: ReactNode }) {
  // A request stays after its dialog closes, for the dialog to animate out;
  // a new one remounts the dialog by its seq.
  const [uninstall, setUninstall] = useState<UninstallRequest | null>(null);
  const [leftovers, setLeftovers] = useState<LeftoversRequest | null>(null);
  const [add, setAdd] = useState<AddDialogRequest | null>(null);
  const [edit, setEdit] = useState<{
    app?: App;
    draft?: Partial<App>;
    seq: number;
    open: boolean;
  } | null>(null);
  const [note, setNote] = useState<NoteRequest | null>(null);
  const [tasks, setTasks] = useState(false);
  const [settings, setSettings] = useState<{ tab: SettingsTab } | null>(null);
  const [help, setHelp] = useState(false);

  const value = useMemo<Actions>(
    () => ({
      install: (ids) =>
        void startTasks(
          "安装",
          ids.length,
          () => Tasks.start(ids, "install"),
          () => setTasks(true),
        ),
      upgrade: (ids) =>
        void startTasks(
          "更新",
          ids.length,
          () => Tasks.start(ids, "upgrade"),
          () => setTasks(true),
        ),
      updateInstalled: (keys) =>
        void startTasks(
          "更新",
          keys.length,
          () => Installed.startUpdates(keys),
          () => setTasks(true),
        ),
      update: (keys) =>
        void startTasks(
          "更新",
          keys.length,
          () => Updates.start(keys),
          () => setTasks(true),
        ),
      previewUninstall: (keys) => setUninstall({ keys, seq: next(), open: true }),
      findLeftovers: (taskId, name) => setLeftovers({ taskId, name, seq: next(), open: true }),
      add: (items, opts) =>
        setAdd({
          items,
          install: opts?.install ?? false,
          target: opts?.target,
          seq: next(),
          open: true,
        }),
      edit: (app, draft) => setEdit({ app, draft, seq: next(), open: true }),
      editNote: ({ app, icon }) =>
        setNote({
          id: app.id,
          name: app.name,
          icon,
          note: app.note ?? "",
          seq: next(),
          open: true,
        }),
      openTasks: () => setTasks(true),
      toggleTasks: () => setTasks((open) => !open),
      openSettings: (tab) => setSettings({ tab: tab ?? "sources" }),
      openHelp: () => setHelp(true),
    }),
    [],
  );

  return (
    <ActionsContext value={value}>
      {children}
      <UninstallDialog
        key={`u${uninstall?.seq ?? 0}`}
        request={uninstall}
        onClose={() => setUninstall((u) => u && { ...u, open: false })}
        onStarted={() => setTasks(true)}
      />
      <LeftoversDialog
        key={`l${leftovers?.seq ?? 0}`}
        request={leftovers}
        onClose={() => setLeftovers((l) => l && { ...l, open: false })}
      />
      <AddDialog
        key={`a${add?.seq ?? 0}`}
        request={add}
        onClose={() => setAdd((a) => a && { ...a, open: false })}
        onInstall={(ids) => value.install(ids)}
      />
      <AppEditor
        key={`e${edit?.seq ?? 0}`}
        request={edit}
        onClose={() => setEdit((e) => e && { ...e, open: false })}
      />
      <NoteEditor
        key={`n${note?.seq ?? 0}`}
        request={note}
        onClose={() => setNote((n) => n && { ...n, open: false })}
      />
      <TasksSheet open={tasks} onOpenChange={setTasks} />
      <SettingsDialog
        open={settings !== null}
        tab={settings?.tab ?? "sources"}
        onOpenChange={(o) => !o && setSettings(null)}
      />
      <HelpDialog open={help} onOpenChange={setHelp} />
    </ActionsContext>
  );
}

export function useActions(): Actions {
  const a = use(ActionsContext);
  if (!a) throw new Error("useActions outside ActionsProvider");
  return a;
}
