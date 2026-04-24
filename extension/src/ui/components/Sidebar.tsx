import type { Drive } from "../../types/shared";
import { useExplorerStore } from "../store/explorer";
import { formatBytes } from "../utils/format";
import { t } from "../utils/i18n";

function DriveRow({ drive }: { drive: Drive }): JSX.Element {
  const navigate = useExplorerStore((s) => s.navigate);
  const currentPath = useExplorerStore((s) => s.currentPath);
  const active = currentPath?.startsWith(drive.path) ?? false;

  const label = drive.label ? `${drive.label} (${drive.path})` : drive.path;
  const capacity =
    drive.totalBytes > 0
      ? t("app_drive_capacity", [
          formatBytes(drive.freeBytes),
          formatBytes(drive.totalBytes)
        ])
      : drive.fsType;

  // Title mixes raw `label`/`fsType` (not translatable — they're data from the
  // Host) with the translated read-only suffix. Building the pieces instead of
  // inlining keeps the concatenation obvious.
  const readOnlySuffix = drive.readOnly
    ? ` (${t("app_drive_read_only_suffix")})`
    : "";

  return (
    <button
      type="button"
      className={`drive-row${active ? " drive-row-active" : ""}`}
      onClick={() => {
        void navigate(drive.path);
      }}
      title={`${label} — ${drive.fsType}${readOnlySuffix}`}
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
      <div className="sidebar-section-title">{t("sidebar_drives")}</div>
      <div className="sidebar-list">
        {drives.length === 0 ? (
          <div className="sidebar-empty">{t("sidebar_no_drives")}</div>
        ) : (
          drives.map((d) => <DriveRow key={d.path} drive={d} />)
        )}
      </div>
      <div className="sidebar-section-title">{t("sidebar_browse")}</div>
      <div className="sidebar-empty">{t("sidebar_browse_empty")}</div>
    </aside>
  );
}
