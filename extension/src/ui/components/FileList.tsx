import { useEffect, useRef, useState } from "react";
import type { Entry } from "../../types/shared";
import {
  useExplorerStore,
  type ColumnOrder,
  type SortableField
} from "../store/explorer";
import { useClipboard } from "../store/clipboard";
import { formatBytes, formatTime } from "../utils/format";

/**
 * FileList — 3-column virtualised-ish listing.
 *
 * Phase 2.2 upgrades:
 *  - Clickable headers drive server-side sort (store.setSort).
 *  - Right-edge resizer handle on every header (window-level mousemove/mouseup
 *    for smooth dragging even when the cursor leaves the header cell).
 *  - HTML5 drag-and-drop reorders columns; the resizer handle opts out of
 *    drag (`draggable={false}` + stopPropagation on its mousedown) so the
 *    resize gesture isn't hijacked.
 *
 * Layout uses div-grid rather than a `<table>` because per-column widths must
 * be runtime-driven and the previous CSS used `max-width: 0` + `table-layout:
 * auto` which makes pixel-accurate width control awkward.
 */

const COLUMN_LABELS: Record<SortableField, string> = {
  name: "Name",
  size: "Size",
  modified: "Modified"
};

const MIN_WIDTHS: Record<SortableField, number> = {
  name: 80,
  size: 60,
  modified: 120
};

function iconFor(entry: Entry): string {
  if (entry.type === "symlink") return "🔗";
  if (entry.type === "directory") return "📁";
  return "📄";
}

function sizeFor(entry: Entry): string {
  if (entry.type === "directory") return "—";
  return formatBytes(entry.sizeBytes);
}

/**
 * Move `from` to land on the `side` of `to`. `side="before"` inserts at the
 * target's index (pushing target right); `side="after"` inserts at the next
 * index. Without `side`, drops on the right half of a header would silently
 * no-op because `from` was already adjacent-before. Example:
 * `["name","size","modified"]`, from=size, to=modified, side="after" yields
 * `["name","modified","size"]`.
 */
function reorderColumns(
  order: ColumnOrder,
  from: SortableField,
  to: SortableField,
  side: "before" | "after"
): ColumnOrder {
  if (from === to) return order;
  const without = order.filter((c) => c !== from);
  let idx = without.indexOf(to);
  if (idx < 0) return order;
  if (side === "after") idx += 1;
  const next = [...without];
  next.splice(idx, 0, from);
  return next;
}

type ResizeState = {
  col: SortableField;
  startX: number;
  startWidth: number;
};

type DragHint = {
  over: SortableField;
  side: "before" | "after";
};

// Props let App.tsx own the global context-menu state (x/y/variant) while
// FileList stays focused on rendering + row interaction. Both callbacks are
// required so FileList never needs to check "is this prop wired?" at runtime.
export interface FileListProps {
  onContextMenuRow: (
    entry: Entry,
    index: number,
    clientX: number,
    clientY: number
  ) => void;
  onContextMenuBlank: (clientX: number, clientY: number) => void;
}

