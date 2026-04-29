import { useCallback, useEffect, useRef, useState } from "react";
import { t } from "../utils/i18n";

/**
 * Hardcoded GitHub Releases asset URL for the Windows installer.
 *
 * TODO(T2 follow-up): The release.yml workflow currently produces
 * `localfx-host-setup-v<version>.exe` which is NOT a stable URL. We need
 * release.yml to additionally upload (or rename to) a fixed
 * `localfx-host-setup-windows.exe` so the `latest/download/...` redirect
 * resolves. Until that ships, this URL will 404. Tracked separately.
 */
const WINDOWS_DOWNLOAD_URL =
  "https://github.com/ssallem/local-fx/releases/latest/download/localfx-host-setup-windows.exe";

/**
 * Max attempts in a single retry sequence. The first attempt fires immediately
 * (no wait, RETRY_BACKOFF_MS[0] = 0) and counts as attempt 1; the next three
 * are gated by the 2s/5s/10s backoff entries below. So the user sees
 * "Retrying… (N/4)" where attempt 1 = the immediate try and attempts 2-4 = the
 * backoff retries. The /4 in the i18n key onboarding_retry_in_progress matches
 * MAX_RETRY_ATTEMPTS — keep them in sync if either changes.
 */
const MAX_RETRY_ATTEMPTS = 4;
/** Backoff schedule between attempts in milliseconds. Index N is the delay BEFORE attempt N+1. */
const RETRY_BACKOFF_MS: readonly number[] = [0, 2000, 5000, 10000];

type OsKind = "windows" | "macos" | "other";

interface HostMissingOnboardingProps {
  onRetry: () => Promise<void>;
  onDismiss?: () => void;
}

/**
 * Detect the host OS using the modern `userAgentData` API, falling back to
 * the legacy `navigator.platform` string. We only need a coarse classifier
 * (windows / macos / other) so the install path can adapt — exact version
 * detection isn't required.
 *
 * `userAgentData` is the only one of these accessors we *want* (it's the
 * non-deprecated path). The `platform` fallback is here purely to keep the
 * component working on older Chrome point releases that haven't shipped UA-CH
 * yet, even though our manifest targets Chrome 116+.
 */
function detectOs(): OsKind {
  const navAny = navigator as Navigator & {
    userAgentData?: { platform?: string };
  };
  const platform =
    navAny.userAgentData?.platform ?? navigator.platform ?? "";
  const lower = platform.toLowerCase();
  if (lower.includes("win")) return "windows";
  if (lower.includes("mac")) return "macos";
  return "other";
}

