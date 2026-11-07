import { cn } from "@/lib/utils";
import { iconURL, useAbout } from "@/lib/api";
import { hue, initial } from "@/lib/text";
import { useEffect, useRef, useState } from "react";

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
  const baseSrc = iconURL(about?.iconBase, path);
  // A new URL gets new state. Activity resumes effects while preserving
  // state and DOM, so an effect must never reset a completed image to loading.
  return (
    <IconImage key={baseSrc} baseSrc={baseSrc} name={name} size={size} className={className} />
  );
}

export function IconImage({
  baseSrc,
  name,
  size = 36,
  className,
}: {
  baseSrc: string;
  name: string;
  size?: number;
  className?: string;
}) {
  const imageRef = useRef<HTMLImageElement>(null);
  const [attempt, setAttempt] = useState(0);
  const src =
    baseSrc && attempt > 0
      ? `${baseSrc}${baseSrc.includes("?") ? "&" : "?"}retry=${attempt}`
      : baseSrc;
  const [state, setState] = useState<"loading" | "ok" | "failed">(src ? "loading" : "failed");
  useEffect(() => {
    // Cached images may complete before effects resume after Activity hides
    // and restores a page. Reconcile from the actual element; never reset it.
    const image = imageRef.current;
    if (image?.complete && image.currentSrc) {
      setState(image.naturalWidth > 0 ? "ok" : "failed");
    }
  }, [src]);
  useEffect(() => {
    if (state !== "failed" || !baseSrc || attempt >= 3) return;
    const timer = setTimeout(() => {
      setAttempt((value) => value + 1);
      setState("loading");
    }, [2000, 10000, 120000][attempt]);
    return () => clearTimeout(timer);
  }, [state, baseSrc, attempt]);
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
            ref={imageRef}
            key={src}
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
