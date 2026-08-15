import Image from "next/image";

import { cn } from "@/lib/utils";

/**
 * The Warder logo.
 *
 * Two forms: the full lockup and the mark on its own: each shipped as a dark
 * ("ink") and a light ("paper") artwork. Both are rendered and one is hidden by
 * the colour scheme, rather than swapping a `src` at runtime: a swap would
 * flash the wrong artwork on first paint, and in a dark interface that means a
 * black logo on a black header.
 *
 * ## Why these components do arithmetic
 *
 * The source files carry transparent padding around the artwork, 12% either
 * side of the lockup, 24.5% above and below it. Dropped into a flexbox as-is,
 * an element whose visible logo is 20px tall occupies 39px and sits 19px
 * further from the edge than everything aligned beneath it, which reads as a
 * misaligned header rather than as padding.
 *
 * So the components take the height the *ink* should be, size the image to
 * suit, and pull the padding back off with negative margins. The layout box
 * that results is exactly the artwork. Measurements below are from the files
 * themselves; if the artwork is regenerated with different padding, re-measure.
 */

/** Proportions of the lockup file, measured from its alpha channel. */
const LOCKUP = {
  aspect: 4, // 2400 × 600
  inkHeight: 0.51,
  padX: 0.12,
  padY: 0.245,
};

/** Proportions of the square mark file. */
const MARK = {
  inkHeight: 0.691,
  padX: 0.131,
  padTop: 0.162,
  padBottom: 0.146,
};

/**
 * The full lockup: mark and wordmark.
 *
 * @param height how tall the visible artwork should be, in pixels.
 */
export function Wordmark({
  height = 20,
  className,
  priority = false,
}: {
  height?: number;
  className?: string;
  priority?: boolean;
}) {
  const imageHeight = Math.round(height / LOCKUP.inkHeight);
  const imageWidth = Math.round(imageHeight * LOCKUP.aspect);

  /*
   * The width and height passed to next/image are the rendered size, not the
   * file's 2400×600. They drive the generated srcset, and declaring the
   * intrinsic size made the header preload a 3840px-wide copy of a logo that
   * paints at 157px.
   */
  const style = {
    marginInline: -Math.round(imageWidth * LOCKUP.padX),
    marginBlock: -Math.round(imageHeight * LOCKUP.padY),
  } as const;

  return (
    <span className={cn("inline-flex shrink-0 items-center", className)}>
      <Image
        src="/warder-lockup-2400x600-ink.png"
        alt="Warder"
        width={imageWidth}
        height={imageHeight}
        priority={priority}
        style={style}
        className="dark:hidden"
      />
      <Image
        src="/warder-lockup-2400x600-paper.png"
        alt=""
        aria-hidden="true"
        width={imageWidth}
        height={imageHeight}
        priority={priority}
        style={style}
        className="hidden dark:block"
      />
    </span>
  );
}

/**
 * The mark on its own, for places too narrow for the lockup, a collapsed
 * sidebar, a footer, an error page.
 *
 * @param size how tall the visible artwork should be, in pixels.
 */
export function Mark({
  size = 20,
  className,
  priority = false,
}: {
  size?: number;
  className?: string;
  priority?: boolean;
}) {
  const imageSize = Math.round(size / MARK.inkHeight);

  // As above: the rendered size, so the srcset is sized for what paints.
  const style = {
    marginInline: -Math.round(imageSize * MARK.padX),
    marginTop: -Math.round(imageSize * MARK.padTop),
    marginBottom: -Math.round(imageSize * MARK.padBottom),
  } as const;

  return (
    <span className={cn("inline-flex shrink-0 items-center", className)}>
      <Image
        src="/warder-mark-1024-ink.png"
        alt="Warder"
        width={imageSize}
        height={imageSize}
        priority={priority}
        style={style}
        className="dark:hidden "
      />
      <Image
        src="/warder-mark-1024-paper.png"
        alt=""
        aria-hidden="true"
        width={imageSize}
        height={imageSize}
        priority={priority}
        style={style}
        className="hidden dark:block"
      />
    </span>
  );
}
