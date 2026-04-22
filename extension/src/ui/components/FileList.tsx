import { useEffect, useRef } from "react";
import type { Entry } from "../../types/shared";
import { useExplorerStore } from "../store/explorer";
import { formatBytes, formatTime } from "../utils/format";

function iconFor(entry: Entry): string {
  if (entry.type === "symlink") return "🔗";
  if (entry.type === "directory") return "📁";
  return "📄";
}

function sizeFor(entry: Entry): string {
  if (entry.type === "directory") return "—";
  return formatBytes(entry.sizeBytes);
}

export function FileList(): JSX.Element {
  const entries = useExplorerStore((s) => s.entries);
  const selectedIndex = useExplorerStore((s) => s.selectedIndex);
  const setSelectedIndex = useExplorerStore((s) => s.setSelectedIndex);
  const navigate = useExplorerStore((s) => s.navigate);
  const loading = useExplorerStore((s) => s.loading);
  const currentPath = useExplorerStore((s) => s.currentPath);

  const selectedRowRef = useRef<HTMLTableRowElement | null>(null);

  // Keep the selected row visible when arrow-key navigating.
  useEffect(() => {
    selectedRowRef.current?.scrollIntoView({ block: "nearest" });
  }, [selectedIndex]);

  function onRowActivate(entry: Entry): void {
    if (entry.type === "directory") {
      void navigate(entry.path);
    }
    // files: no-op for Phase 1
  }

  if (loading && entries.length === 0) {
    return <div className="filelist-empty">Loading…</div>;
  }

  if (currentPath !== null && entries.length === 0) {
    return <div className="filelist-empty">Empty directory</div>;
  }

  return (
    <div className="filelist">
      <table className="filelist-table">
        <thead>
          <tr>
            <th className="col-name">Name</th>
            <th className="col-size">Size</th>
            <th className="col-modified">Modified</th>
          </tr>
        </thead>
        <tbody>
          {entries.map((entry, i) => {
            const selected = i === selectedIndex;
            const hidden = entry.hidden === true;
            return (
              <tr
                key={entry.path}
                ref={selected ? selectedRowRef : null}
                className={[
                  "filelist-row",
                  selected ? "filelist-row-selected" : "",
                  hidden ? "filelist-row-hidden" : "",
                  entry.type === "directory" ? "filelist-row-dir" : ""
                ]
                  .filter(Boolean)
                  .join(" ")}
                onClick={() => setSelectedIndex(i)}
                onDoubleClick={() => onRowActivate(entry)}
              >
                <td className="col-name">
                  <button
                    type="button"
                    className="filelist-name"
                    onClick={(e) => {
                      // Single click selects; keep the parent click handler
                      // for visual selection and use name click to activate
                      // directories (matches Windows single-click-to-open feel).
                      e.stopPropagation();
                      setSelectedIndex(i);
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
                </td>
                <td className="col-size">{sizeFor(entry)}</td>
                <td className="col-modified">{formatTime(entry.modifiedTs)}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
