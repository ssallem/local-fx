import { useExplorerStore } from "../store/explorer";
import type { IpcError } from "../ipc";

function installHint(err: IpcError): string | null {
  if (err.code === "E_HOST_NOT_FOUND") {
    return "Native host 'com.local.fx' was not found. Run the installer or check the NativeMessagingHosts manifest registration.";
  }
  if (err.details && err.details["mayNeedInstall"] === true) {
    return "The native host seems to be missing or unregistered. Run the installer.";
  }
  return null;
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
        <span className="error-banner-msg">{error.message}</span>
        {error.retryable && (
          <span className="error-banner-tag">retryable</span>
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
          Retry
        </button>
        <button type="button" onClick={clearError}>
          Dismiss
        </button>
      </div>
    </div>
  );
}
