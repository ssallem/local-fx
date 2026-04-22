import type { Drive } from "../../types/shared";
import { useExplorerStore } from "../store/explorer";
import { formatBytes } from "../utils/format";

function DriveRow({ drive }: { drive: Drive }): JSX.Element {
  const navigate = useExplorerStore((s) => s.navigate);
  const currentPath = useExplorerStore((s) => s.currentPath);
  const active = currentPath?.startsWith(drive.path) ?? false;

  const label = drive.label ? `${drive.label} (${drive.path})` : drive.path;
  const capacity =
    drive.totalBytes > 0
      ? `${formatBytes(drive.freeBytes)} free / ${formatBytes(drive.totalBytes)}`
      : drive.fsType;

  return (
    <button
      type="button"
      className={`drive-row${active ? " drive-row-active" : ""}`}
      onClick={() => {
        void navigate(drive.path);
      }}
      title={`${label} — ${drive.fsType}${drive.readOnly ? " (read-only)" : ""}`}
    >
      <div className="drive-row-label">💽 {label}</div>
      <div className="drive-row-meta">{capacity}</div>
    </button>
  );
}

export function Sidebar(): JSX.Element {
  const drives = useExplorerStore((s) => s.drives);

  return (
    <aside className="sidebar">
      <div className="sidebar-section-title">Drives</div>
      <div className="sidebar-list">
        {drives.length === 0 ? (
          <div className="sidebar-empty">No drives</div>
        ) : (
          drives.map((d) => <DriveRow key={d.path} drive={d} />)
        )}
      </div>
      <div className="sidebar-section-title">Browse</div>
      <div className="sidebar-empty">(empty)</div>
    </aside>
  );
}
