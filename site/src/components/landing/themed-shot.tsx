// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import type { CSSProperties } from "react";
import screens from "@/content/screens.json";

// Screenshots come from `make site-screens`: real runs of Sonde Desktop,
// both themes, WebP with content-hashed names (site/public/screens). The
// manifest is src/content/screens.json.

type Variant = { w: number; src: string };
type Shot = { width: number; height: number; dark: Variant[]; light: Variant[]; crop?: { x: number; y: number; w: number; h: number } };
export type ShotId = keyof typeof screens.shots;

const shots = screens.shots as Record<string, Shot>;
const srcSet = (v: Variant[]) => v.map((x) => `${x.src} ${x.w}w`).join(", ");
const largest = (v: Variant[]) => v.reduce((a, b) => (b.w > a.w ? b : a));

/**
 * Both themes of a screenshot; CSS shows the one matching html[data-theme].
 * The hidden one is never fetched: only `eager` (the dark hero) loads before
 * it is shown, every other image is lazy.
 */
export function ThemedShot({ id, alt, altLight, eager = false, sizes }: { id: ShotId; alt: string; altLight?: string; eager?: boolean; sizes: string }) {
  const shot = shots[id];
  const common = { width: shot.width, height: shot.height, sizes, decoding: "async" as const };
  return (
    <>
      <img
        className="shot-dark"
        src={largest(shot.dark).src}
        srcSet={srcSet(shot.dark)}
        alt={alt}
        {...common}
        {...(eager ? { fetchPriority: "high" as const, loading: "eager" as const } : { loading: "lazy" as const })}
      />
      <img className="shot-light" src={largest(shot.light).src} srcSet={srcSet(shot.light)} alt={altLight ?? alt} loading="lazy" {...common} />
    </>
  );
}

/**
 * A cropped region of a screenshot as a fluid background (role="img"): the
 * background size and position scale with the box, so the crop holds at any
 * width. The image is the theme's largest variant, set through CSS variables
 * so only the visible theme's file is fetched.
 */
export function ShotCrop({ id, label }: { id: ShotId; label: string }) {
  const shot = shots[id];
  const c = shot.crop;
  if (!c) throw new Error(`screens.json: ${id} has no crop`);
  const pct = (n: number) => `${(n * 100).toFixed(2)}%`;
  const style = {
    aspectRatio: `${c.w} / ${c.h}`,
    backgroundSize: `${pct(shot.width / c.w)} auto`,
    backgroundPosition: `${pct(c.w === shot.width ? 0 : c.x / (shot.width - c.w))} ${pct(c.h === shot.height ? 0 : c.y / (shot.height - c.h))}`,
    "--shot-dark": `url("${largest(shot.dark).src}")`,
    "--shot-light": `url("${largest(shot.light).src}")`,
  } as CSSProperties;
  return <div className="shot-crop" role="img" aria-label={label} style={style} />;
}