export function HostMissingOnboarding({
  onRetry,
  onDismiss
}: Readonly<HostMissingOnboardingProps>): JSX.Element {
  const [retrying, setRetrying] = useState<boolean>(false);
  const [retryAttempt, setRetryAttempt] = useState<number>(0);
  const [retryExhausted, setRetryExhausted] = useState<boolean>(false);
  const [osKind, setOsKind] = useState<OsKind>("other");
  // Re-entry guard for handleRetry. Held in a ref so the callback identity
  // stays stable (no `retrying` in the dep array) — using state here would
  // rebuild handleRetry mid-loop and could lose pending state updates.
  const retryingRef = useRef<boolean>(false);

  useEffect(() => {
    setOsKind(detectOs());
  }, []);

  const handleDownload = useCallback((): void => {
    if (osKind !== "windows") return;
    // window.open from a user-gesture click works in extension contexts and
    // does NOT require the `tabs` permission. Using "_blank" so the current
    // tab (the new-tab override) is preserved. `noopener,noreferrer` strips
    // window.opener (the GitHub release page is third-party) — same idiom
    // as UpdateToast.tsx's download handler; keep them in lock-step.
    window.open(WINDOWS_DOWNLOAD_URL, "_blank", "noopener,noreferrer");
  }, [osKind]);

  const handleRetry = useCallback(async (): Promise<void> => {
    // Re-entry guard via ref keeps the callback identity stable across
    // renders — adding `retrying` to deps below would rebuild handleRetry on
    // every state flip mid-loop, causing button onClick to capture stale
    // copies and (in pathological cases) start a parallel retry sequence.
    if (retryingRef.current) return;
    retryingRef.current = true;
    setRetrying(true);
    setRetryExhausted(false);
    try {
      for (let i = 0; i < MAX_RETRY_ATTEMPTS; i += 1) {
        const wait = RETRY_BACKOFF_MS[i] ?? 0;
        if (wait > 0) {
          await new Promise<void>((resolve) => {
            window.setTimeout(resolve, wait);
          });
        }
        setRetryAttempt(i + 1);
        try {
          await onRetry();
          // Success: the parent's store will clear `error` and unmount us.
          // Resetting local state is a safety net for the rare race where
          // the unmount hasn't fired before we read setRetrying below.
          setRetryAttempt(0);
          return;
        } catch {
          // swallow — the store has already captured the error. Loop to next attempt.
        }
      }
      // Fell through all attempts without success.
      setRetryExhausted(true);
    } finally {
      retryingRef.current = false;
      setRetrying(false);
    }
  }, [onRetry]);

  const downloadLabel =
    osKind === "windows"
      ? t("onboarding_download_btn")
      : t("onboarding_download_btn_unavailable");

  const retryLabel = retrying
    ? t("onboarding_retry_in_progress", [String(retryAttempt)])
    : t("onboarding_retry_btn");

  const steps: ReadonlyArray<{ key: string; captionKey: string }> = [
    { key: "onboarding_step_1", captionKey: "onboarding_step_caption_1" },
    { key: "onboarding_step_2", captionKey: "onboarding_step_caption_2" },
    { key: "onboarding_step_3", captionKey: "onboarding_step_caption_3" },
    { key: "onboarding_step_4", captionKey: "onboarding_step_caption_4" }
  ];

  return (
    <div className="onboarding-panel" role="region" aria-live="polite">
      <h1 className="onboarding-title">{t("onboarding_title")}</h1>
      <p className="onboarding-subtitle">{t("onboarding_subtitle")}</p>

      {osKind === "macos" && (
        <p className="onboarding-subtitle onboarding-macos-pending">
          {t("onboarding_macos_pending")}
        </p>
      )}

      <ol className="onboarding-steps">
        {steps.map((s, idx) => (
          <li key={s.key} className="onboarding-step">
            <span className="onboarding-step-num">{idx + 1}</span>
            <div className="onboarding-step-text">
              <div>{t(s.key)}</div>
              <div className="onboarding-step-caption">{t(s.captionKey)}</div>
            </div>
          </li>
        ))}
      </ol>

      <div className="onboarding-actions">
        {/* `title` only emitted when the OS makes the button unusable —
            with exactOptionalPropertyTypes, splitting the JSX avoids
            passing `title={undefined}` which is rejected as a value. */}
        {osKind !== "windows" ? (
          <button
            type="button"
            className="onboarding-btn-primary"
            onClick={handleDownload}
            disabled
            title={t("onboarding_download_btn_unavailable")}
          >
            {downloadLabel}
          </button>
        ) : (
          <button
            type="button"
            className="onboarding-btn-primary"
            onClick={handleDownload}
            disabled={retrying}
          >
            {downloadLabel}
          </button>
        )}
        <button
          type="button"
          className="onboarding-btn-secondary"
          onClick={() => {
            void handleRetry();
          }}
          disabled={retrying}
        >
          {retrying && <span className="onboarding-spinner" aria-hidden />}
          {retryLabel}
        </button>
        {onDismiss && (
          <button
            type="button"
            className="onboarding-btn-secondary"
            onClick={onDismiss}
            disabled={retrying}
          >
            {t("onboarding_dismiss")}
          </button>
        )}
      </div>

      {retryExhausted && (
        <p className="onboarding-retry-exhausted" role="alert">
          {t("onboarding_retry_exhausted")}
        </p>
      )}

      <p className="onboarding-log-hint">{t("onboarding_log_hint")}</p>
    </div>
  );
}
