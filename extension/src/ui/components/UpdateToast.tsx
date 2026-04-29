// T6 — persistent "new version available" toast.
//
// Mounted alongside <ProgressToasts /> in App.tsx so it shares the same
// bottom-right toast slot. Differs from progress toasts in two ways:
//   - No auto-dismiss: the user must explicitly click ✕ or 다운로드.
//   - Single-instance: only the latest discovered release is shown — a
//     newer version replaces the previous toast in place.
//
// Subscribes to the in-memory `useUpdate` store; the chrome.runtime
// onMessage listener that populates that store lives in App.tsx so this
// component stays purely presentational.

import { useUpdate } from "../store/update";
import { t } from "../utils/i18n";

export function UpdateToast(): JSX.Element | null {
  const available = useUpdate((s) => s.available);
  const dismiss = useUpdate((s) => s.dismiss);

  if (!available) return null;

  const handleDownload = (): void => {
    if (!available.downloadUrl) return;
    // window.open with noopener to prevent the new tab from accessing
    // window.opener — the GitHub release page is third-party.
    window.open(available.downloadUrl, "_blank", "noopener,noreferrer");
  };

  return (
    <div className="progress-toasts" role="status" aria-live="polite">
      <div className="progress-toast state-update-available">
        <div className="progress-toast-header">
          <span className="progress-toast-label">
            {t("update_toast_title", [available.latestVersion])}
          </span>
          <button
            type="button"
            className="progress-toast-close"
            onClick={dismiss}
            aria-label={t("common_close")}
          >
            ×
          </button>
        </div>
        <div className="progress-toast-body">
          <div className="progress-toast-meta">
            <span>
              {t("update_toast_current_version", [available.currentVersion])}
            </span>
          </div>
          {available.releaseNotes && (
            <div
              className="progress-toast-path"
              title={available.releaseNotes}
            >
              {available.releaseNotes.slice(0, 120)}
              {available.releaseNotes.length > 120 ? "…" : ""}
            </div>
          )}
          {available.downloadUrl && (
            <button
              type="button"
              className="progress-toast-cancel"
              onClick={handleDownload}
            >
              {t("update_toast_download_button")}
            </button>
          )}
        </div>
      </div>
    </div>
  );
}
