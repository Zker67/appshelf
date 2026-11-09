// The GitHub token in the settings: where it comes from, and entering,
// replacing or clearing it. Go checks it with GitHub and keeps it
// encrypted; the page never reads it back.

import { Hint } from "@/components/common";
import { Pill } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "@/components/ui/input-group";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { errorText } from "@/lib/api";
import { About, Sources, type TokenSource } from "@/mygo";
import {
  CheckCircle2Icon,
  CircleDashedIcon,
  EyeIcon,
  EyeOffIcon,
  ExternalLinkIcon,
  KeyRoundIcon,
  LoaderCircleIcon,
  LockKeyholeIcon,
  PencilIcon,
  PlusIcon,
  ShieldCheckIcon,
  TerminalIcon,
  Trash2Icon,
  XIcon,
} from "lucide-react";
import { useRef, useState, type FormEvent } from "react";
import { toast } from "sonner";

/** Where a token for public repositories is created: it needs no permission. */
const createURL =
  "https://github.com/settings/personal-access-tokens/new?name=AppShelf&description=AppShelf+%E8%BD%AF%E4%BB%B6%E5%9B%BE%E4%B9%A6%E9%A6%86";

function FromPill({ from }: { from: TokenSource }) {
  switch (from) {
    case "saved":
      return (
        <Pill tone="success">
          <LockKeyholeIcon />
          已加密保存
        </Pill>
      );
    case "env":
      return (
        <Pill tone="primary">
          <TerminalIcon />
          来自环境变量
        </Pill>
      );
    default:
      return (
        <Pill tone="muted">
          <CircleDashedIcon />
          未设置
        </Pill>
      );
  }
}

/** TokenRow shows the GitHub token's source and edits it in place. */
export function TokenRow({ from }: { from: TokenSource }) {
  const [editing, setEditing] = useState(false);
  const [value, setValue] = useState("");
  const [shown, setShown] = useState(false);
  const [saving, setSaving] = useState(false);
  const [clearing, setClearing] = useState(false);
  const input = useRef<HTMLInputElement>(null);

  const open = () => {
    setValue("");
    setShown(false);
    setEditing(true);
  };
  const cancel = () => {
    setEditing(false);
    setValue("");
  };
  const save = async (e?: FormEvent) => {
    e?.preventDefault();
    if (!value.trim() || saving) return;
    setSaving(true);
    try {
      await Sources.setGitHubToken(value);
      toast.success("已保存 GitHub token", { description: "已验证可用，立即生效" });
      setEditing(false);
      setValue("");
    } catch (err) {
      toast.error("没有保存 token", { description: errorText(err) });
      input.current?.focus();
    } finally {
      setSaving(false);
    }
  };
  const clear = async () => {
    setClearing(true);
    try {
      await Sources.clearGitHubToken();
      toast.success("已清除保存的 token", {
        description: "如果设置了环境变量，会改用它",
      });
    } catch (err) {
      toast.error("清除失败", { description: errorText(err) });
    } finally {
      setClearing(false);
    }
  };
  const openCreate = () =>
    void About.openURL(createURL).catch((err: unknown) =>
      toast.error("无法打开网页", { description: errorText(err) }),
    );

  return (
    <div className="flex flex-col">
      <div className="flex items-center gap-3 px-4 py-3">
        <KeyRoundIcon className="size-4 shrink-0 text-muted-foreground" />
        <span className="w-24 shrink-0 text-sm">token</span>
        <FromPill from={from} />
        <Hint>
          {from === "saved"
            ? "用 Windows 加密保存在本机，优先于环境变量"
            : from === "env"
              ? "来自环境变量 GH_TOKEN 或 GITHUB_TOKEN"
              : "不填时每小时最多 60 次请求；填写后每小时 5000 次"}
        </Hint>
        <span className="flex-1" />
        {!editing && (
          <div className="flex shrink-0 items-center gap-1.5">
            {from === "saved" && (
              <Button variant="ghost" size="sm" disabled={clearing} onClick={() => void clear()}>
                {clearing ? <LoaderCircleIcon className="animate-spin" /> : <Trash2Icon />}
                清除
              </Button>
            )}
            <Button variant={from === "" ? "default" : "outline"} size="sm" onClick={open}>
              {from === "" ? <PlusIcon /> : <PencilIcon />}
              {from === "" ? "填写 token" : from === "saved" ? "更换" : "改用填写的"}
            </Button>
          </div>
        )}
      </div>
      {editing && (
        <form className="anim-rise flex flex-col gap-2 px-4 pb-3.5" onSubmit={(e) => void save(e)}>
          <div className="flex items-center gap-2">
            <InputGroup className="h-8 flex-1">
              <InputGroupAddon>
                <KeyRoundIcon />
              </InputGroupAddon>
              <InputGroupInput
                ref={input}
                autoFocus
                type={shown ? "text" : "password"}
                autoComplete="off"
                spellCheck={false}
                placeholder="github_pat_… 或 ghp_…"
                className="font-mono text-xs"
                value={value}
                disabled={saving}
                onChange={(e) => setValue(e.target.value)}
              />
              <InputGroupAddon align="inline-end">
                <Tooltip>
                  <TooltipTrigger asChild>
                    <InputGroupButton
                      size="icon-xs"
                      aria-label={shown ? "隐藏" : "显示"}
                      onClick={() => setShown((s) => !s)}
                    >
                      {shown ? <EyeOffIcon /> : <EyeIcon />}
                    </InputGroupButton>
                  </TooltipTrigger>
                  <TooltipContent>{shown ? "隐藏" : "显示"}</TooltipContent>
                </Tooltip>
              </InputGroupAddon>
            </InputGroup>
            <Button type="submit" size="sm" disabled={!value.trim() || saving}>
              {saving ? <LoaderCircleIcon className="animate-spin" /> : <CheckCircle2Icon />}
              {saving ? "验证中" : "验证并保存"}
            </Button>
            <Button type="button" variant="ghost" size="sm" disabled={saving} onClick={cancel}>
              <XIcon />
              取消
            </Button>
          </div>
          <div className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
            <ShieldCheckIcon className="size-3.5 shrink-0 text-success" />
            <span>不用勾选任何权限</span>
            <Hint>只读公开仓库；用 Windows 加密保存在本机，优先于环境变量</Hint>
            <span className="flex-1" />
            <Button
              type="button"
              variant="link"
              size="xs"
              className="h-auto p-0"
              onClick={openCreate}
            >
              <ExternalLinkIcon />
              在 GitHub 创建
            </Button>
          </div>
        </form>
      )}
    </div>
  );
}
