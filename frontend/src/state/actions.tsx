// The dialogs every page opens: installing, adding, editing, an app's
// note, the tasks, the settings and the guide.

import {
  AddDialog,
  type AddCandidate,
  type AddDialogRequest,
} from "@/components/dialogs/add-dialog";
import { AppEditor } from "@/components/dialogs/app-editor";
import { HelpDialog } from "@/components/dialogs/help-dialog";
import { NoteEditor, type NoteRequest } from "@/components/dialogs/note-editor";
import { PreviewDialog, type PreviewRequest } from "@/components/dialogs/preview-dialog";
import { SettingsDialog, type SettingsTab } from "@/components/dialogs/settings-dialog";
import { TasksSheet } from "@/components/dialogs/tasks-sheet";
import type { AddTarget } from "@/lib/add-target";
import type { App, ShelfApp, TaskAction } from "@/mygo";
import { createContext, use, useMemo, useState, type ReactNode } from "react";

interface Actions {
  /** preview shows what installing or updating apps would do, to start it. */
  preview: (ids: string[], action: TaskAction) => void;
  previewInstalledUpdates: (keys: string[]) => void;
  /** previewUpdates previews updates of packages, by source key. */
  previewUpdates: (keys: string[]) => void;
  /** add adds packages to the catalog with a category and tags. */
  add: (items: AddCandidate[], opts?: { install?: boolean; target?: AddTarget }) => void;
  /** edit edits an app, or adds one by hand with what is known of it. */
  edit: (app?: App, draft?: Partial<App>) => void;
  /** editNote writes an app's own note. */
  editNote: (item: ShelfApp) => void;
  openTasks: () => void;
  openSettings: (tab?: SettingsTab) => void;
  openHelp: () => void;
}

const ActionsContext = createContext<Actions | null>(null);

let counter = 0;
const next = () => ++counter;

export function ActionsProvider({ children }: { children: ReactNode }) {
  // A request stays after its dialog closes, for the dialog to animate out;
  // a new one remounts the dialog by its seq.
  const [preview, setPreview] = useState<PreviewRequest | null>(null);
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
      preview: (ids, action) => setPreview({ ids, action, seq: next(), open: true }),
      previewInstalledUpdates: (keys) =>
        setPreview({ ids: keys, action: "upgrade", installed: true, seq: next(), open: true }),
      previewUpdates: (keys) =>
        setPreview({ ids: keys, action: "upgrade", updates: true, seq: next(), open: true }),
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
      openSettings: (tab) => setSettings({ tab: tab ?? "sources" }),
      openHelp: () => setHelp(true),
    }),
    [],
  );

  return (
    <ActionsContext value={value}>
      {children}
      <PreviewDialog
        key={`p${preview?.seq ?? 0}`}
        request={preview}
        onClose={() => setPreview((p) => p && { ...p, open: false })}
        onStarted={() => setTasks(true)}
      />
      <AddDialog
        key={`a${add?.seq ?? 0}`}
        request={add}
        onClose={() => setAdd((a) => a && { ...a, open: false })}
        onInstall={(ids) => setPreview({ ids, action: "install", seq: next(), open: true })}
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
