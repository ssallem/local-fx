import { useEffect, useLayoutEffect, useRef, useState } from "react";

/**
 * ContextMenu — a lightweight floating menu anchored at (x, y) in viewport
 * coordinates.
 *
 * Why not a Portal? The explorer is a single-tree app with a fixed z-index
 * stack; plain `position: fixed` on the root element sits above all siblings
 * (toolbar/filelist/statusbar) because .context-menu carries z-index: 200.
 * Introducing a Portal would add a tree-shape exception that breaks the
 * existing Escape-key/backdrop pattern used by dialogs.
 *
 * Why window-level mousedown capture for click-outside? React's onClick fires
 * on the TARGET of the click, but bubbled events stop at the menu root. A
 * capture-phase window listener sees every mousedown regardless of the DOM
 * path, and mousedown (not click) is the right trigger because a down-event
 * outside + up inside shouldn't leave the menu open.
 */

export interface ContextMenuItem {
  label: string;
  onClick: () => void;
  disabled?: boolean;
  danger?: boolean;
  separator?: boolean;
  shortcut?: string;
}

interface Props {
  x: number;
  y: number;
  items: ContextMenuItem[];
  onClose: () => void;
}

// Rough per-row height used for viewport-edge repositioning. A little larger
// than the actual 30px so separators (4px bar) don't push us off screen.
const APPROX_ROW_HEIGHT = 32;
const MENU_WIDTH = 220;
const VIEWPORT_MARGIN = 8;

// First selectable item in `items` starting at `from` in direction `dir` (±1).
// Skips separators and disabled rows so keyboard nav never lands on a dead row.
function nextSelectable(
  items: ContextMenuItem[],
  from: number,
  dir: 1 | -1
): number {
  const n = items.length;
  if (n === 0) return -1;
  // Walk at most n steps so a menu of only separators terminates cleanly
  // instead of looping forever.
  let idx = from;
  for (let step = 0; step < n; step += 1) {
    idx = (idx + dir + n) % n;
    const it = items[idx];
    if (it && !it.separator && !it.disabled) return idx;
  }
  return -1;
}

export function ContextMenu({ x, y, items, onClose }: Props): JSX.Element {
  const rootRef = useRef<HTMLDivElement | null>(null);
  // Seed activeIndex at the first selectable row so "menu opens → Enter"
  // picks a sensible default without extra arrow-key taps.
  const [activeIndex, setActiveIndex] = useState<number>(() =>
    nextSelectable(items, -1, 1)
  );
  // Position after viewport clamp. We defer to an effect so we can measure
  // the real rendered height once — falling back to the approximation for
  // the first paint.
  const [pos, setPos] = useState<{ x: number; y: number }>(() => {
    const rowsHeight = items.length * APPROX_ROW_HEIGHT;
    const clampedX = Math.min(
      Math.max(VIEWPORT_MARGIN, x),
      window.innerWidth - MENU_WIDTH - VIEWPORT_MARGIN
    );
    const clampedY = Math.min(
      Math.max(VIEWPORT_MARGIN, y),
      window.innerHeight - rowsHeight - VIEWPORT_MARGIN
    );
    return { x: clampedX, y: clampedY };
  });

  // Re-clamp once we've mounted and can measure the real DOM height. Without
  // this pass a menu opened near the bottom edge with separators would still
  // overflow because the approximation under-counts.
  useLayoutEffect(() => {
    const el = rootRef.current;
    if (!el) return;
    const rect = el.getBoundingClientRect();
    const clampedX = Math.min(
      Math.max(VIEWPORT_MARGIN, x),
      window.innerWidth - rect.width - VIEWPORT_MARGIN
    );
    const clampedY = Math.min(
      Math.max(VIEWPORT_MARGIN, y),
      window.innerHeight - rect.height - VIEWPORT_MARGIN
    );
    setPos((prev) =>
      prev.x === clampedX && prev.y === clampedY
        ? prev
        : { x: clampedX, y: clampedY }
    );
    // Intentionally only react to props x/y changes — items length is
    // effectively fixed for a single open.
  }, [x, y]);

  // Click-outside + Escape. Using capture on mousedown so we beat any
  // bubbling menu item handlers.
  useEffect(() => {
    function onMouseDown(e: MouseEvent): void {
      const root = rootRef.current;
      if (!root) return;
      if (e.target instanceof Node && root.contains(e.target)) return;
      onClose();
    }
    function onKey(e: KeyboardEvent): void {
      if (e.key === "Escape") {
        e.preventDefault();
        e.stopPropagation();
        onClose();
        return;
      }
      if (e.key === "ArrowDown") {
        e.preventDefault();
        setActiveIndex((cur) => nextSelectable(items, cur, 1));
        return;
      }
      if (e.key === "ArrowUp") {
        e.preventDefault();
        setActiveIndex((cur) => nextSelectable(items, cur, -1));
        return;
      }
      if (e.key === "Enter") {
        e.preventDefault();
        setActiveIndex((cur) => {
          const it = cur >= 0 ? items[cur] : undefined;
          if (it && !it.disabled && !it.separator) {
            it.onClick();
            onClose();
          }
          return cur;
        });
        return;
      }
      if (e.key === "Home") {
        e.preventDefault();
        setActiveIndex(nextSelectable(items, -1, 1));
        return;
      }
      if (e.key === "End") {
        e.preventDefault();
        setActiveIndex(nextSelectable(items, 0, -1));
      }
    }
    window.addEventListener("mousedown", onMouseDown, true);
    window.addEventListener("keydown", onKey, true);
    return (): void => {
      window.removeEventListener("mousedown", onMouseDown, true);
      window.removeEventListener("keydown", onKey, true);
    };
  }, [items, onClose]);

  // Scroll/resize can leave the menu anchored on a stale coordinate. Easiest
  // UX is to close — users can re-open with a fresh right-click.
  useEffect(() => {
    window.addEventListener("scroll", onClose, true);
    window.addEventListener("resize", onClose);
    return (): void => {
      window.removeEventListener("scroll", onClose, true);
      window.removeEventListener("resize", onClose);
    };
  }, [onClose]);

  return (
    <div
      ref={rootRef}
      className="context-menu"
      role="menu"
      style={{ left: pos.x, top: pos.y }}
    >
      {items.map((item, i) => {
        if (item.separator) {
          return (
            // eslint-disable-next-line react/no-array-index-key
            <div key={`sep-${i}`} className="context-menu-separator" />
          );
        }
        const classes = [
          "context-menu-item",
          item.disabled ? "disabled" : "",
          item.danger ? "danger" : "",
          i === activeIndex ? "active" : ""
        ]
          .filter(Boolean)
          .join(" ");
        return (
          <div
            // eslint-disable-next-line react/no-array-index-key
            key={`item-${i}-${item.label}`}
            className={classes}
            role="menuitem"
            aria-disabled={item.disabled ? "true" : "false"}
            onMouseEnter={() => {
              if (!item.disabled) setActiveIndex(i);
            }}
            onClick={() => {
              if (item.disabled) return;
              item.onClick();
              onClose();
            }}
          >
            <span className="context-menu-label">{item.label}</span>
            {item.shortcut ? (
              <span className="context-menu-shortcut">{item.shortcut}</span>
            ) : null}
          </div>
        );
      })}
    </div>
  );
}
