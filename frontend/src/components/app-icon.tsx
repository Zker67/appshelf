import { cn } from "@/lib/utils";
import { iconURL, useAbout } from "@/lib/api";
import { hue, initial } from "@/lib/text";
import { useEffect, useState } from "react";

/**
 * AppIcon shows a program's icon, which Go finds and serves; while it
 * loads a soft tile holds its place, and without one a letter stands in.
 */
export function AppIcon({
  path,
  name,
  size = 36,
  className,
}: {
  path: string;
  name: string;
  size?: number;
  className?: string;
}) {
  const { data: about } = useAbout();
  const src = iconURL(about?.iconBase, path);
  const [state, setState] = useState<"loading" | "ok" | "failed">(src ? "loading" : "failed");
  useEffect(() => setState(src ? "loading" : "failed"), [src]);
  const h = hue(name);
  return (
    <div
      className={cn("relative shrink-0 select-none", className)}
      style={{ width: size, height: size }}
      aria-hidden
    >
      {state === "failed" ? (
        <div
          className="anim-fade flex size-full items-center justify-center rounded-[26%] font-semibold text-white shadow-[inset_0_1px_0_rgb(255_255_255/0.25)]"
          style={{
            fontSize: size * 0.44,
            background: `linear-gradient(145deg, oklch(0.66 0.13 ${h}), oklch(0.5 0.16 ${h + 18}))`,
          }}
        >
          {initial(name)}
        </div>
      ) : (
        <>
          {state === "loading" && <div className="absolute inset-0 rounded-[26%] bg-muted" />}
          <img
            src={src}
            alt=""
            loading="lazy"
            decoding="async"
            draggable={false}
            onLoad={() => setState("ok")}
            onError={() => setState("failed")}
            className={cn(
              "absolute inset-0 size-full object-contain transition-[opacity,transform] duration-300 ease-out",
              state === "ok" ? "scale-100 opacity-100" : "scale-90 opacity-0",
            )}
          />
        </>
      )}
    </div>
  );
}
