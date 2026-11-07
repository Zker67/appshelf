// The icons of the pages, drawn for them. index.css animates their
// parts: once on hover, in a loop while the page is selected.

import type { SVGProps } from "react";

const base: SVGProps<SVGSVGElement> = {
  viewBox: "0 0 24 24",
  fill: "none",
  stroke: "currentColor",
  strokeWidth: 1.7,
  strokeLinecap: "round",
  strokeLinejoin: "round",
  "aria-hidden": true,
};

/** ShelfIcon: a book and boxes on a shelf, the top box floating. */
export function ShelfIcon(props: SVGProps<SVGSVGElement>) {
  return (
    <svg {...base} {...props} className={`nav-icon nav-icon-shelf ${props.className ?? ""}`}>
      <path d="M2.5 20.5h19" />
      <rect className="box-a" x="4.5" y="6.5" width="3.6" height="14" rx="1" />
      <rect className="box-b" x="10" y="11.5" width="7" height="9" rx="1.3" />
      <rect
        className="box-c"
        x="11.5"
        y="5"
        width="4"
        height="4"
        rx="1"
        fill="currentColor"
        fillOpacity="0.18"
      />
      <path d="M13.5 14.2v2" opacity="0.6" />
    </svg>
  );
}

/** DiscoverIcon: a lens and a spark. */
export function DiscoverIcon(props: SVGProps<SVGSVGElement>) {
  return (
    <svg {...base} {...props} className={`nav-icon nav-icon-discover ${props.className ?? ""}`}>
      <g className="lens">
        <circle cx="10.5" cy="11" r="6" />
        <path d="M15 15.5l5 5" />
        <path d="M7.8 9.4a3.2 3.2 0 0 1 2.2-1.6" opacity="0.55" />
      </g>
      <path
        className="spark"
        d="M19 2.6l.8 1.9 1.9.8-1.9.8-.8 1.9-.8-1.9-1.9-.8 1.9-.8z"
        fill="currentColor"
        stroke="none"
      />
    </svg>
  );
}

/** InstalledIcon: a screen with a check. */
export function InstalledIcon(props: SVGProps<SVGSVGElement>) {
  return (
    <svg {...base} {...props} className={`nav-icon nav-icon-installed ${props.className ?? ""}`}>
      <rect
        className="glow"
        x="4.6"
        y="5.6"
        width="14.8"
        height="9.3"
        rx="1"
        fill="currentColor"
        stroke="none"
      />
      <rect x="3" y="4" width="18" height="12.5" rx="2" />
      <path d="M12 16.5v3.5M8.5 20.5h7" />
      <path className="check" d="M8.6 10.4l2.3 2.3 4.6-4.6" />
    </svg>
  );
}

/** UpdatesIcon: an arrow rising out of a ring. */
export function UpdatesIcon(props: SVGProps<SVGSVGElement>) {
  return (
    <svg {...base} {...props} className={`nav-icon nav-icon-updates ${props.className ?? ""}`}>
      <path className="ring" d="M19.4 13.6A7.6 7.6 0 1 1 15.8 5.2" />
      <path className="ring-tip" d="M16.4 2.8l-.6 2.6 2.6.6" />
      <g className="arrow">
        <path d="M12 16V8.6" />
        <path d="M9 11.4l3-3 3 3" />
      </g>
    </svg>
  );
}
