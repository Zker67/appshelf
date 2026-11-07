// The action at the end of a list row of apps: adding one to the catalog,
// or opening one that is in it.

import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { ArrowRightIcon, CheckIcon, LoaderCircleIcon, PlusIcon } from "lucide-react";

/** InShelfButton tells an app is in the catalog, and opens it there. */
export function InShelfButton({ onClick }: { onClick: () => void }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          size="xs"
          variant="ghost"
          className="group/shelf border border-primary/20 bg-primary/[0.07] text-primary hover:border-primary/35 hover:bg-primary/[0.13] hover:text-primary dark:bg-primary/[0.12] dark:hover:bg-primary/20"
          onClick={(e) => {
            e.stopPropagation();
            onClick();
          }}
        >
          <CheckIcon />
          已在清单
          <ArrowRightIcon className="-ml-0.5 opacity-60 transition-transform duration-150 group-hover/shelf:translate-x-0.5 group-hover/shelf:opacity-100" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>在我的清单中查看</TooltipContent>
    </Tooltip>
  );
}

/** AddButton adds an app to the catalog, saying where it goes. */
export function AddButton({
  label = "加入",
  adding,
  hint,
  onClick,
}: {
  label?: string;
  adding: boolean;
  hint: string;
  onClick: () => void;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          size="xs"
          variant="outline"
          disabled={adding}
          className="transition-colors group-hover/row:border-primary/40 group-hover/row:text-primary"
          onClick={(e) => {
            e.stopPropagation();
            onClick();
          }}
        >
          {adding ? <LoaderCircleIcon className="animate-spin" /> : <PlusIcon />}
          {label}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{hint}</TooltipContent>
    </Tooltip>
  );
}
