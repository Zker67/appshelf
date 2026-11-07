// Detection rules proposed for an app of a GitHub repository: of the
// programs installed that look like it, and of the repository's name.

import { InputGroupButton } from "@/components/ui/input-group";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Discover, type RuleSuggestion } from "@/mygo";
import { useQuery } from "@tanstack/react-query";
import { ChevronDownIcon, LoaderCircleIcon, MonitorCheckIcon, ScanSearchIcon } from "lucide-react";

/** useRuleSuggestions reads the rules proposed for a repository. */
export function useRuleSuggestions(repo: string) {
  const valid = /^[\w.-]+\/[\w.-]+$/.test(repo);
  return useQuery({
    // Under the installed list's key: read again when it changes.
    queryKey: ["installed", "rules", repo.toLowerCase()],
    queryFn: () => Discover.suggestRules(repo),
    enabled: valid,
  });
}

/** suggestionLabel names what a rule finds. */
export function suggestionLabel(s: RuleSuggestion): string {
  return s.from ? `${s.from}${s.version ? ` ${s.version}` : ""}` : "按仓库名";
}

/** RuleSuggestions is the menu of rules proposed, at the end of a rule field. */
export function RuleSuggestions({
  repo,
  onPick,
}: {
  repo: string;
  onPick: (rule: string) => void;
}) {
  const q = useRuleSuggestions(repo);
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <InputGroupButton size="xs" className="text-muted-foreground" disabled={!q.data?.length}>
          {q.isFetching && <LoaderCircleIcon className="animate-spin" />}
          建议
          <ChevronDownIcon />
        </InputGroupButton>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-96">
        <DropdownMenuLabel className="text-xs text-muted-foreground">
          选择一条规则
        </DropdownMenuLabel>
        {q.data?.map((s) => (
          <DropdownMenuItem
            key={s.rule}
            onSelect={() => onPick(s.rule)}
            className="flex-col items-start gap-0.5"
          >
            <span className="flex items-center gap-1.5 text-sm">
              {s.from ? (
                <MonitorCheckIcon className="text-success" />
              ) : (
                <ScanSearchIcon className="text-muted-foreground" />
              )}
              {suggestionLabel(s)}
            </span>
            <span className="pl-6 font-mono text-xs break-all text-muted-foreground">{s.rule}</span>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