export function FileList({
  onContextMenuRow,
  onContextMenuBlank
}: FileListProps): JSX.Element {
  const entries = useExplorerStore((s) => s.entries);
  const selectedIndices = useExplorerStore((s) => s.selectedIndices);
  const lastAnchorIndex = useExplorerStore((s) => s.lastAnchorIndex);
  const selectOnly = useExplorerStore((s) => s.selectOnly);
  const selectRange = useExplorerStore((s) => s.selectRange);
  const toggleSelect = useExplorerStore((s) => s.toggleSelect);
  const navigate = useExplorerStore((s) => s.navigate);
  const openEntry = useExplorerStore((s) => s.openEntry);
  const loading = useExplorerStore((s) => s.loading);
  const currentPath = useExplorerStore((s) => s.currentPath);
  const sortField = useExplorerStore((s) => s.sortField);
  const sortOrder = useExplorerStore((s) => s.sortOrder);
  const columnWidths = useExplorerStore((s) => s.columnWidths);
  const columnOrder = useExplorerStore((s) => s.columnOrder);
  const setSort = useExplorerStore((s) => s.setSort);
  const setColumnWidth = useExplorerStore((s) => s.setColumnWidth);
  const setColumnOrder = useExplorerStore((s) => s.setColumnOrder);

  // Cut-mode tint: rows whose absolute path sits in the clipboard while mode
  // === "cut" render at 50% opacity. Subscribing here (rather than in App)
  // keeps the row-list localised and avoids an extra prop drill.
  const clipMode = useClipboard((s) => s.mode);
  const clipPaths = useClipboard((s) => s.paths);

  const selectedRowRef = useRef<HTMLDivElement | null>(null);

  const [resizing, setResizing] = useState<ResizeState | null>(null);
  const [dragCol, setDragCol] = useState<SortableField | null>(null);
  const [dragHint, setDragHint] = useState<DragHint | null>(null);

  // Keep the primary (anchor) row visible when arrow-key navigating. With
  // multi-select we pick the anchor — that's the row the user is actively
  // steering. Falls back to the smallest selected index if no anchor.
  useEffect(() => {
    selectedRowRef.current?.scrollIntoView({ block: "nearest" });
  }, [lastAnchorIndex]);

  // Window-level resize tracking — attaching to the handle itself would stop
  // firing the moment the cursor leaves the handle's 6px hit area.
  useEffect(() => {
    if (!resizing) return;
    const onMove = (e: MouseEvent): void => {
      const delta = e.clientX - resizing.startX;
      const next = Math.max(
        MIN_WIDTHS[resizing.col],
        resizing.startWidth + delta
      );
      setColumnWidth(resizing.col, next);
    };
    const onUp = (): void => setResizing(null);
    window.addEventListener("mousemove", onMove);
    window.addEventListener("mouseup", onUp);
    return (): void => {
      window.removeEventListener("mousemove", onMove);
      window.removeEventListener("mouseup", onUp);
    };
  }, [resizing, setColumnWidth]);

  // Safety net for abandoned header drags. Chrome won't fire React's
  // `onDragEnd` reliably when the user cancels with ESC or drops outside the
  // window, which leaves `dragCol`/`dragHint` stale and poisons the next
  // dragover. Window-level listeners catch every terminal event; mouseup is
  // belt-and-braces for browsers that occasionally miss dragend.
  useEffect(() => {
    const cleanup = (): void => {
      setDragCol(null);
      setDragHint(null);
    };
    window.addEventListener("dragend", cleanup);
    window.addEventListener("drop", cleanup);
    window.addEventListener("mouseup", cleanup);
    return (): void => {
      window.removeEventListener("dragend", cleanup);
      window.removeEventListener("drop", cleanup);
      window.removeEventListener("mouseup", cleanup);
    };
  }, []);

  function onRowActivate(entry: Entry): void {
    if (entry.type === "directory") {
      void navigate(entry.path);
      return;
    }
    // Phase 2.1: files open via OS default handler.
    void openEntry(entry.path);
  }

  // Click dispatch table.
  //   bare click  → single-select (clears everything else).
  //   Shift+click → range from anchor to i (replaces prior selection).
  //   Ctrl/⌘+click → toggle i in the set; anchor jumps to i.
  // Shared by the row-level onClick and the Name-cell button.
  function applyClickSelection(
    e: { shiftKey: boolean; ctrlKey: boolean; metaKey: boolean },
    i: number
  ): void {
    if (e.shiftKey) {
      selectRange(i);
    } else if (e.ctrlKey || e.metaKey) {
      toggleSelect(i);
    } else {
      selectOnly(i);
    }
  }

  function startResize(
    col: SortableField,
    clientX: number,
    currentWidth: number
  ): void {
    setResizing({ col, startX: clientX, startWidth: currentWidth });
  }

  function handleHeaderClick(col: SortableField): void {
    void setSort(col);
  }

  function onDragStartHeader(
    e: React.DragEvent<HTMLDivElement>,
    col: SortableField
  ): void {
    setDragCol(col);
    e.dataTransfer.effectAllowed = "move";
    // Required for Firefox to actually fire dragover/drop.
    try {
      e.dataTransfer.setData("text/x-column", col);
    } catch {
      // Some browsers throw on unknown MIME types; ignore — state is the
      // source of truth.
    }
  }

  function onDragOverHeader(
    e: React.DragEvent<HTMLDivElement>,
    col: SortableField
  ): void {
    if (!dragCol || dragCol === col) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = "move";
    // Hint which side of the target we'll drop on: left half = before,
    // right half = after. Purely cosmetic (the drop logic inserts at target).
    const rect = e.currentTarget.getBoundingClientRect();
    const side: DragHint["side"] =
      e.clientX - rect.left < rect.width / 2 ? "before" : "after";
    setDragHint({ over: col, side });
  }

  function onDragLeaveHeader(col: SortableField): void {
    if (dragHint && dragHint.over === col) setDragHint(null);
  }

  function onDropHeader(
    e: React.DragEvent<HTMLDivElement>,
    col: SortableField
  ): void {
    e.preventDefault();
    if (!dragCol || dragCol === col) {
      setDragCol(null);
      setDragHint(null);
      return;
    }
    // Respect the visual hint's side so a "drop on right half" actually lands
    // after the target. Fallback to "before" if the hint was cleared by a
    // quick drop without a preceding dragover.
    const side = dragHint?.side ?? "before";
    const next = reorderColumns(columnOrder, dragCol, col, side);
    setColumnOrder(next);
    setDragCol(null);
    setDragHint(null);
  }

  function onDragEndHeader(): void {
    setDragCol(null);
    setDragHint(null);
  }

  // Handler for right-click on a row. We keep the existing selection if the
  // row is already part of it (matches Windows Explorer: right-clicking a
  // selected file targets all selected files), otherwise we single-select
  // the clicked row before opening the menu so multi-select actions have a
  // sensible target set.
  function onRowContextMenu(
    e: React.MouseEvent<HTMLDivElement>,
    entry: Entry,
    i: number
  ): void {
    e.preventDefault();
    e.stopPropagation();
    if (!selectedIndices.has(i)) {
      selectOnly(i);
    }
    onContextMenuRow(entry, i, e.clientX, e.clientY);
  }

  function onBlankContextMenu(e: React.MouseEvent<HTMLDivElement>): void {
    // Only fire when the user right-clicks on the list background, not on a
    // row that bubbled up. Row handler already calls stopPropagation.
    e.preventDefault();
    onContextMenuBlank(e.clientX, e.clientY);
  }

  if (loading && entries.length === 0) {
    return (
      <div className="filelist" onContextMenu={onBlankContextMenu}>
        <div className="filelist-empty">Loading…</div>
      </div>
    );
  }

  if (currentPath !== null && entries.length === 0) {
    return (
      <div className="filelist" onContextMenu={onBlankContextMenu}>
        <div className="filelist-empty">Empty directory</div>
      </div>
    );
  }

  // Build the grid template from the columnOrder + columnWidths. The LAST
  // column flexes to fill remaining horizontal space so the list never leaves
  // a dead stripe on wide viewports.
  const gridTemplate = columnOrder
    .map((col, i) => {
      const isLast = i === columnOrder.length - 1;
      const w = columnWidths[col];
      return isLast ? `minmax(${w}px, 1fr)` : `${w}px`;
    })
    .join(" ");

  function renderCell(col: SortableField, entry: Entry, i: number): JSX.Element {
    const textAlignClass = col === "size" ? "col-size" : "";
    if (col === "name") {
      return (
        <div key={col} className={`filelist-cell col-name ${textAlignClass}`}>
          <button
            type="button"
            className="filelist-name"
            onClick={(e) => {
              e.stopPropagation();
              // Modifier keys mean "select only, don't open" — matches
              // native file-manager UX where Ctrl+Click never activates.
              if (e.shiftKey || e.ctrlKey || e.metaKey) {
                applyClickSelection(e, i);
                return;
              }
              selectOnly(i);
              onRowActivate(entry);
            }}
            title={entry.path}
          >
            <span className="filelist-icon">{iconFor(entry)}</span>
            <span className="filelist-label">{entry.name}</span>
            {entry.readOnly ? (
              <span className="filelist-ro" title="read-only">
                🔒
              </span>
            ) : null}
          </button>
        </div>
      );
    }
    if (col === "size") {
      return (
        <div key={col} className="filelist-cell col-size">
          {sizeFor(entry)}
        </div>
      );
    }
    return (
      <div key={col} className="filelist-cell col-modified">
        {formatTime(entry.modifiedTs)}
      </div>
    );
  }

  return (
    <div className="filelist" onContextMenu={onBlankContextMenu}>
      <div
        className="filelist-head"
        style={{ gridTemplateColumns: gridTemplate }}
      >
        {columnOrder.map((col) => {
          const active = sortField === col;
          const indicator = active ? (sortOrder === "asc" ? " ▲" : " ▼") : "";
          const dropClass =
            dragHint && dragHint.over === col
              ? dragHint.side === "before"
                ? "drop-before"
                : "drop-after"
              : "";
          return (
            <div
              key={col}
              className={[
                "filelist-header",
                active ? "filelist-header-active" : "",
                dropClass
              ]
                .filter(Boolean)
                .join(" ")}
              draggable
              onDragStart={(e) => onDragStartHeader(e, col)}
              onDragOver={(e) => onDragOverHeader(e, col)}
              onDragLeave={() => onDragLeaveHeader(col)}
              onDrop={(e) => onDropHeader(e, col)}
              onDragEnd={onDragEndHeader}
            >
              <button
                type="button"
                className="filelist-header-button"
                onClick={() => handleHeaderClick(col)}
              >
                {COLUMN_LABELS[col]}
                {indicator}
              </button>
              <div
                className="filelist-header-resizer"
                draggable={false}
                onMouseDown={(e) => {
                  // stopPropagation prevents the parent header's drag gesture
                  // from starting, preventDefault suppresses text selection.
                  e.preventDefault();
                  e.stopPropagation();
                  startResize(col, e.clientX, columnWidths[col]);
                }}
              />
            </div>
          );
        })}
      </div>
      <div className="filelist-body">
        {entries.map((entry, i) => {
          const selected = selectedIndices.has(i);
          // Attach scrollIntoView ref to the anchor row (primary focus). If
          // no anchor yet, attach to the first selected row so reveal still
          // works after selectAll / initial click.
          const isScrollTarget =
            lastAnchorIndex >= 0 ? i === lastAnchorIndex : selected;
          const hidden = entry.hidden === true;
          const cutPending =
            clipMode === "cut" && clipPaths.includes(entry.path);
          return (
            <div
              key={entry.path}
              ref={isScrollTarget ? selectedRowRef : null}
              className={[
                "filelist-row",
                selected ? "filelist-row-selected" : "",
                hidden ? "filelist-row-hidden" : "",
                entry.type === "directory" ? "filelist-row-dir" : "",
                cutPending ? "cut-pending" : ""
              ]
                .filter(Boolean)
                .join(" ")}
              style={{ gridTemplateColumns: gridTemplate }}
              onClick={(e) => applyClickSelection(e, i)}
              onDoubleClick={() => onRowActivate(entry)}
              onContextMenu={(e) => onRowContextMenu(e, entry, i)}
            >
              {columnOrder.map((col) => renderCell(col, entry, i))}
            </div>
          );
        })}
      </div>
    </div>
  );
}
