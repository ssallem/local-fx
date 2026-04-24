import { useExplorerStore } from "../store/explorer";
import type { IpcError } from "../ipc";
import { t } from "../utils/i18n";

function installHint(err: IpcError): string | null {
  if (err.code === "E_HOST_NOT_FOUND") {
    return t("error_hint_host_not_found");
  }
  if (err.details && err.details["mayNeedInstall"] === true) {
    return t("error_hint_may_need_install");
  }
  return null;
}

/**
 * Turn the wire-level `IpcError` into the user-facing sentence.
 *
 * Strategy: prefer a `error_<code_lower>` translation keyed on the ErrorCode
 * so the displayed string adapts to the active locale, but fall back to the
 * raw `err.message` the Host emitted when we don't have a translation yet.
 * The raw message usually contains contextual bits (paths, syscall names)
 * that a fixed template can't capture — so we only swap it in when a
 * translation exists.
 */
function translateErrorMessage(err: IpcError): string {
  const key = `error_${err.code.toLowerCase()}`;
  const translated = chrome.i18n.getMessage(key);
  return translated || err.message;
}

export function ErrorBanner(): JSX.Element | null {
  const error = useExplorerStore((s) => s.error);
  const clearError = useExplorerStore((s) => s.clearError);
  const reload = useExplorerStore((s) => s.reload);
  const loadDrives = useExplorerStore((s) => s.loadDrives);
  const currentPath = useExplorerStore((s) => s.currentPath);

  if (!error) return null;

  const hint = installHint(error);

  return (
    <div role="alert" className="error-banner">
      <div className="error-banner-body">
        <strong className="error-banner-code">{error.code}</strong>
        <span className="error-banner-msg">{translateErrorMessage(error)}</span>
        {error.retryable && (
          <span className="error-banner-tag">{t("error_retryable_tag")}</span>
        )}
        {hint && <div className="error-banner-hint">{hint}</div>}
      </div>
      <div className="error-banner-actions">
        <button
          type="button"
          onClick={() => {
            if (currentPath === null) {
              void loadDrives();
            } else {
              void reload();
            }
          }}
        >
          {t("common_retry")}
        </button>
        <button type="button" onClick={clearError}>
          {t("common_dismiss")}
        </button>
      </div>
    </div>
  );
}
